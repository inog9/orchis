package demo_test

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"orchis-go/internal/demo"
	"orchis-go/internal/testutil"
	"strings"
	"testing"
	"time"
)

type artifactSpy struct {
	fail     bool
	blobs    []string
	findings []uuid.UUID
}

func (s *artifactSpy) Remove(ctx context.Context, key string) error {
	if s.fail {
		return errors.New("local storage unavailable")
	}
	s.blobs = append(s.blobs, key)
	return nil
}
func (s *artifactSpy) RemoveFindings(ctx context.Context, tenant uuid.UUID, ids []uuid.UUID) error {
	if s.fail {
		return errors.New("local search unavailable")
	}
	s.findings = append(s.findings, ids...)
	return nil
}
func TestPurgeRetriesArtifactCleanupAfterDatabaseCommit(t *testing.T) {
	env := testutil.New(t)
	ctx := context.Background()
	addAdmin(t, env, env.TenantA)
	spy := &artifactSpy{fail: true}
	svc := &demo.Service{DB: env.App, Blob: spy, Search: spy}
	if err := svc.Seed(ctx, env.TenantA, 1, nil, "", "test"); err != nil {
		t.Fatal(err)
	}
	st, err := svc.Purge(ctx, env.TenantA, nil, "", "test")
	if err != nil {
		t.Fatal(err)
	}
	if st.HasDemo || st.CleanupPending == 0 {
		t.Fatalf("Database deletion must be distinguished from artifact cleanup: %+v", st)
	}
	if err := svc.Seed(ctx, env.TenantA, 1, nil, "", "test"); err == nil {
		t.Fatal("Reseed raced pending artifact deletion")
	}
	spy = &artifactSpy{}
	svc = &demo.Service{DB: env.App, Blob: spy, Search: spy}
	st, err = svc.Purge(ctx, env.TenantA, nil, "", "retry")
	if err != nil || st.HasDemo || st.CleanupPending != 0 {
		t.Fatalf("Retry incomplete: %+v %v", st, err)
	}
	if len(spy.blobs) == 0 || len(spy.findings) == 0 {
		t.Fatal("External adapters not invoked")
	}
	for _, key := range spy.blobs {
		if !strings.HasPrefix(key, env.TenantA.String()+"/reports/") {
			t.Fatal("Unexpected artifact scope")
		}
	}
	if n := count(t, env, `SELECT count(*) FROM audit_log WHERE tenant_id=$1 AND action='demo.purge'`, env.TenantA); n != 1 {
		t.Fatal("Retry duplicated operator audit", n)
	}
	if err := svc.Seed(ctx, env.TenantA, 1, nil, "", "reseed"); err != nil {
		t.Fatal(err)
	}
}
func TestPurgeDoesNotDeleteArtifactsWhenDatabaseRollsBack(t *testing.T) {
	env := testutil.New(t)
	ctx := context.Background()
	addAdmin(t, env, env.TenantA)
	spy := &artifactSpy{}
	svc := &demo.Service{DB: env.App, Blob: spy, Search: spy}
	if err := svc.Seed(ctx, env.TenantA, 1, nil, "", "test"); err != nil {
		t.Fatal(err)
	}
	_, err := env.Owner.Exec(ctx, `CREATE FUNCTION reject_demo_purge() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='demo.purge' THEN RAISE EXCEPTION 'local audit unavailable'; END IF; RETURN NEW; END $$;
 CREATE TRIGGER reject_demo_purge BEFORE INSERT ON audit_log FOR EACH ROW EXECUTE FUNCTION reject_demo_purge()`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Purge(ctx, env.TenantA, nil, "", "test"); err == nil {
		t.Fatal("Audit failure accepted")
	}
	if len(spy.blobs)+len(spy.findings) != 0 {
		t.Fatal("External deletion escaped a rolled-back DB transaction")
	}
	if n := count(t, env, `SELECT count(*) FROM demo_cleanup_tasks WHERE tenant_id=$1`, env.TenantA); n != 0 {
		t.Fatal("Cleanup intent survived rollback")
	}
	if n := count(t, env, `SELECT count(*) FROM assets WHERE tenant_id=$1 AND is_demo`, env.TenantA); n != 50 {
		t.Fatal("Partial database deletion")
	}
}
func TestPurgeWaitsForRenderingAndRejectsForeignObjectKeys(t *testing.T) {
	for _, mode := range []string{"rendering", "foreign-key"} {
		t.Run(mode, func(t *testing.T) {
			env := testutil.New(t)
			ctx := context.Background()
			addAdmin(t, env, env.TenantA)
			spy := &artifactSpy{}
			svc := &demo.Service{DB: env.App, Blob: spy, Search: spy}
			if err := svc.Seed(ctx, env.TenantA, 1, nil, "", "test"); err != nil {
				t.Fatal(err)
			}
			query := `UPDATE reports SET status='rendering' WHERE id=(SELECT id FROM reports WHERE tenant_id=$1 AND is_demo LIMIT 1)`
			if mode == "foreign-key" {
				query = `UPDATE reports SET pdf_key='some-real-object' WHERE id=(SELECT id FROM reports WHERE tenant_id=$1 AND is_demo LIMIT 1)`
			}
			if _, err := env.Owner.Exec(ctx, query, env.TenantA); err != nil {
				t.Fatal(err)
			}
			if _, err := svc.Purge(ctx, env.TenantA, nil, "", "test"); err == nil {
				t.Fatal("Unsafe purge accepted")
			}
			if len(spy.blobs)+len(spy.findings) != 0 || count(t, env, `SELECT count(*) FROM assets WHERE tenant_id=$1 AND is_demo`, env.TenantA) != 50 {
				t.Fatal("Purge escaped guard")
			}
		})
	}
}
func TestCleanupRejectsCrossTenantObjectKey(t *testing.T) {
	env := testutil.New(t)
	ctx := context.Background()
	spy := &artifactSpy{}
	svc := &demo.Service{DB: env.App, Blob: spy, Search: spy}
	key := env.TenantB.String() + "/reports/" + uuid.NewString() + ".pdf"
	if _, err := env.Owner.Exec(ctx, `INSERT INTO demo_cleanup_tasks(tenant_id,kind,target) VALUES($1,'blob',$2)`, env.TenantA, key); err != nil {
		t.Fatal(err)
	}
	if err := svc.CleanupOnce(ctx, env.TenantA); err == nil {
		t.Fatal("Cross-tenant object key accepted")
	}
	if len(spy.blobs) != 0 {
		t.Fatal("Foreign object removed")
	}
}
func TestPurgeCannotCascadeARealChildInsertedDuringItsGuard(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	env := testutil.New(t)
	addAdmin(t, env, env.TenantA)
	spy := &artifactSpy{}
	svc := &demo.Service{DB: env.App, Blob: spy, Search: spy}
	if err := svc.Seed(ctx, env.TenantA, 1, nil, "", "test"); err != nil {
		t.Fatal(err)
	}
	var asset uuid.UUID
	if err := env.Owner.QueryRow(ctx, `SELECT id FROM assets WHERE tenant_id=$1 AND is_demo LIMIT 1`, env.TenantA).Scan(&asset); err != nil {
		t.Fatal(err)
	}
	conn, err := env.Owner.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, `SELECT id FROM assets WHERE id=$1 FOR KEY SHARE`, asset); err != nil {
		t.Fatal(err)
	}
	ownerPID := int32(conn.Conn().PgConn().PID())
	done := make(chan error, 1)
	go func() { _, e := svc.Purge(ctx, env.TenantA, nil, "", "concurrent"); done <- e }()
	// Observe an actual PostgreSQL dependency, not a timing-only assertion.
	for {
		var blocked int
		if err = env.Owner.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid))`, ownerPID).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked > 0 {
			break
		}
		select {
		case e := <-done:
			t.Fatalf("Purge completed before held FK lock: %v", e)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(5 * time.Millisecond):
		}
	}
	fid := uuid.New()
	if _, err = tx.Exec(ctx, `INSERT INTO findings(id,tenant_id,asset_id,type,value,value_hash,title) VALUES($1,$2,$3,'text','real-concurrent','\x01','REAL fixture data')`, fid, env.TenantA, asset); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-done:
		if err == nil {
			t.Fatal("Real concurrent child was silently cascaded")
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if count(t, env, `SELECT count(*) FROM findings WHERE id=$1`, fid) != 1 || count(t, env, `SELECT count(*) FROM assets WHERE id=$1`, asset) != 1 {
		t.Fatal("Real data or its demo parent was removed")
	}
	if len(spy.blobs)+len(spy.findings) != 0 {
		t.Fatal("External data deleted after rejected purge")
	}
}

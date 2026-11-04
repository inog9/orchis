package retention

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"orchis-go/internal/testutil"
	"reflect"
	"testing"
)

type cleaner struct {
	keys                 []string
	tenant               uuid.UUID
	ids                  []uuid.UUID
	failBlob, failSearch bool
}

func (c *cleaner) Remove(_ context.Context, key string) error {
	if c.failBlob {
		return errors.New("fixture storage unavailable")
	}
	c.keys = append(c.keys, key)
	return nil
}
func (c *cleaner) RemoveFindings(_ context.Context, tenant uuid.UUID, ids []uuid.UUID) error {
	if c.failSearch {
		return errors.New("fixture search unavailable")
	}
	c.tenant = tenant
	c.ids = append(c.ids, ids...)
	return nil
}
func TestRetentionCleanupExactKeysRestartAndTenantScope(t *testing.T) {
	e := testutil.New(t)
	ctx := context.Background()
	op, org, report, execution, finding := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	must := func(q string, args ...any) {
		t.Helper()
		if _, err := e.Owner.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	must(`INSERT INTO data_deletions(id,tenant_id,organization_id,organization_name,counts) VALUES($1,$2,$3,'Fixture','{}')`, op, e.TenantA, org)
	for _, v := range []struct {
		kind string
		id   uuid.UUID
	}{{"report_pdf", report}, {"execution_raw", execution}, {"search_finding", finding}} {
		must(`INSERT INTO data_cleanup_tasks(tenant_id,kind,object_id,organization_id,operation_id) VALUES($1,$2,$3,$4,$5)`, e.TenantA, v.kind, v.id, org, op)
		must(`INSERT INTO data_cleanup_tasks(tenant_id,kind,object_id) VALUES($1,$2,$3)`, e.TenantB, v.kind, v.id)
	}
	count := func(tenant uuid.UUID) int {
		t.Helper()
		var n int
		if err := e.Owner.QueryRow(ctx, `SELECT count(*) FROM data_cleanup_tasks WHERE tenant_id=$1`, tenant).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	missing := Service{DB: e.App}
	if err := missing.CleanupOnce(ctx, e.TenantA); err != nil || count(e.TenantA) != 3 {
		t.Fatal("missing adapters must preserve durable cleanup")
	}
	c := &cleaner{failBlob: true}
	s := Service{DB: e.App, Blob: c, Search: c}
	if err := s.CleanupOnce(ctx, e.TenantA); err == nil || count(e.TenantA) != 3 {
		t.Fatal("storage failure lost deletion intents")
	}
	c.failBlob = false
	c.failSearch = true
	restarted := Service{DB: e.App, Blob: c, Search: c}
	if err := restarted.CleanupOnce(ctx, e.TenantA); err == nil || count(e.TenantA) != 1 {
		t.Fatal("partial cleanup should keep failed search intent")
	}
	var complete bool
	if err := e.Owner.QueryRow(ctx, `SELECT completed_at IS NOT NULL FROM data_deletions WHERE id=$1`, op).Scan(&complete); err != nil || complete {
		t.Fatal("incomplete external cleanup reported complete")
	}
	c.failSearch = false
	if err := restarted.CleanupOnce(ctx, e.TenantA); err != nil {
		t.Fatal(err)
	}
	if count(e.TenantA) != 0 || count(e.TenantB) != 3 {
		t.Fatal("cleanup escaped tenant or left completed intents")
	}
	if !reflect.DeepEqual(c.keys, []string{e.TenantA.String() + "/reports/" + report.String() + ".pdf", "executions/" + e.TenantA.String() + "/" + execution.String() + "/output.txt"}) || c.tenant != e.TenantA || !reflect.DeepEqual(c.ids, []uuid.UUID{finding}) {
		t.Fatalf("unexpected external deletion: %+v", c)
	}
	if err := e.Owner.QueryRow(ctx, `SELECT completed_at IS NOT NULL FROM data_deletions WHERE id=$1`, op).Scan(&complete); err != nil || !complete {
		t.Fatal("completed cleanup not persisted")
	}
	if err := restarted.CleanupOnce(ctx, e.TenantA); err != nil || len(c.keys) != 2 || len(c.ids) != 1 {
		t.Fatal("completed cleanup should be idempotent")
	}
}

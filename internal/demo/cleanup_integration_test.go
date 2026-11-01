package demo_test

import (
	"bytes"
	"context"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"orchis-go/internal/blob"
	"orchis-go/internal/demo"
	"orchis-go/internal/search"
	"orchis-go/internal/testutil"
	"strings"
	"testing"
	"time"
)

// Real local MinIO and Elasticsearch, with uniquely owned bucket/index. The
// object bytes are a synthetic fixture, not a PDF rendering proof.
func TestPurgeRemovesOwnedMinIOAndSearchArtifactsOnly(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	env := testutil.New(t)
	addAdmin(t, env, env.TenantA)
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	bucket := "orchis-demo-cleanup-" + suffix
	admin, err := minio.New("127.0.0.1:19000", &minio.Options{Creds: credentials.NewStaticV4("orchis", "orchis-dev-secret", ""), Secure: false})
	if err != nil {
		t.Fatal(err)
	}
	keys := []string{}
	t.Cleanup(func() {
		cleanup, done := context.WithTimeout(context.Background(), 15*time.Second)
		defer done()
		for _, key := range keys {
			_ = admin.RemoveObject(cleanup, bucket, key, minio.RemoveObjectOptions{})
		}
		if err := admin.RemoveBucket(cleanup, bucket); err != nil && minio.ToErrorResponse(err).Code != "NoSuchBucket" {
			t.Errorf("Owned bucket cleanup failed: %v", err)
		}
	})
	store, err := blob.Open(ctx, blob.Options{Endpoint: "127.0.0.1:19000", AccessKey: "orchis", SecretKey: "orchis-dev-secret", Bucket: bucket})
	if err != nil {
		t.Fatal(err)
	}
	es := search.New("http://127.0.0.1:19200")
	es.Index = "orchis-demo-cleanup-" + suffix
	t.Cleanup(func() {
		cleanup, done := context.WithTimeout(context.Background(), 15*time.Second)
		defer done()
		if err := es.DeleteIndex(cleanup); err != nil {
			t.Errorf("Owned index cleanup failed: %v", err)
		}
	})
	if err = es.EnsureIndex(ctx); err != nil {
		t.Fatal(err)
	}
	svc := &demo.Service{DB: env.App, Blob: store, Search: es}
	if err = svc.Seed(ctx, env.TenantA, 1, nil, "", "test"); err != nil {
		t.Fatal(err)
	}
	var report uuid.UUID
	if err = env.Owner.QueryRow(ctx, `SELECT id FROM reports WHERE tenant_id=$1 AND is_demo LIMIT 1`, env.TenantA).Scan(&report); err != nil {
		t.Fatal(err)
	}
	key := env.TenantA.String() + "/reports/" + report.String() + ".pdf"
	controlKey := env.TenantA.String() + "/reports/" + uuid.NewString() + ".pdf"
	keys = append(keys, key, controlKey)
	value := []byte("LOCAL SYNTHETIC OBJECT FIXTURE")
	for _, k := range keys {
		if err = store.Put(ctx, k, value, "application/octet-stream"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = env.Owner.Exec(ctx, `UPDATE reports SET pdf_key=$3,status='ready' WHERE tenant_id=$1 AND id=$2`, env.TenantA, report, key); err != nil {
		t.Fatal(err)
	}
	rows, err := env.Owner.Query(ctx, `SELECT id FROM findings WHERE tenant_id=$1 AND is_demo ORDER BY id LIMIT 2`, env.TenantA)
	if err != nil {
		t.Fatal(err)
	}
	ids := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	if len(ids) != 2 {
		t.Fatal("Missing fixture findings")
	}
	control := uuid.New()
	for _, d := range []struct {
		id, tenant uuid.UUID
		title      string
	}{{ids[0], env.TenantA, "DEMO artifact"}, {ids[1], env.TenantB, "foreign control"}, {control, env.TenantA, "real control"}} {
		if err = es.IndexFinding(ctx, d.id, search.FindingDoc{TenantID: d.tenant, Title: d.title, Type: "text", Severity: "info", Value: d.title, LastSeenAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	if err = es.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	st, err := svc.Purge(ctx, env.TenantA, nil, "", "test")
	if err != nil || st.CleanupPending != 0 || st.HasDemo {
		t.Fatalf("Incomplete purge %+v %v", st, err)
	}
	if _, err = store.Get(ctx, key); err == nil {
		t.Fatal("Demo object remains")
	}
	if got, err := store.Get(ctx, controlKey); err != nil || !bytes.Equal(got, value) {
		t.Fatal("Non-demo object changed")
	}
	if found, err := es.SearchFindings(ctx, env.TenantA, "DEMO", 200); err != nil || len(found) != 0 {
		t.Fatal("Demo index document remains", err)
	}
	if found, err := es.SearchFindings(ctx, env.TenantA, "real control", 200); err != nil || len(found) != 1 || found[0] != control {
		t.Fatal("Real index document changed", err)
	}
	if found, err := es.SearchFindings(ctx, env.TenantB, "foreign control", 200); err != nil || len(found) != 1 || found[0] != ids[1] {
		t.Fatal("Foreign tenant document changed", err)
	}
	if _, err = svc.Purge(ctx, env.TenantA, nil, "", "retry"); err != nil {
		t.Fatal(err)
	}
}

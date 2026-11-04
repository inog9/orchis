package retention

import (
	"bytes"
	"context"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"orchis-go/internal/blob"
	"orchis-go/internal/search"
	"orchis-go/internal/testutil"
	"strings"
	"testing"
	"time"
)

func TestOrganizationDeletionRemovesRealOwnedMinIOAndESArtifacts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	e := testutil.New(t)
	p := admin(t, e, e.TenantA)
	pb := admin(t, e, e.TenantB)
	a, b, foreign := fixture(t, e, p), fixture(t, e, p), fixture(t, e, pb)
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	bucket := "orchis-retention-" + suffix
	mc, err := minio.New("127.0.0.1:19000", &minio.Options{Creds: credentials.NewStaticV4("orchis", "orchis-dev-secret", ""), Secure: false})
	if err != nil {
		t.Fatal(err)
	}
	keys := []string{}
	t.Cleanup(func() {
		c, done := context.WithTimeout(context.Background(), 15*time.Second)
		defer done()
		for _, key := range keys {
			_ = mc.RemoveObject(c, bucket, key, minio.RemoveObjectOptions{})
		}
		if err := mc.RemoveBucket(c, bucket); err != nil && minio.ToErrorResponse(err).Code != "NoSuchBucket" {
			t.Error(err)
		}
	})
	store, err := blob.Open(ctx, blob.Options{Endpoint: "127.0.0.1:19000", AccessKey: "orchis", SecretKey: "orchis-dev-secret", Bucket: bucket})
	if err != nil {
		t.Fatal(err)
	}
	es := search.New("http://127.0.0.1:19200")
	es.Index = "orchis-retention-" + suffix
	t.Cleanup(func() {
		c, done := context.WithTimeout(context.Background(), 15*time.Second)
		defer done()
		if err := es.DeleteIndex(c); err != nil {
			t.Error(err)
		}
	})
	if err = es.EnsureIndex(ctx); err != nil {
		t.Fatal(err)
	}
	body := []byte("LOCAL SYNTHETIC DELETION FIXTURE")
	for _, v := range []struct {
		f      orgFixture
		tenant uuid.UUID
	}{{a, p.TenantID}, {b, p.TenantID}, {foreign, pb.TenantID}} {
		keys = append(keys, v.tenant.String()+"/reports/"+v.f.report.String()+".pdf", "executions/"+v.tenant.String()+"/"+v.f.execution.String()+"/output.txt")
		if err = es.IndexFinding(ctx, v.f.finding, search.FindingDoc{TenantID: v.tenant, Title: v.f.name, Type: "text", Value: v.f.name, Severity: "info", LastSeenAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	for _, key := range keys {
		if err = store.Put(ctx, key, body, "application/octet-stream"); err != nil {
			t.Fatal(err)
		}
	}
	if err = es.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	// Database commit must not depend on external storage availability.
	offline := &cleaner{failBlob: true, failSearch: true}
	s := Service{DB: e.App, Blob: offline, Search: offline}
	preview, err := s.Preview(ctx, p, a.org)
	if err != nil {
		t.Fatal(err)
	}
	operation, err := s.DeleteOrganization(ctx, p, a.org, DeleteInput{Name: preview.Name, Version: preview.Version, Counts: preview.Counts}, "", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.CleanupOnce(ctx, p.TenantID); err == nil {
		t.Fatal("outage unexpectedly completed")
	}
	state, err := s.Deletion(ctx, p, operation.ID)
	if err != nil || state.Status != "external_cleanup_pending" || state.Pending != 3 {
		t.Fatalf("outage lost cleanup intents: %+v %v", state, err)
	}
	// Simulate process restart with restored real adapters.
	restarted := Service{DB: e.App, Blob: store, Search: es}
	if err = restarted.CleanupOnce(ctx, p.TenantID); err != nil {
		t.Fatal(err)
	}
	for i, key := range keys {
		got, err := store.Get(ctx, key)
		if i < 2 {
			if err == nil {
				t.Fatal("owned object remains", key)
			}
		} else if err != nil || !bytes.Equal(got, body) {
			t.Fatal("other organisation artifact changed", key, err)
		}
	}
	for _, v := range []struct {
		f      orgFixture
		tenant uuid.UUID
		want   int
	}{{a, p.TenantID, 0}, {b, p.TenantID, 1}, {foreign, pb.TenantID, 1}} {
		found, err := es.SearchFindings(ctx, v.tenant, v.f.name, 200)
		if err != nil || len(found) != v.want {
			t.Fatalf("wrong scoped search cleanup %s: %v %v", v.f.name, found, err)
		}
	}
	state, err = restarted.Deletion(ctx, p, operation.ID)
	if err != nil || state.Status != "completed" || state.Pending != 0 {
		t.Fatalf("completion: %+v %v", state, err)
	}
	if err = restarted.CleanupOnce(ctx, p.TenantID); err != nil {
		t.Fatal(err)
	}
}

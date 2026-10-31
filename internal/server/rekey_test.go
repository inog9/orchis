package server_test

import (
	"bytes"
	"context"
	"orchis-go/internal/ai"
	"testing"
)

func TestMasterRekeyRejectsQueuedProviderWork(t *testing.T) {
	f, org, report, policy := aiFixture(t)
	r := f.a.do("PUT", "/api/v1/organizations/"+org.String()+"/ai", policy, f.admin)
	if r.Status != 200 {
		t.Fatalf("policy setup %d", r.Status)
	}
	aiRequest(t, f, org, report, "local-rekey-job")
	svc := &ai.Service{DB: f.a.env.App, Tenant: f.a.env.TenantA, Key: bytes.Repeat([]byte{9}, 32)}
	if n, err := svc.Rekey(context.Background(), bytes.Repeat([]byte{8}, 32)); err == nil || n != 0 {
		t.Fatal("rotation accepted queued provider work")
	}
}

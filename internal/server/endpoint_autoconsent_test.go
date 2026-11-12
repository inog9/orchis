package server_test

import (
	"context"
	"testing"
	"time"

	"orchis-go/internal/db"
	"orchis-go/internal/endpoint"
)

func TestEnrollmentNeverCreatesOrganizationalConsent(t *testing.T) {
	// The shared fixture deliberately sets the deprecated AutoConsent option:
	// even that must not grant approval on enrollment.
	f := newAgentFixture(t)
	org := createOrg(t, f.a, f.admin, "explicit-consent")
	r := f.a.do("POST", "/api/v1/agents/enrollment-tokens", map[string]any{"os": "linux", "arch": "x86_64", "mode": "service", "organization_id": org.ID.String()}, f.admin)
	if r.Status != 201 {
		t.Fatalf("token: %d %s", r.Status, r.Body)
	}
	var token struct{ Token string }
	r.JSON(&token)
	r = f.a.do("POST", "/api/v1/agent/enroll", f.enrollBody(token.Token, "explicit-consent-host"), "", "X-Requested-With", "")
	if r.Status != 200 {
		t.Fatalf("enroll: %d %s", r.Status, r.Body)
	}
	var n int
	if err := f.a.env.Owner.QueryRow(context.Background(), `SELECT count(*) FROM osquery_org_settings WHERE organization_id=$1`, org.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("enrollment granted %d query approvals", n)
	}
}

func TestCollectionLoopNeverCreatesOrganizationalConsent(t *testing.T) {
	f := newAgentFixture(t)
	_, asset := f.authorizedAgent(t, "explicit-scheduler")
	org := f.joinOrg(t, asset)
	svc := &endpoint.Service{DB: &db.DB{Pool: f.a.env.Owner}, AutoConsent: true}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	time.AfterFunc(250*time.Millisecond, cancel)
	svc.RunCollection(ctx, f.a.env.TenantA, time.Minute)
	var n int
	if err := f.a.env.Owner.QueryRow(context.Background(), `SELECT count(*) FROM osquery_org_settings WHERE organization_id=$1`, org).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("scheduler granted %d query approvals", n)
	}
}

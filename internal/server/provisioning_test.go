package server_test

import (
	"context"
	"encoding/base64"
	"orchis-go/internal/execution"
	"orchis-go/internal/testutil"
	"strings"
	"testing"
)

func TestProvisioningCannotRestoreRotatedCredential(t *testing.T) {
	env := testutil.New(t)
	ctx := context.Background()
	token := "oij_" + base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("x", 32)))
	for range 2 {
		if e := execution.ProvisionToken(ctx, env.App, env.TenantA, "nmap", token); e != nil {
			t.Fatal(e)
		}
	}
	current, e := execution.IssueToken(ctx, env.App, env.TenantA, "nmap")
	if e != nil {
		t.Fatal(e)
	}
	if e = execution.ProvisionToken(ctx, env.App, env.TenantA, "nmap", token); e == nil {
		t.Fatal("provisioning resurrected a rotated token")
	}
	rt := execution.Runtime{DB: env.App, Tenant: env.TenantA}
	if _, e = rt.Authenticate(ctx, token); e == nil {
		t.Fatal("old token became valid")
	}
	if _, e = rt.Authenticate(ctx, current); e != nil {
		t.Fatal(e)
	}
	if e = execution.ProvisionToken(ctx, env.App, env.TenantA, "nmap", current); e != nil {
		t.Fatal("current file cannot be applied idempotently", e)
	}
}

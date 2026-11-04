package server_test

import (
	"context"
	"github.com/google/uuid"
	"orchis-go/internal/execution"
	"orchis-go/internal/osquery"
	"orchis-go/internal/server"
	"strings"
	"testing"
)

func TestOsqueryResultRechecksFrozenPolicyAndAuthorization(t *testing.T) {
	for _, kind := range []string{"unchanged", "unrelated-query", "disabled", "disable-enable", "asset-revoked", "asset-reauthorized", "moved-org", "agent-revoked", "missing-snapshot"} {
		t.Run(kind, func(t *testing.T) {
			f := newAgentFixture(t)
			secret, asset := f.approvedAgent(t, "policy-box", "osquery.users")
			ex := f.agentTask(t, secret, asset, "osquery.users")
			var org string
			if err := f.a.env.Owner.QueryRow(context.Background(), `SELECT organization_id FROM assets WHERE id=$1`, asset).Scan(&org); err != nil {
				t.Fatal(err)
			}
			setting := func(key string, enabled bool) {
				t.Helper()
				if r := f.a.do("PUT", "/api/v1/organizations/"+org+"/osquery/"+key, f.collectionChoice(t, org, key, enabled), f.admin); r.Status != 204 {
					t.Fatalf("setting: %d %s", r.Status, r.Body)
				}
			}
			switch kind {
			case "unrelated-query":
				setting("osquery.network", true)
			case "disabled":
				setting("osquery.users", false)
			case "disable-enable":
				setting("osquery.users", false)
				setting("osquery.users", true)
			case "asset-revoked":
				if _, err := f.a.env.Owner.Exec(context.Background(), `UPDATE assets SET testing_authorized=false WHERE id=$1`, asset); err != nil {
					t.Fatal(err)
				}
			case "asset-reauthorized":
				if _, err := f.a.env.Owner.Exec(context.Background(), `UPDATE assets SET testing_authorized=false,version=version+1 WHERE id=$1`, asset); err != nil {
					t.Fatal(err)
				}
				if _, err := f.a.env.Owner.Exec(context.Background(), `UPDATE assets SET testing_authorized=true,version=version+1 WHERE id=$1`, asset); err != nil {
					t.Fatal(err)
				}
			case "moved-org":
				other := createOrg(t, f.a, f.admin, "moved-"+asset[:8])
				if _, err := f.a.env.Owner.Exec(context.Background(), `UPDATE assets SET organization_id=$2 WHERE id=$1`, asset, other.ID); err != nil {
					t.Fatal(err)
				}
			case "agent-revoked":
				if _, err := f.a.env.Owner.Exec(context.Background(), `UPDATE agents SET status='revoked' WHERE asset_id=$1`, asset); err != nil {
					t.Fatal(err)
				}
			case "missing-snapshot":
				if _, err := f.a.env.Owner.Exec(context.Background(), `UPDATE executions SET endpoint_policy=NULL WHERE id=$1`, ex); err != nil {
					t.Fatal(err)
				}
			}
			var in execution.Injector
			if err := f.a.env.Owner.QueryRow(context.Background(), `SELECT integration_id,agent_id FROM executions WHERE id=$1`, ex).Scan(&in.ID, &in.AgentID); err != nil {
				t.Fatal(err)
			}
			store := &endpointStoreSpy{}
			rt := server.Runtime(server.Options{DB: f.a.env.App, Tenant: f.a.env.TenantA, Store: store})
			zero := 0
			body := execution.Result{ExitCode: &zero, Output: "policy-secret-marker", Datasets: []osquery.Dataset{{Query: "osquery.users", Status: osquery.StatusOK, Rows: []map[string]string{{"username": "policy-toor", "uid": "0", "shell": "/bin/sh", "groups": "root"}}}}}
			if err := rt.Complete(context.Background(), &in, uuid.MustParse(ex), body); err != nil {
				t.Fatal(err)
			}
			good := kind == "unchanged" || kind == "unrelated-query"
			got := f.exec(t, ex)
			n := rowCount(t, f, `SELECT count(*) FROM endpoint_datasets WHERE asset_id=$1`, asset)
			findings := rowCount(t, f, `SELECT count(*) FROM findings WHERE asset_id=$1`, asset)
			if good {
				if got.Outcome != "success" || n != 1 || findings != 1 {
					t.Fatalf("izin tetap berlaku: %+v dataset=%d temuan=%d", got, n, findings)
				}
			} else {
				if got.Outcome != "error" || n != 0 || findings != 0 {
					t.Fatalf("hasil kedaluwarsa tidak dibuang: %+v dataset=%d temuan=%d", got, n, findings)
				}
				var reason string
				if err := f.a.env.Owner.QueryRow(context.Background(), `SELECT error FROM executions WHERE id=$1`, ex).Scan(&reason); err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(reason, "discarded") && !strings.Contains(reason, "approval snapshot") {
					t.Fatalf("alasan tidak jelas: %q", reason)
				}
			}
			if store.calls != 0 {
				t.Fatal("hasil endpoint menulis objek mentah")
			}
		})
	}
}

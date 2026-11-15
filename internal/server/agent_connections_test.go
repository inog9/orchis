package server_test

import (
	"context"
	"testing"
	"time"

	"orchis-go/internal/endpoint"
	"orchis-go/internal/server"
)

func TestConnectionsTickQueuesOutboundCollectionOncePerDueAgent(t *testing.T) {
	f := newAgentFixture(t)
	_, asset := f.approvedAgent(t, "conn-host", "osquery.os_release")
	ctx := context.Background()
	svc := server.EndpointService(server.Options{DB: f.a.env.App})
	q := `SELECT count(*) FROM executions e JOIN contracts c ON c.tenant_id=e.tenant_id AND c.id=e.contract_id WHERE e.target_asset_id=$1 AND c.key='blueteam.connections' AND e.state='ready'`
	for i := 0; i < 2; i++ {
		if err := svc.ConnectionsTick(ctx, f.a.env.TenantA, endpoint.ConnectionsInterval); err != nil {
			t.Fatal(err)
		}
	}
	if n := rowCount(t, f, q, asset); n != 1 {
		t.Fatalf("expected exactly one queued connections collection, got %d", n)
	}
	// An organisation with no approved collection gets nothing.
	if _, err := f.a.env.Owner.Exec(ctx, `UPDATE executions SET state='ended',outcome='success',ended_at=now() WHERE target_asset_id=$1`, asset); err != nil {
		t.Fatal(err)
	}
	if _, err := f.a.env.Owner.Exec(ctx, `UPDATE endpoint_collection_schedule SET last_enqueued_at=now()-interval '5 minutes'`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.a.env.Owner.Exec(ctx, `UPDATE osquery_org_settings SET enabled=false`); err != nil {
		t.Fatal(err)
	}
	if err := svc.ConnectionsTick(ctx, f.a.env.TenantA, time.Minute); err != nil {
		t.Fatal(err)
	}
	if n := rowCount(t, f, q, asset); n != 0 {
		t.Fatalf("collected without an approved organisation: %d", n)
	}
}

func TestSweepClosesTasksOfAgentsThatStayedOffline(t *testing.T) {
	f := newAgentFixture(t)
	_, asset := f.approvedAgent(t, "gone-host", "osquery.os_release")
	ctx := context.Background()
	svc := server.EndpointService(server.Options{DB: f.a.env.App})
	if err := svc.ConnectionsTick(ctx, f.a.env.TenantA, endpoint.ConnectionsInterval); err != nil {
		t.Fatal(err)
	}
	if _, err := f.a.env.Owner.Exec(ctx, `UPDATE executions SET updated_at=now()-interval '1 hour' WHERE target_asset_id=$1`, asset); err != nil {
		t.Fatal(err)
	}
	if _, err := f.a.env.Owner.Exec(ctx, `UPDATE agents SET last_heartbeat_at=now()-interval '1 hour' WHERE asset_id=$1`, asset); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.rt.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if n := rowCount(t, f, `SELECT count(*) FROM executions WHERE target_asset_id=$1 AND state='ready'`, asset); n != 0 {
		t.Fatalf("%d tasks still ready for an offline agent", n)
	}
	if n := rowCount(t, f, `SELECT count(*) FROM executions WHERE target_asset_id=$1 AND state='ended' AND outcome='timeout'`, asset); n == 0 {
		t.Fatal("offline agent task was not closed as timeout")
	}
}

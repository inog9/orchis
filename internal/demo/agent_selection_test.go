package demo_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"orchis-go/internal/auth"
	"orchis-go/internal/chain"
	"orchis-go/internal/contract"
	"orchis-go/internal/demo"
	"orchis-go/internal/endpoint"
	"orchis-go/internal/execution"
	"orchis-go/internal/httpx"
	"orchis-go/internal/scenario"
	"orchis-go/internal/simulation"
	"orchis-go/internal/testutil"
)

// Agent sintetis tetap terlihat sebagai inventaris, tetapi tidak dapat menerima tugas nyata.
func TestDemoAgentsCannotReceiveNewEndpointWork(t *testing.T) {
	env := testutil.New(t)
	ctx := context.Background()
	addAdmin(t, env, env.TenantA)
	if err := (&demo.Service{DB: env.App}).Seed(ctx, env.TenantA, 7, nil, "", "selection"); err != nil {
		t.Fatal(err)
	}
	p := &auth.Principal{TenantID: env.TenantA}
	if err := env.Owner.QueryRow(ctx, `SELECT id FROM users WHERE tenant_id=$1 AND NOT is_demo LIMIT 1`, env.TenantA).Scan(&p.UserID); err != nil {
		t.Fatal(err)
	}
	var asset, agent uuid.UUID
	if err := env.Owner.QueryRow(ctx, `SELECT a.id,g.id FROM assets a JOIN agents g ON g.tenant_id=a.tenant_id AND g.asset_id=a.id WHERE a.tenant_id=$1 AND a.testing_authorized AND g.is_demo AND g.status<>'revoked' ORDER BY g.last_heartbeat_at DESC NULLS LAST LIMIT 1`, env.TenantA).Scan(&asset, &agent); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Owner.Exec(ctx, `UPDATE agents SET last_heartbeat_at=now() WHERE id=$1`, agent); err != nil {
		t.Fatal(err)
	}
	data, err := (&endpoint.Service{DB: env.App}).AssetData(ctx, p, asset)
	if err != nil || data.HasAgent {
		t.Fatalf("demo-only asset advertised an executable agent: %+v %v", data, err)
	}
	at := &execution.Service{DB: env.App}
	test, err := at.CreateTest(ctx, p, execution.TestInput{Name: "Real endpoint request", ContractKey: "agent.system_info", Params: json.RawMessage(`{}`), TargetAssetID: &asset}, "", "selection")
	if err != nil {
		t.Fatal(err)
	}
	before := count(t, env, `SELECT count(*) FROM executions WHERE tenant_id=$1`, env.TenantA)
	if _, err := at.Run(ctx, p, test.ID, "", "selection"); !missingAgent(err) {
		t.Fatalf("synthetic agent accepted an atomic request: %v", err)
	}
	if got := count(t, env, `SELECT count(*) FROM executions WHERE tenant_id=$1`, env.TenantA); got != before {
		t.Fatal("synthetic agent received a new atomic execution")
	}
	sc := &scenario.Service{DB: env.App}
	s, err := sc.Create(ctx, p, scenario.Input{Name: "Endpoint selection regression", Severity: "low"}, "", "selection")
	if err != nil {
		t.Fatal(err)
	}
	s, err = sc.SetTargets(ctx, p, s.ID, scenario.TargetsInput{Version: s.Version, AssetIDs: []uuid.UUID{asset}}, "", "selection")
	if err != nil {
		t.Fatal(err)
	}
	s, err = sc.SetGraph(ctx, p, s.ID, scenario.GraphInput{Version: s.Version, Steps: []scenario.StepDTO{
		{Key: "first", Title: "First", ContractKey: "agent.system_info", Params: json.RawMessage(`{}`), TriggerMode: "after_parents", IsEnabled: true},
		{Key: "next", Title: "Next", ContractKey: "agent.system_info", Params: json.RawMessage(`{}`), TriggerMode: "after_parents", IsEnabled: true},
	}, Edges: []chain.Edge{{Parent: "first", Child: "next"}}}, "", "selection")
	if err != nil {
		t.Fatal(err)
	}
	engine := &simulation.Engine{Guard: contract.Guard{AllowLoopback: true}}
	sims := &simulation.Service{DB: env.App, Engine: engine}
	if _, err := sims.Launch(ctx, p, s.ID, simulation.LaunchInput{}, "", "selection"); !missingAgent(err) {
		t.Fatalf("synthetic agent accepted scenario launch: %v", err)
	}
	// Kontrol positif: baris yang mewakili agent nyata tetap dapat dipilih.
	if _, err := env.Owner.Exec(ctx, `UPDATE agents SET is_demo=false WHERE id=$1`, agent); err != nil {
		t.Fatal(err)
	}
	if _, err := at.Run(ctx, p, test.ID, "", "selection"); err != nil {
		t.Fatal("real agent rejected:", err)
	}
	run, err := sims.Launch(ctx, p, s.ID, simulation.LaunchInput{}, "", "selection")
	if err != nil {
		t.Fatal(err)
	}
	// Uji ulang saat chain maju: agent yang kini sintetis tidak boleh menerima langkah anak.
	if _, err := env.Owner.Exec(ctx, `UPDATE agents SET is_demo=true WHERE id=$1`, agent); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Owner.Exec(ctx, `UPDATE executions SET state='ended',outcome='success',ended_at=now() WHERE tenant_id=$1 AND simulation_id=$2`, env.TenantA, run.ID); err != nil {
		t.Fatal(err)
	}
	if err := env.App.InTenant(ctx, env.TenantA, func(tx pgx.Tx) error { return engine.Advance(ctx, tx, env.TenantA, run.ID) }); err != nil {
		t.Fatal(err)
	}
	if n := count(t, env, `SELECT count(*) FROM executions e JOIN simulation_steps s ON s.tenant_id=e.tenant_id AND s.id=e.simulation_step_id WHERE e.tenant_id=$1 AND e.simulation_id=$2 AND s.key='next' AND e.state='ended' AND e.outcome='error' AND e.agent_id IS NULL AND e.error LIKE '%no agent%'`, env.TenantA, run.ID); n != 1 {
		t.Fatal("synthetic agent received a chain follow-up")
	}
}

func missingAgent(err error) bool {
	var problem *httpx.Problem
	if !errors.As(err, &problem) || problem.Status != 422 {
		return false
	}
	for _, field := range problem.Fields {
		if strings.Contains(field.Message, "has no agent") {
			return true
		}
	}
	return false
}

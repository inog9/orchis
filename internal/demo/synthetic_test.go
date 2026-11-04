package demo_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"orchis-go/internal/auth"
	"orchis-go/internal/contract"
	"orchis-go/internal/demo"
	"orchis-go/internal/execution"
	"orchis-go/internal/mitre"
	"orchis-go/internal/reporting"
	"orchis-go/internal/simulation"
	"orchis-go/internal/testutil"
)

func TestSyntheticSnapshotTotalsCoverageAndComparisonsAgree(t *testing.T) {
	env := testutil.New(t)
	ctx := context.Background()
	addAdmin(t, env, env.TenantA)
	svc := &demo.Service{DB: env.App}
	if err := svc.Seed(ctx, env.TenantA, 2, nil, "", "snapshots"); err != nil {
		t.Fatal(err)
	}
	rows, err := env.Owner.Query(ctx, `SELECT organization_id,snapshot FROM reports WHERE tenant_id=$1 AND is_demo ORDER BY organization_id,created_at`, env.TenantA)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	previous := map[uuid.UUID]reporting.Snapshot{}
	seen := 0
	for rows.Next() {
		var org uuid.UUID
		var raw []byte
		if err := rows.Scan(&org, &raw); err != nil {
			t.Fatal(err)
		}
		var snap reporting.Snapshot
		if err := json.Unmarshal(raw, &snap); err != nil {
			t.Fatal(err)
		}
		if snap.CreatedAt.After(time.Now()) {
			t.Fatal("future synthetic snapshot")
		}
		sums := map[string]int{"critical": 0, "high": 0, "medium": 0, "low": 0, "info": 0}
		for _, a := range snap.Assets {
			if a.Status == "skipped" && len(a.Findings) > 0 {
				t.Fatal("skipped asset claims current findings")
			}
			for _, f := range a.Findings {
				if f.ID == "" {
					t.Fatal("missing stable finding reference")
				}
				if f.Type == "vulnerability" {
					sums[f.Severity]++
				}
			}
			for _, m := range a.Methods {
				if _, err := contract.NormalizeVersion(m.Action, m.Version, m.Params); err != nil {
					t.Fatal(err)
				}
				if err := (contract.Guard{AllowLoopback: true}).CheckHost(contract.HostOf(m.Target)); err != nil {
					t.Fatal("non-local demo methodology", err)
				}
			}
		}
		if !reflect.DeepEqual(sums, snap.BySeverity) {
			t.Fatalf("totals differ: %+v vs %+v", sums, snap.BySeverity)
		}
		c := reporting.Coverage(snap.Assets)
		if c.Total != 10 || c.Tested != 3 || c.Skipped != 6 || c.Incomplete != 1 {
			t.Fatalf("wrong coverage: %+v", c)
		}
		if old, ok := previous[org]; ok {
			expected := "DEMO / SYNTHETIC: " + reporting.Compare(snap, old)
			if snap.Comparison != expected || !strings.Contains(expected, "Compared 6 completed target/tool checks") {
				t.Fatalf("misleading comparison: %s", snap.Comparison)
			}
		}
		if snap.ComparisonDetails == nil {
			t.Fatal("missing structured synthetic comparison")
		}
		if old, ok := previous[org]; ok {
			b := reporting.NewComparison(snap)
			b.Add(old, uuid.Nil)
			want := b.Result()
			if !reflect.DeepEqual(snap.ComparisonDetails.Totals, want.Totals) || !reflect.DeepEqual(snap.ComparisonDetails.BySeverity, want.BySeverity) {
				t.Fatal("synthetic comparison totals diverge from frozen observations")
			}
		}
		if !snap.IsDemo || !strings.Contains(snap.Label, "SYNTHETIC") {
			t.Fatal("missing provenance")
		}
		previous[org] = snap
		seen++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if seen != 15 {
		t.Fatal("missing historical snapshots")
	}
}

func TestSyntheticHistoryCannotDispatchAndIsNotMITREExecutionEvidence(t *testing.T) {
	env := testutil.New(t)
	ctx := context.Background()
	addAdmin(t, env, env.TenantA)
	svc := &demo.Service{DB: env.App, Blob: emptyArtifacts{}, Search: emptyArtifacts{}}
	if err := svc.Seed(ctx, env.TenantA, 2, nil, "", "runs"); err != nil {
		t.Fatal(err)
	}
	engine := &simulation.Engine{Guard: contract.Guard{AllowLoopback: true}}
	sims := &simulation.Service{DB: env.App, Engine: engine}
	p := &auth.Principal{TenantID: env.TenantA}
	var running, paused, exec, integ uuid.UUID
	for status, out := range map[string]*uuid.UUID{"running": &running, "paused": &paused} {
		if err := env.Owner.QueryRow(ctx, `SELECT id FROM simulations WHERE tenant_id=$1 AND is_demo AND status=$2 LIMIT 1`, env.TenantA, status).Scan(out); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := sims.Tick(ctx, env.TenantA); err != nil {
		t.Fatal(err)
	}
	if err := env.App.InTenant(ctx, env.TenantA, func(tx pgx.Tx) error { return engine.Advance(ctx, tx, env.TenantA, running) }); err != nil {
		t.Fatal(err)
	}
	if _, err := sims.Resume(ctx, p, paused, "", "runs"); err == nil {
		t.Fatal("synthetic history resumed")
	}
	if n := count(t, env, `SELECT count(*) FROM outbox WHERE tenant_id=$1`, env.TenantA); n != 0 {
		t.Fatal("synthetic dispatch queued")
	}
	if err := env.Owner.QueryRow(ctx, `SELECT id,integration_id FROM executions WHERE tenant_id=$1 AND is_demo LIMIT 1`, env.TenantA).Scan(&exec, &integ); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Owner.Exec(ctx, `UPDATE executions SET state='ready',outcome=NULL WHERE id=$1`, exec); err == nil {
		t.Fatal("demo execution became dispatchable")
	}
	if _, err := env.Owner.Exec(ctx, `UPDATE executions SET raw_output_key='unexpected-object' WHERE id=$1`, exec); err == nil {
		t.Fatal("demo history references raw output")
	}
	if _, err := (&execution.Runtime{DB: env.App, Tenant: env.TenantA}).Claim(ctx, &execution.Injector{ID: integ}, exec); err == nil {
		t.Fatal("demo execution claim accepted")
	}
	if _, err := env.Owner.Exec(ctx, `INSERT INTO mitre_tactics(id,name,shortname) VALUES('TA0007','Discovery','discovery'); INSERT INTO mitre_techniques(id,name,tactic_ids,platforms) VALUES('T1046','Network Service Discovery',ARRAY['TA0007'],ARRAY['Linux'])`); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Owner.Exec(ctx, `INSERT INTO contract_techniques(tenant_id,contract_id,technique_id) SELECT tenant_id,id,'T1046' FROM contracts WHERE tenant_id=$1 AND key='nmap.tcp_connect'`, env.TenantA); err != nil {
		t.Fatal(err)
	}
	matrix, err := (&mitre.Service{DB: env.App}).Matrix(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if matrix.Total != 1 || matrix.Mapped != 1 {
		t.Fatalf("MITRE fixture was not loaded: %+v", matrix)
	}
	if matrix.Exercised != 0 {
		t.Fatal("synthetic success counted as trained evidence")
	}
	// Idempotence preserves the entire frozen report bytes, not merely record counts.
	var before, after string
	env.Owner.QueryRow(ctx, `SELECT string_agg(snapshot::text,'' ORDER BY id) FROM reports WHERE tenant_id=$1`, env.TenantA).Scan(&before)
	if err := svc.Seed(ctx, env.TenantA, 2, nil, "", "repeat"); err != nil {
		t.Fatal(err)
	}
	env.Owner.QueryRow(ctx, `SELECT string_agg(snapshot::text,'' ORDER BY id) FROM reports WHERE tenant_id=$1`, env.TenantA).Scan(&after)
	if before != after {
		t.Fatal("same seed rewrote frozen snapshot")
	}
	if n := count(t, env, `SELECT count(*) FROM findings f WHERE f.tenant_id=$1 AND f.is_demo AND EXISTS(SELECT 1 FROM finding_occurrences o WHERE o.tenant_id=f.tenant_id AND o.finding_id=f.id AND (o.seen_at<f.first_seen_at OR o.seen_at>f.last_seen_at))`, env.TenantA); n != 0 {
		t.Fatal("occurrence outside first/last seen range")
	}
	if _, err := svc.Purge(ctx, env.TenantA, nil, "", "purge"); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"simulations", "executions", "simulation_steps", "simulation_targets", "simulation_decisions", "finding_occurrences"} {
		if n := count(t, env, `SELECT count(*) FROM `+table+` WHERE tenant_id=$1`, env.TenantA); n != 0 {
			t.Fatalf("orphan %s: %d", table, n)
		}
	}
}

func TestOldDemoFormatRequiresExplicitPurge(t *testing.T) {
	env := testutil.New(t)
	ctx := context.Background()
	addAdmin(t, env, env.TenantA)
	if _, err := env.Owner.Exec(ctx, `INSERT INTO demo_seed_state(tenant_id,seed,format_version) VALUES($1,2,1)`, env.TenantA); err != nil {
		t.Fatal(err)
	}
	svc := &demo.Service{DB: env.App}
	if err := svc.Seed(ctx, env.TenantA, 2, nil, "", "old-format"); err == nil || !strings.Contains(err.Error(), "format changed") {
		t.Fatalf("old format silently mixed: %v", err)
	}
	if n := count(t, env, `SELECT count(*) FROM organizations WHERE tenant_id=$1`, env.TenantA); n != 0 {
		t.Fatal("partial seed on version conflict")
	}
}

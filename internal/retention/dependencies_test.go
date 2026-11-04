package retention

import (
	"context"
	"github.com/google/uuid"
	"orchis-go/internal/testutil"
	"testing"
)

func TestOrganizationDeletionProtectsForeignHistoryAndDemo(t *testing.T) {
	e := testutil.New(t)
	ctx := context.Background()
	p := admin(t, e, e.TenantA)
	a, b := fixture(t, e, p), fixture(t, e, p)
	s := Service{DB: e.App}
	must := func(q string, args ...any) {
		t.Helper()
		if _, err := e.Owner.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	reject := func(reason string) {
		t.Helper()
		v, err := s.Preview(ctx, p, a.org)
		if err != nil || len(v.Blockers) == 0 {
			t.Fatal("missing deletion blocker", reason, err)
		}
		_, err = s.DeleteOrganization(ctx, p, a.org, DeleteInput{Name: v.Name, Version: v.Version, Counts: v.Counts}, "", "fixture")
		problem(t, err, 409)
		var n int
		if err = e.Owner.QueryRow(ctx, `SELECT count(*) FROM organizations WHERE id=ANY($1::uuid[])`, []uuid.UUID{a.org, b.org}).Scan(&n); err != nil || n != 2 {
			t.Fatal("blocked deletion changed organisations", err)
		}
	}
	must(`INSERT INTO assessment_targets(tenant_id,assessment_id,asset_id,asset_name,addresses) VALUES($1,$2,$3,'Moved asset','[]')`, p.TenantID, b.assessment, a.asset)
	reject("foreign assessment history")
	must(`DELETE FROM assessment_targets WHERE tenant_id=$1 AND assessment_id=$2`, p.TenantID, b.assessment)
	rangeID := uuid.New()
	must(`INSERT INTO network_range_authorizations(id,tenant_id,organization_id,agent_id,control_asset_id,cidr,consent_revision,authorized_by) VALUES($1,$2,$3,$4,$5,'127.0.0.0/30',1,$6)`, rangeID, p.TenantID, b.org, a.agent, a.asset, p.UserID)
	reject("foreign range control")
	must(`DELETE FROM network_range_authorizations WHERE id=$1`, rangeID)
	must(`UPDATE reports SET snapshot=jsonb_build_object('assets',jsonb_build_array(jsonb_build_object('id',$2::uuid))) WHERE id=$1`, b.report, a.asset)
	reject("foreign frozen report remains after raw results expired")
	must(`UPDATE reports SET snapshot='{}' WHERE id=$1`, b.report)
	must(`UPDATE organizations SET is_demo=true WHERE id=$1`, a.org)
	reject("demo uses explicit purge")
}
func TestOrganizationDeletionKeepsSharedChainAndOtherTargets(t *testing.T) {
	e := testutil.New(t)
	ctx := context.Background()
	p := admin(t, e, e.TenantA)
	a, b := fixture(t, e, p), fixture(t, e, p)
	s := Service{DB: e.App}
	sim, step := uuid.New(), uuid.New()
	must := func(q string, args ...any) {
		t.Helper()
		if _, err := e.Owner.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	must(`INSERT INTO simulations(id,tenant_id,name,status) VALUES($1,$2,'Shared chain','finished')`, sim, p.TenantID)
	must(`INSERT INTO simulation_targets(tenant_id,simulation_id,asset_id,scan_organization_id) VALUES($1,$2,$3,$5),($1,$2,$4,$6)`, p.TenantID, sim, a.asset, b.asset, a.org, b.org)
	must(`INSERT INTO simulation_steps(id,tenant_id,simulation_id,key,title,contract_id,contract_version,trigger_mode) SELECT $1,$2,$3,'shared','Shared check',contract_id,contract_version,'after_parents' FROM atomic_tests WHERE tenant_id=$2 AND id=$4`, step, p.TenantID, sim, a.atomic)
	for _, f := range []orgFixture{a, b} {
		must(`UPDATE executions SET atomic_test_id=NULL,simulation_id=$2,simulation_step_id=$3 WHERE id=$1`, f.execution, sim, step)
		must(`INSERT INTO simulation_decisions(tenant_id,simulation_id,step_id,asset_id,status,reason) VALUES($1,$2,$3,$4,'finished','fixture')`, p.TenantID, sim, step, f.asset)
	}
	v, err := s.Preview(ctx, p, a.org)
	if err != nil || len(v.Blockers) > 0 {
		t.Fatal("shared chain should remove only selected target", v, err)
	}
	if _, err = s.DeleteOrganization(ctx, p, a.org, DeleteInput{Name: v.Name, Version: v.Version, Counts: v.Counts}, "", "shared"); err != nil {
		t.Fatal(err)
	}
	var sims, targets, decisions, executions int
	if err = e.Owner.QueryRow(ctx, `SELECT (SELECT count(*) FROM simulations WHERE id=$1),(SELECT count(*) FROM simulation_targets WHERE simulation_id=$1 AND asset_id=$2),(SELECT count(*) FROM simulation_decisions WHERE simulation_id=$1 AND asset_id=$2),(SELECT count(*) FROM executions WHERE id=$3)`, sim, b.asset, b.execution).Scan(&sims, &targets, &decisions, &executions); err != nil || sims != 1 || targets != 1 || decisions != 1 || executions != 1 {
		t.Fatalf("shared chain was cascaded: %d %d %d %d %v", sims, targets, decisions, executions, err)
	}
	var target *uuid.UUID
	if err = e.Owner.QueryRow(ctx, `SELECT target_asset_id FROM atomic_tests WHERE id=$1`, a.atomic).Scan(&target); err != nil || target != nil {
		t.Fatal("reusable atomic configuration removed instead of detached", err)
	}
}

package demo

import (
	"context"
	"encoding/json"
	"fmt"
	"orchis-go/internal/chain"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type inertResult struct {
	outcome, target string
	findings        []uuid.UUID
}

// Frozen illustrative history only: no queue entries, leases, claims, credentials,
// raw objects or dispatch messages. The database also enforces terminal executions.
func (g *generator) seedInertScenario(ctx context.Context, tx pgx.Tx, id uuid.UUID, name, status string, si int, asset uuid.UUID, when time.Time, results map[string]inertResult) error {
	scenario := g.id("scenario", fmt.Sprint(si))
	var ended *time.Time
	if status == "finished" || status == "failed" || status == "canceled" {
		at := when.Add(2 * time.Minute)
		ended = &at
	}
	if _, err := tx.Exec(ctx, `INSERT INTO simulations(id,tenant_id,scenario_id,name,status,safe_mode,launched_by,created_at,started_at,ended_at,is_demo) VALUES($1,$2,$3,$4,$5,true,$6,$7,$7,$8,true) ON CONFLICT(id) DO NOTHING`, id, g.tenant, scenario, name, status, g.createdBy, when, ended); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO simulation_targets(tenant_id,simulation_id,asset_id,addresses) VALUES($1,$2,$3,'[{"kind":"ip","value":"127.0.0.1","is_primary":true}]') ON CONFLICT DO NOTHING`, g.tenant, id, asset); err != nil {
		return err
	}
	xs := []chain.Exec{}
	fs := []chain.Found{}
	for _, st := range demoScenarios[si].steps {
		step := g.id("inert-step", id.String(), st.key)
		if _, err := tx.Exec(ctx, `INSERT INTO simulation_steps(id,tenant_id,simulation_id,key,title,contract_id,contract_version,params,trigger_mode,is_enabled,position_x,position_y)
SELECT $1,$2,$3,key,title,contract_id,contract_version,params,trigger_mode,is_enabled,position_x,position_y FROM scenario_steps WHERE tenant_id=$2 AND scenario_id=$4 AND key=$5 ON CONFLICT(id) DO NOTHING`, step, g.tenant, id, scenario, st.key); err != nil {
			return err
		}
		result, ok := results[st.key]
		if !ok {
			continue
		}
		target := result.target
		if target == "" {
			target = "127.0.0.1"
		}
		exec := g.id("inert-execution", id.String(), st.key)
		errText := ""
		if result.outcome != "success" {
			errText = "Demo/synthetic failure; no process was run."
		}
		xs = append(xs, chain.Exec{Step: st.key, State: "ended", Outcome: result.outcome, Asset: asset.String()})
		if _, err := tx.Exec(ctx, `INSERT INTO executions(id,tenant_id,simulation_id,simulation_step_id,contract_id,contract_version,target_asset_id,target_address,executor_kind,integration_id,state,outcome,params,input_hash,created_at,started_at,ended_at,error,is_demo)
SELECT $1,$2,$3,s.id,s.contract_id,s.contract_version,$5,$6,CASE WHEN c.needs_agent THEN 'agent' ELSE 'injector' END,c.integration_id,'ended',$7,s.params,$8,$9,$9,$9::timestamptz+interval '1 minute',$10,true
FROM simulation_steps s JOIN contracts c ON c.tenant_id=s.tenant_id AND c.id=s.contract_id WHERE s.tenant_id=$2 AND s.id=$4 ON CONFLICT(id) DO NOTHING`, exec, g.tenant, id, step, asset, target, result.outcome, hash(exec.String()), when, errText); err != nil {
			return err
		}
		for _, finding := range result.findings {
			var found chain.Found
			found.ID = finding.String()
			found.Step = st.key
			found.Asset = asset.String()
			found.Target = target
			if err := tx.QueryRow(ctx, `SELECT type,value,severity FROM findings WHERE tenant_id=$1 AND id=$2 AND asset_id=$3 AND is_demo`, g.tenant, finding, asset).Scan(&found.Type, &found.Value, &found.Severity); err != nil {
				return err
			}
			fs = append(fs, found)
			if _, err := tx.Exec(ctx, `INSERT INTO finding_occurrences(id,tenant_id,finding_id,execution_id,simulation_id,seen_at,details) VALUES($1,$2,$3,$4,$5,$6,'{"is_demo":true,"label":"Synthetic occurrence; no check was executed"}') ON CONFLICT DO NOTHING`, g.id("inert-occurrence", exec.String(), finding.String()), g.tenant, finding, exec, id, when); err != nil {
				return err
			}
		}
	}
	for _, edge := range demoEdges(si) {
		rule, err := json.Marshal(edge)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO simulation_step_edges(tenant_id,simulation_id,parent_step_id,child_step_id,rule) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, g.tenant, id, g.id("inert-step", id.String(), edge.Parent), g.id("inert-step", id.String(), edge.Child), rule); err != nil {
			return err
		}
	}

	steps := []chain.Step{}
	for _, st := range demoScenarios[si].steps {
		steps = append(steps, chain.Step{Key: st.key, Enabled: true, Mode: st.trigger})
	}
	decision := chain.Evaluate(steps, demoEdges(si), xs, fs)
	for key, state := range decision.Status {
		reason := decision.Reasons[key]
		if reason == "" {
			reason = "Illustrative " + state + " state; no worker was called."
		}
		if state == chain.StEligible {
			state = chain.StWaiting
			reason = "Eligible in synthetic graph only; dispatch is disabled."
		}
		if _, err := tx.Exec(ctx, `INSERT INTO simulation_decisions(tenant_id,simulation_id,step_id,asset_id,status,reason,preview) VALUES($1,$2,$3,$4,$5,$6,'{"synthetic":true,"never_dispatched":true}') ON CONFLICT DO NOTHING`, g.tenant, id, g.id("inert-step", id.String(), key), asset, state, "Demo/synthetic: "+reason); err != nil {
			return err
		}
	}
	return nil
}

func (g *generator) seedRunStates(ctx context.Context, tx pgx.Tx) error {
	asset := g.id("org", "0", "asset", "0")
	portFinding := g.id("org", "0", "asset", "0", "finding", "0")
	cases := []struct {
		name, status string
		scenario     int
		results      map[string]inertResult
	}{
		{"matched branch", "finished", 0, map[string]inertResult{"discover": {"success", "127.0.0.1", []uuid.UUID{portFinding}}, "scan": {"success", "http://127.0.0.1:80", nil}}},
		{"else branch", "finished", 0, map[string]inertResult{"discover": {"success", "127.0.0.1", nil}, "no-web": {"success", "127.0.0.1", nil}}},
		{"failure branch", "finished", 0, map[string]inertResult{"discover": {"error", "127.0.0.1", nil}, "failure": {"success", "127.0.0.1", nil}}},
		{"failed parent and blocked child", "failed", 1, map[string]inertResult{"info": {"error", "local", nil}}},
		{"canceled", "canceled", 0, map[string]inertResult{"discover": {"canceled", "127.0.0.1", nil}}},
		{"paused", "paused", 0, nil}, {"running / waiting", "running", 0, nil},
		{"scheduled / waiting", "scheduled", 0, nil}, {"draft / waiting", "draft", 2, nil},
	}
	for i, item := range cases {
		when := g.now.Add(-time.Duration(10+i) * time.Minute)
		if err := g.seedInertScenario(ctx, tx, g.id("run-state", fmt.Sprint(i)), "Demo: synthetic "+item.name, item.status, item.scenario, asset, when, item.results); err != nil {
			return err
		}
	}
	return nil
}

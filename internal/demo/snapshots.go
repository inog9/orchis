package demo

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"orchis-go/internal/osquery"
	"orchis-go/internal/reporting"
)

// Synthetic snapshots use the same coverage and comparison rules as real reports.
// Totals are derived from the included findings, never scaled independently.
func (g *generator) seedAssessmentsAndReports(ctx context.Context, tx pgx.Tx, oi int, orgID uuid.UUID, orgName string, assets []assetInfo, _ map[string]int) error {
	var previous *reporting.Snapshot
	for ri, days := range []int{90, 30, 0} {
		assess := g.id("org", fmt.Sprint(oi), "assessment", fmt.Sprint(ri))
		created := g.now.AddDate(0, 0, -days).Add(-25 * time.Minute)
		ended := created.Add(25 * time.Minute)
		idem := fmt.Sprintf("demo-seed-%d-org-%d-run-%d", g.seed, oi, ri)
		if _, err := tx.Exec(ctx, `INSERT INTO assessments(id,tenant_id,organization_id,name,profile,idempotency_key,request_hash,status,created_by,created_at,ended_at,is_demo)
VALUES($1,$2,$3,$4,'quick',$5,$6,'finished',$7,$8,$9,true) ON CONFLICT(id) DO NOTHING`, assess, g.tenant, orgID, fmt.Sprintf("%s synthetic run %d", orgName, ri+1), idem, hex.EncodeToString(hash(idem)), g.createdBy, created, ended); err != nil {
			return err
		}
		snap := reporting.Snapshot{IsDemo: true, Label: "DEMO / SYNTHETIC - not a real assessment. Do not use for real security decisions.", Organization: orgName, AssessmentID: assess, Profile: "quick", CreatedAt: created, Status: "finished", Risk: "Synthetic demo data - risk not evaluated.", BySeverity: map[string]int{"critical": 0, "high": 0, "medium": 0, "low": 0, "info": 0}, Assets: []reporting.Asset{}, Remediation: "Synthetic example only. Verify actual observations before any remediation.", Comparison: "Comparison unavailable: first synthetic snapshot."}
		for ai, a := range assets {
			entry := reporting.Asset{ID: a.id, Name: a.name, Status: "skipped", SkipReason: "Synthetic snapshot: testing was not authorized.", Findings: []reporting.Finding{}, Methods: []reporting.Method{}}
			if assetBlueprints[ai].addr == "" {
				entry.SkipReason = "Synthetic snapshot: asset has no configured address."
			}
			var simulationID *uuid.UUID
			if ai == 0 || ai == 2 || ai == 4 || ai == 1 {
				entry.Status = "finished"
				entry.SkipReason = ""
				result := map[string]inertResult{"discover": {outcome: "success", target: "127.0.0.1"}, "scan": {outcome: "success", target: "http://127.0.0.1:80"}}
				for fi, f := range a.findings {
					if f.Type != "open_port" && ri == 0 && fi == 1 {
						continue
					}
					if f.Type != "open_port" && ri == 2 && fi > 0 && (ai%2 == 0 || fi > 1) {
						continue
					}
					if ai == 1 && f.Type == "vulnerability" {
						continue
					} // failed vulnerability scan supplies no invented result
					entry.Findings = append(entry.Findings, f)
					key := "discover"
					if f.Type == "vulnerability" {
						key = "scan"
						snap.BySeverity[f.Severity]++
					}
					item := result[key]
					item.findings = append(item.findings, uuid.MustParse(f.ID))
					result[key] = item
				}
				begin, finish := created, created.Add(time.Minute)
				entry.Methods = []reporting.Method{
					{Action: "nmap.tcp_connect", Version: 1, Params: json.RawMessage(`{"profile":"web","service_detection":false}`), Target: "127.0.0.1", Outcome: "success", MITRE: []string{"T1046"}, StartedAt: &begin, EndedAt: &finish},
					{Action: "nuclei.scan", Version: 1, Params: json.RawMessage(`{"severity":["info","low","medium","high","critical"],"tags":[]}`), Target: "http://127.0.0.1:80", Outcome: "success", MITRE: []string{"T1595.002"}, StartedAt: &begin, EndedAt: &finish},
				}
				for i := range entry.Findings {
					method := entry.Methods[0]
					if entry.Findings[i].Type == "vulnerability" {
						method = entry.Methods[1]
					}
					entry.Findings[i].Coverage = []string{reporting.CoverageKey(method)}
				}
				if ai == 1 {
					entry.Status = "failed"
					entry.Methods[1].Outcome = "error"
					entry.Methods[1].Error = "Synthetic local scanner connection failure; this does not establish safety."
					item := result["scan"]
					item.outcome = "error"
					result["scan"] = item
				}
				sim := g.id("org", fmt.Sprint(oi), "assessment-simulation", fmt.Sprint(ri), fmt.Sprint(ai))
				simulationID = &sim
				if err := g.seedInertScenario(ctx, tx, sim, fmt.Sprintf("Demo: synthetic %s / run %d", a.name, ri+1), entry.Status, 0, a.id, created, result); err != nil {
					return err
				}
				// Ensure the frozen per-execution params equal the report methodology.
				for key, index := range map[string]int{"discover": 0, "scan": 1} {
					if _, err := tx.Exec(ctx, `UPDATE executions SET params=$3 WHERE tenant_id=$1 AND id=$2 AND is_demo`, g.tenant, g.id("inert-execution", sim.String(), key), entry.Methods[index].Params); err != nil {
						return err
					}
				}
			}
			if ai == 0 {
				entry.Endpoint = osquery.BuildPosture(syntheticEndpointSets(oi), []int{80})
				if entry.Endpoint != nil {
					entry.Endpoint.Source = "osquery (demo, synthetic)"
					by := map[string]int{"critical": 0, "high": 0, "medium": 0, "low": 0, "info": 0}
					for _, f := range entry.Endpoint.Findings {
						by[f.Severity]++
					}
					snap.EndpointSummary = &reporting.EndpointSummary{AssetsTotal: len(assets), AssetsWithData: 1, BySeverity: by}
				}
			}
			var addresses json.RawMessage
			if err := tx.QueryRow(ctx, `SELECT COALESCE(jsonb_agg(jsonb_build_object('kind',kind,'value',value,'is_primary',is_primary) ORDER BY kind,value),'[]') FROM asset_addresses WHERE tenant_id=$1 AND asset_id=$2`, g.tenant, a.id).Scan(&addresses); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO assessment_targets(tenant_id,assessment_id,asset_id,asset_name,addresses,simulation_id,skip_reason) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING`, g.tenant, assess, a.id, a.name, addresses, simulationID, entry.SkipReason); err != nil {
				return err
			}
			snap.Assets = append(snap.Assets, entry)
		}
		comparison := reporting.NewComparison(snap)
		if previous != nil {
			comparison.Add(*previous, g.id("org", fmt.Sprint(oi), "report", fmt.Sprint(ri-1)))
		}
		details := comparison.Result()
		snap.ComparisonDetails = &details
		snap.Comparison = "DEMO / SYNTHETIC: " + details.Summary()
		raw, err := json.Marshal(snap)
		if err != nil {
			return err
		}
		status, failure, attempts := "queued", "", 0
		if ri == 1 {
			status, failure, attempts = "failed", "Renderer unreachable (synthetic retry example).", 2
		}
		if _, err := tx.Exec(ctx, `INSERT INTO reports(id,tenant_id,organization_id,assessment_id,version,snapshot,status,error,attempts,created_at,updated_at,is_demo) VALUES($1,$2,$3,$4,1,$5,$6,$7,$8,$9,$9,true) ON CONFLICT(id) DO NOTHING`, g.id("org", fmt.Sprint(oi), "report", fmt.Sprint(ri)), g.tenant, orgID, assess, raw, status, failure, attempts, ended); err != nil {
			return err
		}
		previous = &snap
	}
	return nil
}

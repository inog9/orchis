package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"orchis-go/internal/audit"
	"orchis-go/internal/contract"
	"orchis-go/internal/hackbot"
	"orchis-go/internal/httpx"
	"orchis-go/internal/ids"
	"orchis-go/internal/scancontrol"
)

// RecoverHackbot creates a separate, explicitly incomplete historical record.
// Only the injector that owned the original failed execution can recover it.
// No target is contacted, and the original execution and report remain frozen.
func (r *Runtime) RecoverHackbot(ctx context.Context, in *Injector, original uuid.UUID, res hackbot.Result) (uuid.UUID, error) {
	if in.AgentID != nil || res.SessionID != original.String() || res.Phase != "report" {
		return uuid.Nil, httpx.BadRequest("Invalid recovery identity.")
	}
	if err := res.Validate(); err != nil {
		return uuid.Nil, httpx.BadRequest(err.Error())
	}
	if res.Disposition != "complete" && res.Disposition != "limited" && res.Disposition != "input_required" {
		return uuid.Nil, httpx.BadRequest("A completed evidence review is required.")
	}
	// A recovered record is not a new scan and cannot establish full coverage.
	res.Disposition = "limited"
	res.Blockers = append(res.Blockers, "Recovered from an earlier incomplete scan; no new scan was performed.")
	if len(res.Blockers) > 100 {
		return uuid.Nil, httpx.BadRequest("Too many coverage limitations.")
	}
	payload, err := json.Marshal(res)
	if err != nil {
		return uuid.Nil, err
	}
	var recovered uuid.UUID
	err = r.DB.InTenant(ctx, r.Tenant, func(tx pgx.Tx) error {
		if err := scancontrol.Lock(ctx, tx, r.Tenant); err != nil {
			return err
		}
		var oldScan, asset uuid.UUID
		var oldStatus, oldTool, originalState, originalOutcome string
		err := tx.QueryRow(ctx, `SELECT a.id,j.control_asset_id,a.status,j.tool,e.state,COALESCE(e.outcome,'')
FROM executions e JOIN scan_jobs j ON j.tenant_id=e.tenant_id AND j.simulation_id=e.simulation_id
JOIN assessments a ON a.tenant_id=j.tenant_id AND a.id=j.assessment_id
JOIN contracts c ON c.tenant_id=e.tenant_id AND c.id=e.contract_id
WHERE e.tenant_id=$1 AND e.id=$2 AND e.integration_id=$3 AND c.key=$4 AND a.is_scan
  AND a.results_expired_at IS NULL AND a.report_expired_at IS NULL FOR UPDATE OF a,e`, r.Tenant, original, in.ID, contract.KeyHackbot).Scan(&oldScan, &asset, &oldStatus, &oldTool, &originalState, &originalOutcome)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound("Execution")
		}
		if err != nil {
			return err
		}
		if oldTool != "hackbot" || (oldStatus != "failed" && oldStatus != "incomplete") || originalState != "ended" || (originalOutcome != "error" && originalOutcome != "timeout") {
			return httpx.Conflict("Only an unsuccessful, finished Hackbot scan can be recovered.")
		}
		key := "hackbot-recovery:" + original.String()
		err = tx.QueryRow(ctx, `SELECT id FROM assessments WHERE tenant_id=$1 AND idempotency_key=$2`, r.Tenant, key).Scan(&recovered)
		if err == nil {
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		// Restrict recovery to a single target: it must not claim unrelated
		// targets were rechecked or silently omit their jobs.
		var targets int
		if err = tx.QueryRow(ctx, `SELECT count(DISTINCT control_asset_id) FROM scan_jobs WHERE tenant_id=$1 AND assessment_id=$2`, r.Tenant, oldScan).Scan(&targets); err != nil {
			return err
		}
		if targets != 1 {
			return httpx.Conflict("Recovery currently requires a single-target scan.")
		}
		recovered = ids.New()
		tag, err := tx.Exec(ctx, `INSERT INTO assessments(id,tenant_id,organization_id,name,profile,idempotency_key,request_hash,status,created_by,ended_at,is_scan,runner,scan_options,report_language)
SELECT $3,tenant_id,organization_id,name || ' (recovered)',profile,$4,$4,'incomplete',created_by,now(),true,runner,scan_options,report_language
FROM assessments WHERE tenant_id=$1 AND id=$2`, r.Tenant, oldScan, recovered, key)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return httpx.NotFound("Scan")
		}
		type job struct {
			id, sim, execution, step, target uuid.UUID
			tool                             string
			version                          int
		}
		rows, err := tx.Query(ctx, `SELECT j.id,j.simulation_id,e.id,e.simulation_step_id,e.target_asset_id,j.tool,e.contract_version FROM scan_jobs j
JOIN executions e ON e.tenant_id=j.tenant_id AND e.simulation_id=j.simulation_id
WHERE j.tenant_id=$1 AND j.assessment_id=$2 ORDER BY j.id,e.id`, r.Tenant, oldScan)
		if err != nil {
			return err
		}
		var jobs []job
		for rows.Next() {
			var j job
			if err = rows.Scan(&j.id, &j.sim, &j.execution, &j.step, &j.target, &j.tool, &j.version); err != nil {
				rows.Close()
				return err
			}
			jobs = append(jobs, j)
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return err
		}
		var total int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM scan_jobs WHERE tenant_id=$1 AND assessment_id=$2`, r.Tenant, oldScan).Scan(&total); err != nil {
			return err
		}
		if total != len(jobs) {
			return httpx.Conflict("Recovery requires exactly one completed execution per original job.")
		}
		found := false
		for _, j := range jobs {
			sim, step, execution, jobID := ids.New(), ids.New(), ids.New(), ids.New()
			if _, err = tx.Exec(ctx, `INSERT INTO simulations(id,tenant_id,name,status,started_at,ended_at,timeout_seconds,safe_mode,launched_by)
SELECT $3,tenant_id,name || ' (recovered)',CASE WHEN status='finished' THEN 'finished' ELSE 'failed' END,started_at,ended_at,timeout_seconds,safe_mode,launched_by FROM simulations WHERE tenant_id=$1 AND id=$2`, r.Tenant, j.sim, sim); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `INSERT INTO simulation_steps(id,tenant_id,simulation_id,key,title,contract_id,contract_version,params,delay_seconds,trigger_mode,trigger,mappings,is_enabled,position_x,position_y)
SELECT $3,tenant_id,$4,key,title,contract_id,contract_version,params,delay_seconds,trigger_mode,trigger,mappings,is_enabled,position_x,position_y FROM simulation_steps WHERE tenant_id=$1 AND id=$2`, r.Tenant, j.step, step, sim); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `INSERT INTO simulation_targets(tenant_id,simulation_id,asset_id,addresses) SELECT tenant_id,$3,asset_id,addresses FROM simulation_targets WHERE tenant_id=$1 AND simulation_id=$2`, r.Tenant, j.sim, sim); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `INSERT INTO executions(id,tenant_id,simulation_id,simulation_step_id,contract_id,contract_version,target_asset_id,target_address,executor_kind,integration_id,agent_id,state,outcome,params,input_hash,started_at,ended_at,exit_code,error,hackbot_result)
SELECT $3,tenant_id,$4,$5,contract_id,contract_version,target_asset_id,target_address,executor_kind,integration_id,agent_id,'ended',CASE WHEN id=$6 THEN 'error' ELSE outcome END,params,input_hash,started_at,ended_at,exit_code,CASE WHEN id=$6 THEN 'Recovered report from previous scan; no new testing was performed.' ELSE error END,CASE WHEN id=$6 THEN $7::jsonb ELSE hackbot_result END FROM executions WHERE tenant_id=$1 AND id=$2`, r.Tenant, j.execution, execution, sim, step, original, payload); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `INSERT INTO scan_jobs(id,tenant_id,assessment_id,control_asset_id,asset_id,range_id,range_version,name,target,addresses,tool,contract_id,contract_version,params,scope,simulation_id,status,error,ended_at)
SELECT $3,tenant_id,$4,control_asset_id,asset_id,range_id,range_version,name,target,addresses,tool,contract_id,contract_version,params,scope,$5,CASE WHEN tool='hackbot' THEN 'incomplete' ELSE status END,CASE WHEN tool='hackbot' THEN 'Recovered report from previous scan; no new testing was performed.' ELSE error END,ended_at FROM scan_jobs WHERE tenant_id=$1 AND id=$2`, r.Tenant, j.id, jobID, recovered, sim); err != nil {
				return err
			}
			if j.execution == original {
				found = true
				fs := make([]NewFinding, 0, len(res.Findings))
				for _, f := range res.Findings {
					fs = append(fs, NewFinding{Type: "vulnerability", Value: f.Value, Title: f.Title, Severity: f.Severity, Details: f.Details, Identity: f.Title})
				}
				def := contract.FindVersion(contract.KeyHackbot, j.version)
				if def == nil {
					return fmt.Errorf("Hackbot contract missing")
				}
				n, rejected, err := ingest(ctx, tx, r.Tenant, execution, j.target, &sim, def.OutputTypes, fs)
				if err != nil {
					return err
				}
				if n != len(fs) || len(rejected) > 0 {
					return httpx.Conflict("Recovered findings could not all be represented without duplication.")
				}
			} else {
				if j.tool == "hackbot" {
					return httpx.Conflict("Recovery requires exactly one Hackbot execution.")
				}
				if _, err = tx.Exec(ctx, `INSERT INTO finding_occurrences(id,tenant_id,finding_id,execution_id,simulation_id,seen_at,evidence_key,details,finding_snapshot)
SELECT gen_random_uuid(),tenant_id,finding_id,$3,$4,seen_at,evidence_key,details,finding_snapshot FROM finding_occurrences WHERE tenant_id=$1 AND execution_id=$2`, r.Tenant, j.execution, execution, sim); err != nil {
					return err
				}
			}
		}
		if !found {
			return httpx.Conflict("Original Hackbot job was not found.")
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: r.Tenant, Action: "scan.hackbot_recovery", ResourceType: "scan", ResourceID: &recovered, Detail: map[string]any{"source_scan_id": oldScan, "source_execution_id": original, "integration_id": in.ID, "findings": len(res.Findings)}})
	})
	return recovered, err
}

package retention

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"orchis-go/internal/scancontrol"
	"time"
)

// Sweep removes a bounded batch in one transaction. Frozen reports have their own
// lifetime; scan metadata survives only while a retained report needs its FK.
func (s *Service) Sweep(ctx context.Context, tenant uuid.UUID) error {
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}
	run, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	return s.DB.InTenant(run, tenant, func(tx pgx.Tx) error {
		if err := scancontrol.Lock(run, tx, tenant); err != nil {
			return err
		}
		if _, err := tx.Exec(run, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "retention:"+tenant.String()); err != nil {
			return err
		}
		p, err := readPolicy(run, tx, tenant)
		if err != nil {
			return err
		}
		reports, err := queryIDs(run, tx, `SELECT id FROM reports WHERE tenant_id=$1 AND NOT is_demo AND status<>'rendering' AND created_at<$2 ORDER BY id LIMIT 64 FOR UPDATE SKIP LOCKED`, tenant, cutoff(now, p.ReportDays))
		if err != nil {
			return err
		}
		if err = removeMetadata(run, tx, tenant, map[string][]uuid.UUID{"report": reports}); err != nil {
			return err
		}
		if err = queueReports(run, tx, tenant, reports, nil); err != nil {
			return err
		}
		for _, q := range []string{
			`DELETE FROM ai_job_actions WHERE tenant_id=$1 AND job_id IN(SELECT id FROM ai_jobs WHERE tenant_id=$1 AND report_id=ANY($2::uuid[]))`,
			`DELETE FROM ai_jobs WHERE tenant_id=$1 AND report_id=ANY($2::uuid[])`,
			`UPDATE assessments SET report_expired_at=$3 WHERE tenant_id=$1 AND id IN(SELECT assessment_id FROM reports WHERE tenant_id=$1 AND id=ANY($2::uuid[]))`,
			`DELETE FROM reports WHERE tenant_id=$1 AND id=ANY($2::uuid[])`,
		} {
			args := []any{tenant, reports}
			if q[0:6] == "UPDATE" {
				args = append(args, now)
			}
			if _, err = tx.Exec(run, q, args...); err != nil {
				return err
			}
		}
		scans, err := queryIDs(run, tx, `SELECT a.id FROM assessments a WHERE a.tenant_id=$1 AND NOT a.is_demo AND a.status<>'running' AND a.results_expired_at IS NULL AND COALESCE(a.ended_at,a.created_at)<$2 AND (a.report_expired_at IS NOT NULL OR EXISTS(SELECT 1 FROM reports r WHERE r.tenant_id=a.tenant_id AND r.assessment_id=a.id)) AND NOT EXISTS(SELECT 1 FROM scan_jobs j WHERE j.tenant_id=a.tenant_id AND j.assessment_id=a.id AND j.status IN('waiting','running')) AND NOT EXISTS(SELECT 1 FROM simulations s WHERE s.tenant_id=a.tenant_id AND s.status NOT IN('finished','canceled','failed') AND s.id IN(SELECT simulation_id FROM assessment_targets t WHERE t.tenant_id=a.tenant_id AND t.assessment_id=a.id UNION SELECT simulation_id FROM scan_jobs j WHERE j.tenant_id=a.tenant_id AND j.assessment_id=a.id)) ORDER BY a.id LIMIT 32 FOR UPDATE OF a SKIP LOCKED`, tenant, cutoff(now, p.ScanDays))
		if err != nil {
			return err
		}
		for _, q := range []string{
			`DELETE FROM scan_jobs WHERE tenant_id=$1 AND assessment_id=ANY($2::uuid[])`,
			`DELETE FROM assessment_targets WHERE tenant_id=$1 AND assessment_id=ANY($2::uuid[])`,
			`UPDATE assessments SET results_expired_at=$3,scan_options=(scan_options-'asset_ids'-'range_ids')||'{"asset_ids":[],"range_ids":[]}'::jsonb WHERE tenant_id=$1 AND id=ANY($2::uuid[])`,
		} {
			args := []any{tenant, scans}
			if q[0:6] == "UPDATE" {
				args = append(args, now)
			}
			if _, err = tx.Exec(run, q, args...); err != nil {
				return err
			}
		}
		// References from a retained or active assessment pin a shared simulation.
		sims, err := queryIDs(run, tx, `SELECT s.id FROM simulations s WHERE s.tenant_id=$1 AND NOT s.is_demo AND s.status IN('finished','canceled','failed') AND COALESCE(s.ended_at,s.created_at)<$2 AND NOT EXISTS(SELECT 1 FROM assessment_targets t WHERE t.tenant_id=s.tenant_id AND t.simulation_id=s.id) AND NOT EXISTS(SELECT 1 FROM scan_jobs j WHERE j.tenant_id=s.tenant_id AND j.simulation_id=s.id) AND NOT EXISTS(SELECT 1 FROM executions e WHERE e.tenant_id=s.tenant_id AND e.simulation_id=s.id AND e.state<>'ended') ORDER BY s.id LIMIT 64 FOR UPDATE OF s SKIP LOCKED`, tenant, cutoff(now, p.ScanDays))
		if err != nil {
			return err
		}
		executions, err := queryIDs(run, tx, `SELECT e.id FROM executions e WHERE e.tenant_id=$1 AND NOT e.is_demo AND e.state='ended' AND (e.simulation_id=ANY($2::uuid[]) OR (e.simulation_id IS NULL AND e.ended_at<$3)) ORDER BY e.id LIMIT 512 FOR UPDATE OF e SKIP LOCKED`, tenant, sims, cutoff(now, p.ScanDays))
		if err != nil {
			return err
		}
		// Do not cascade a simulation until every execution has a durable blob intent.
		if err = removeMetadata(run, tx, tenant, map[string][]uuid.UUID{"execution": executions}); err != nil {
			return err
		}
		if err = queueExecutions(run, tx, tenant, executions, nil); err != nil {
			return err
		}
		for _, q := range []string{`UPDATE ai_job_actions SET execution_id=NULL WHERE tenant_id=$1 AND execution_id=ANY($2::uuid[])`, `DELETE FROM executions WHERE tenant_id=$1 AND id=ANY($2::uuid[])`} {
			if _, err = tx.Exec(run, q, tenant, executions); err != nil {
				return err
			}
		}
		emptySims, err := queryIDs(run, tx, `SELECT s.id FROM simulations s WHERE tenant_id=$1 AND id=ANY($2::uuid[]) AND NOT EXISTS(SELECT 1 FROM executions e WHERE e.tenant_id=s.tenant_id AND e.simulation_id=s.id)`, tenant, sims)
		if err != nil {
			return err
		}
		if err = removeMetadata(run, tx, tenant, map[string][]uuid.UUID{"simulation": emptySims}); err != nil {
			return err
		}
		if _, err = tx.Exec(run, `DELETE FROM simulation_decisions d WHERE tenant_id=$1 AND simulation_id=ANY($2::uuid[]) AND NOT EXISTS(SELECT 1 FROM executions e WHERE e.tenant_id=d.tenant_id AND e.simulation_id=d.simulation_id)`, tenant, sims); err != nil {
			return err
		}
		if _, err = tx.Exec(run, `DELETE FROM simulations s WHERE tenant_id=$1 AND id=ANY($2::uuid[]) AND NOT EXISTS(SELECT 1 FROM executions e WHERE e.tenant_id=s.tenant_id AND e.simulation_id=s.id)`, tenant, sims); err != nil {
			return err
		}
		findings, err := queryIDs(run, tx, `SELECT id FROM findings WHERE tenant_id=$1 AND NOT is_demo AND last_seen_at<$2 ORDER BY id LIMIT 256 FOR UPDATE SKIP LOCKED`, tenant, cutoff(now, p.FindingsDays))
		if err != nil {
			return err
		}
		if err = removeMetadata(run, tx, tenant, map[string][]uuid.UUID{"finding": findings}); err != nil {
			return err
		}
		if err = queueFindings(run, tx, tenant, findings, nil); err != nil {
			return err
		}
		// UUID identity is lost on expiry: a later rediscovery needs a fresh baseline.
		if _, err = tx.Exec(run, `UPDATE organizations SET findings_epoch=findings_epoch+1 WHERE tenant_id=$1 AND id IN(SELECT COALESCE(f.organization_id,a.organization_id) FROM findings f LEFT JOIN assets a ON a.tenant_id=f.tenant_id AND a.id=f.asset_id WHERE f.tenant_id=$1 AND f.id=ANY($2::uuid[]))`, tenant, findings); err != nil {
			return err
		}
		if _, err = tx.Exec(run, `DELETE FROM findings WHERE tenant_id=$1 AND id=ANY($2::uuid[])`, tenant, findings); err != nil {
			return err
		}
		endpointBatch, err := tx.Exec(run, `DELETE FROM endpoint_datasets d WHERE d.tenant_id=$1 AND (d.asset_id,d.query_key) IN(SELECT x.asset_id,x.query_key FROM endpoint_datasets x JOIN assets a ON a.tenant_id=x.tenant_id AND a.id=x.asset_id WHERE x.tenant_id=$1 AND NOT a.is_demo AND COALESCE(x.collected_at,x.last_attempt_at)<$2 ORDER BY COALESCE(x.collected_at,x.last_attempt_at),x.asset_id,x.query_key LIMIT 512)`, tenant, cutoff(now, p.EndpointDays))
		if err != nil {
			return err
		}
		auditBatch, err := tx.Exec(run, `DELETE FROM audit_log WHERE tenant_id=$1 AND (id,at) IN(SELECT id,at FROM audit_log WHERE tenant_id=$1 AND at<$2 ORDER BY at,id LIMIT 512)`, tenant, cutoff(now, p.AuditDays))
		if err != nil {
			return err
		}
		if _, err = tx.Exec(run, `DELETE FROM data_deletions d WHERE tenant_id=$1 AND completed_at<$2 AND NOT EXISTS(SELECT 1 FROM data_cleanup_tasks t WHERE t.tenant_id=d.tenant_id AND t.operation_id=d.id)`, tenant, cutoff(now, p.AuditDays)); err != nil {
			return err
		}
		expiredAssessments, err := queryIDs(run, tx, `SELECT a.id FROM assessments a WHERE tenant_id=$1 AND results_expired_at IS NOT NULL AND NOT EXISTS(SELECT 1 FROM reports r WHERE r.tenant_id=a.tenant_id AND r.assessment_id=a.id) ORDER BY a.id LIMIT 64 FOR UPDATE OF a SKIP LOCKED`, tenant)
		if err != nil {
			return err
		}
		if err = removeMetadata(run, tx, tenant, map[string][]uuid.UUID{"assessment": expiredAssessments}); err != nil {
			return err
		}
		if _, err = tx.Exec(run, `DELETE FROM assessments a WHERE tenant_id=$1 AND id=ANY($2::uuid[])`, tenant, expiredAssessments); err != nil {
			return err
		}
		var completedAt *time.Time = &now
		if len(expiredAssessments) == 64 || len(reports) == 64 || len(scans) == 32 || len(sims) == 64 || len(executions) == 512 || len(findings) == 256 || endpointBatch.RowsAffected() == 512 || auditBatch.RowsAffected() == 512 {
			completedAt = nil
		}
		_, err = tx.Exec(run, `INSERT INTO retention_policies(tenant_id,version,last_sweep_at) VALUES($1,0,$2) ON CONFLICT(tenant_id) DO UPDATE SET last_sweep_at=$2,last_error_code=''`, tenant, completedAt)
		return err
	})
}

// Cleanup retries every five seconds. Sweeps run hourly and after a policy edit;
// persisted last_sweep_at makes restart safe. Logs never include stored contents.
func (s *Service) Run(ctx context.Context, tenant uuid.UUID) {
	timer := time.NewTicker(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			var p Policy
			err := s.DB.InTenantRead(ctx, tenant, func(tx pgx.Tx) error { var e error; p, e = readPolicy(ctx, tx, tenant); return e })
			clock := time.Now().UTC()
			if s.Now != nil {
				clock = s.Now().UTC()
			}
			if err == nil && (p.LastSweepAt == nil || clock.Sub(*p.LastSweepAt) >= time.Hour) {
				if s.Sweep(ctx, tenant) != nil {
					s.recordError(ctx, tenant, "retention_sweep_failed")
				}
			}
			if s.CleanupOnce(ctx, tenant) != nil {
				s.recordError(ctx, tenant, "retention_cleanup_pending")
			} else if p.LastErrorCode == "retention_cleanup_pending" && p.CleanupPending == 0 {
				s.recordError(ctx, tenant, "")
			}
		}
	}
}
func (s *Service) recordError(ctx context.Context, tenant uuid.UUID, code string) {
	_ = s.DB.InTenant(ctx, tenant, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO retention_policies(tenant_id,version,last_error_code) VALUES($1,0,$2) ON CONFLICT(tenant_id) DO UPDATE SET last_error_code=$2`, tenant, code)
		return err
	})
}

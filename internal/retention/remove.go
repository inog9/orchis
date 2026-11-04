package retention

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"orchis-go/internal/httpx"
	"orchis-go/internal/reporting"
)

func queryIDs(ctx context.Context, tx pgx.Tx, q string, args ...any) ([]uuid.UUID, error) {
	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	out := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, id)
	}
	err = rows.Err()
	rows.Close()
	return out, err
}
func queueReports(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, ids []uuid.UUID, operation *uuid.UUID) error {
	rows, err := tx.Query(ctx, `SELECT id,organization_id,pdf_key,status FROM reports WHERE tenant_id=$1 AND id=ANY($2::uuid[]) ORDER BY id FOR UPDATE`, tenant, ids)
	if err != nil {
		return err
	}
	type item struct {
		id, org    uuid.UUID
		key, state string
	}
	items := []item{}
	for rows.Next() {
		var v item
		if err = rows.Scan(&v.id, &v.org, &v.key, &v.state); err != nil {
			rows.Close()
			return err
		}
		items = append(items, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, v := range items {
		if v.state == "rendering" {
			return httpx.Conflict("A PDF is rendering; its artifacts cannot be deleted yet.")
		}
		expected := tenant.String() + "/reports/" + v.id.String() + ".pdf"
		if v.key != "" && v.key != expected {
			return httpx.Conflict("A report references an unexpected object key; no data was removed.")
		}
		if _, err = tx.Exec(ctx, `INSERT INTO data_cleanup_tasks(tenant_id,kind,object_id,organization_id,operation_id) VALUES($1,'report_pdf',$2,$3,$4) ON CONFLICT DO NOTHING`, tenant, v.id, v.org, operation); err != nil {
			return err
		}
	}
	return nil
}
func queueExecutions(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, ids []uuid.UUID, operation *uuid.UUID) error {
	rows, err := tx.Query(ctx, `SELECT e.id,COALESCE(e.scan_organization_id,a.organization_id),COALESCE(e.raw_output_key,'') FROM executions e JOIN assets a ON a.tenant_id=e.tenant_id AND a.id=e.target_asset_id WHERE e.tenant_id=$1 AND e.id=ANY($2::uuid[]) ORDER BY e.id FOR UPDATE OF e`, tenant, ids)
	if err != nil {
		return err
	}
	type item struct {
		id  uuid.UUID
		org *uuid.UUID
		key string
	}
	items := []item{}
	for rows.Next() {
		var v item
		if err = rows.Scan(&v.id, &v.org, &v.key); err != nil {
			rows.Close()
			return err
		}
		items = append(items, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, v := range items {
		expected := "executions/" + tenant.String() + "/" + v.id.String() + "/output.txt"
		if v.key != "" && v.key != expected {
			return httpx.Conflict("An execution references an unexpected object key; no data was removed.")
		}
		if _, err = tx.Exec(ctx, `INSERT INTO data_cleanup_tasks(tenant_id,kind,object_id,organization_id,operation_id) VALUES($1,'execution_raw',$2,$3,$4) ON CONFLICT DO NOTHING`, tenant, v.id, v.org, operation); err != nil {
			return err
		}
	}
	return nil
}
func queueFindings(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, ids []uuid.UUID, operation *uuid.UUID) error {
	_, err := tx.Exec(ctx, `INSERT INTO data_cleanup_tasks(tenant_id,kind,object_id,organization_id,operation_id) SELECT f.tenant_id,'search_finding',f.id,COALESCE(f.organization_id,a.organization_id),$3::uuid FROM findings f LEFT JOIN assets a ON a.tenant_id=f.tenant_id AND a.id=f.asset_id WHERE f.tenant_id=$1 AND f.id=ANY($2::uuid[]) ON CONFLICT DO NOTHING`, tenant, ids, operation)
	return err
}
func removeOrganization(ctx context.Context, tx pgx.Tx, tenant, org, operation, actor uuid.UUID, preview Preview) error {
	counts, err := json.Marshal(preview.Counts)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO data_deletions(id,tenant_id,organization_id,organization_name,requested_by,counts) VALUES($1,$2,$3,$4,$5,$6)`, operation, tenant, org, reporting.Redact(preview.Name, 200), actor, counts); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO scan_controls(id,tenant_id,organization_id,paused,paused_by,paused_at) VALUES($4,$1,$2,true,$3,now()) ON CONFLICT(tenant_id,organization_id) WHERE organization_id IS NOT NULL DO UPDATE SET paused=true,paused_by=$3,paused_at=now(),updated_at=now(),version=scan_controls.version+1`, tenant, org, actor, uuid.New()); err != nil {
		return err
	}
	assets, err := queryIDs(ctx, tx, `SELECT id FROM assets WHERE tenant_id=$1 AND organization_id=$2`, tenant, org)
	if err != nil {
		return err
	}
	agents, err := queryIDs(ctx, tx, `SELECT id FROM agents WHERE tenant_id=$1 AND (organization_id=$2 OR asset_id=ANY($3::uuid[]))`, tenant, org, assets)
	if err != nil {
		return err
	}
	assessments, err := queryIDs(ctx, tx, `SELECT id FROM assessments WHERE tenant_id=$1 AND organization_id=$2`, tenant, org)
	if err != nil {
		return err
	}
	ownedSims, err := queryIDs(ctx, tx, `SELECT simulation_id FROM assessment_targets WHERE tenant_id=$1 AND assessment_id=ANY($2::uuid[]) AND simulation_id IS NOT NULL UNION SELECT simulation_id FROM scan_jobs WHERE tenant_id=$1 AND assessment_id=ANY($2::uuid[]) AND simulation_id IS NOT NULL`, tenant, assessments)
	if err != nil {
		return err
	}
	allSims, err := queryIDs(ctx, tx, `SELECT simulation_id FROM simulation_targets WHERE tenant_id=$1 AND (asset_id=ANY($3::uuid[]) OR scan_organization_id=$2) UNION SELECT unnest($4::uuid[])`, tenant, org, assets, ownedSims)
	if err != nil {
		return err
	}
	executions, err := queryIDs(ctx, tx, `SELECT id FROM executions WHERE tenant_id=$1 AND (target_asset_id=ANY($3::uuid[]) OR scan_organization_id=$2 OR simulation_id=ANY($4::uuid[]))`, tenant, org, assets, ownedSims)
	if err != nil {
		return err
	}
	reports, err := queryIDs(ctx, tx, `SELECT id FROM reports WHERE tenant_id=$1 AND organization_id=$2`, tenant, org)
	if err != nil {
		return err
	}
	findings, err := queryIDs(ctx, tx, `SELECT id FROM findings WHERE tenant_id=$1 AND (organization_id=$2 OR asset_id=ANY($3::uuid[]))`, tenant, org, assets)
	if err != nil {
		return err
	}
	if err = queueReports(ctx, tx, tenant, reports, &operation); err != nil {
		return err
	}
	if err = queueExecutions(ctx, tx, tenant, executions, &operation); err != nil {
		return err
	}
	if err = queueFindings(ctx, tx, tenant, findings, &operation); err != nil {
		return err
	}
	if err = removeMetadata(ctx, tx, tenant, map[string][]uuid.UUID{"organization": {org}, "asset": assets, "agent": agents, "assessment": assessments, "report": reports, "finding": findings, "execution": executions, "simulation": ownedSims}); err != nil {
		return err
	}
	// SQL is static and every statement retains tenant and selected identity bounds.
	statements := []struct {
		q    string
		args []any
	}{
		{`DELETE FROM ai_job_actions WHERE tenant_id=$1 AND job_id IN(SELECT id FROM ai_jobs WHERE tenant_id=$1 AND (organization_id=$2 OR report_id=ANY($3::uuid[])))`, []any{tenant, org, reports}},
		{`DELETE FROM ai_jobs WHERE tenant_id=$1 AND (organization_id=$2 OR report_id=ANY($3::uuid[]))`, []any{tenant, org, reports}},
		{`DELETE FROM ai_policies WHERE tenant_id=$1 AND organization_id=$2`, []any{tenant, org}},
		{`DELETE FROM endpoint_collection_schedule WHERE tenant_id=$1 AND agent_id=ANY($2::uuid[])`, []any{tenant, agents}},
		{`DELETE FROM agent_enrollment_tokens WHERE tenant_id=$1 AND organization_id=$2`, []any{tenant, org}},
		{`DELETE FROM scan_jobs WHERE tenant_id=$1 AND assessment_id=ANY($2::uuid[])`, []any{tenant, assessments}},
		{`DELETE FROM assessment_targets WHERE tenant_id=$1 AND assessment_id=ANY($2::uuid[])`, []any{tenant, assessments}},
		{`DELETE FROM reports WHERE tenant_id=$1 AND id=ANY($2::uuid[])`, []any{tenant, reports}},
		{`DELETE FROM assessments WHERE tenant_id=$1 AND id=ANY($2::uuid[])`, []any{tenant, assessments}},
		{`DELETE FROM simulation_decisions d WHERE tenant_id=$1 AND (asset_id=ANY($3::uuid[]) OR simulation_id=ANY($4::uuid[]) OR EXISTS(SELECT 1 FROM simulation_targets t WHERE t.tenant_id=d.tenant_id AND t.simulation_id=d.simulation_id AND t.asset_id=d.asset_id AND t.scan_organization_id=$2))`, []any{tenant, org, assets, ownedSims}},
		{`DELETE FROM executions WHERE tenant_id=$1 AND id=ANY($2::uuid[])`, []any{tenant, executions}},
		{`DELETE FROM simulation_targets WHERE tenant_id=$1 AND (asset_id=ANY($3::uuid[]) OR scan_organization_id=$2 OR simulation_id=ANY($4::uuid[]))`, []any{tenant, org, assets, ownedSims}},
		{`DELETE FROM simulations s WHERE tenant_id=$1 AND id=ANY($2::uuid[]) AND NOT EXISTS(SELECT 1 FROM simulation_targets t WHERE t.tenant_id=s.tenant_id AND t.simulation_id=s.id) AND NOT EXISTS(SELECT 1 FROM assessment_targets t WHERE t.tenant_id=s.tenant_id AND t.simulation_id=s.id) AND NOT EXISTS(SELECT 1 FROM scan_jobs j WHERE j.tenant_id=s.tenant_id AND j.simulation_id=s.id)`, []any{tenant, allSims}},
		{`DELETE FROM findings WHERE tenant_id=$1 AND id=ANY($2::uuid[])`, []any{tenant, findings}},
		{`DELETE FROM network_range_authorizations WHERE tenant_id=$1 AND (organization_id=$2 OR control_asset_id=ANY($3::uuid[]) OR agent_id=ANY($4::uuid[]))`, []any{tenant, org, assets, agents}},
		{`DELETE FROM agents WHERE tenant_id=$1 AND id=ANY($2::uuid[])`, []any{tenant, agents}},
		{`DELETE FROM assets WHERE tenant_id=$1 AND id=ANY($2::uuid[])`, []any{tenant, assets}},
		{`DELETE FROM scan_controls WHERE tenant_id=$1 AND organization_id=$2`, []any{tenant, org}},
		{`DELETE FROM organizations WHERE tenant_id=$1 AND id=$2`, []any{tenant, org}},
		{`UPDATE data_cleanup_tasks SET operation_id=$3 WHERE tenant_id=$1 AND organization_id=$2 AND operation_id IS NULL`, []any{tenant, org, operation}},
		{`UPDATE data_deletions d SET completed_at=now() WHERE tenant_id=$1 AND id=$2 AND NOT EXISTS(SELECT 1 FROM data_cleanup_tasks t WHERE t.tenant_id=d.tenant_id AND t.operation_id=d.id)`, []any{tenant, operation}},
	}
	for _, statement := range statements {
		if _, err = tx.Exec(ctx, statement.q, statement.args...); err != nil {
			return err
		}
	}
	return nil
}

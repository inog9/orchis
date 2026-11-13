-- +goose Up
-- endpoint_baselines adalah tabel per tenant: wajib RLS seperti tabel tenant lain.
SELECT enable_tenant_rls('endpoint_baselines');

-- Indeks pendukung untuk foreign key komposit (dipakai saat hapus/update induk dan join).
CREATE INDEX IF NOT EXISTS idx_ecr_requested_by ON endpoint_collection_requests(tenant_id, requested_by);
CREATE INDEX IF NOT EXISTS idx_ecr_asset ON endpoint_collection_requests(tenant_id, asset_id);
CREATE INDEX IF NOT EXISTS idx_audit_schedules_org ON audit_schedules(tenant_id, organization_id);
CREATE INDEX IF NOT EXISTS idx_audit_schedules_created_by ON audit_schedules(tenant_id, created_by);
CREATE INDEX IF NOT EXISTS idx_audit_schedules_run_as ON audit_schedules(tenant_id, run_as);
CREATE INDEX IF NOT EXISTS idx_audit_schedule_runs_run_as ON audit_schedule_runs(tenant_id, run_as);
CREATE INDEX IF NOT EXISTS idx_audit_schedule_runs_scan ON audit_schedule_runs(tenant_id, scan_id);
CREATE INDEX IF NOT EXISTS idx_scan_recommendations_report ON scan_recommendations(tenant_id, report_id);

-- +goose Down
DROP INDEX IF EXISTS idx_scan_recommendations_report, idx_audit_schedule_runs_scan, idx_audit_schedule_runs_run_as, idx_audit_schedules_run_as, idx_audit_schedules_created_by, idx_audit_schedules_org, idx_ecr_asset, idx_ecr_requested_by;
ALTER TABLE endpoint_baselines NO FORCE ROW LEVEL SECURITY;
ALTER TABLE endpoint_baselines DISABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON endpoint_baselines;

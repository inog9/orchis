-- +goose Up
ALTER TABLE organizations ADD COLUMN findings_epoch bigint NOT NULL DEFAULT 0 CHECK(findings_epoch>=0);
CREATE TABLE retention_policies (
 tenant_id uuid PRIMARY KEY REFERENCES tenants(id),
 findings_days integer NOT NULL DEFAULT 180 CHECK(findings_days BETWEEN 1 AND 3650),
 scan_days integer NOT NULL DEFAULT 90 CHECK(scan_days BETWEEN 1 AND 3650),
 report_days integer NOT NULL DEFAULT 180 CHECK(report_days BETWEEN 1 AND 3650),
 endpoint_days integer NOT NULL DEFAULT 30 CHECK(endpoint_days BETWEEN 1 AND 3650),
 audit_days integer NOT NULL DEFAULT 365 CHECK(audit_days BETWEEN 90 AND 3650),
 version integer NOT NULL DEFAULT 1 CHECK(version>=0), -- zero materializes implicit defaults without an admin edit
 last_error_code text NOT NULL DEFAULT '',updated_at timestamptz NOT NULL DEFAULT now(),last_sweep_at timestamptz
);
SELECT enable_tenant_rls('retention_policies');
CREATE TABLE data_deletions (
 id uuid PRIMARY KEY,tenant_id uuid NOT NULL REFERENCES tenants(id),organization_id uuid NOT NULL,
 organization_name text NOT NULL,requested_by uuid,counts jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),completed_at timestamptz,
 UNIQUE(tenant_id,id),FOREIGN KEY(tenant_id,requested_by) REFERENCES users(tenant_id,id) ON DELETE SET NULL(requested_by)
);
SELECT enable_tenant_rls('data_deletions');
CREATE INDEX idx_data_deletions_requester ON data_deletions(tenant_id,requested_by);
CREATE INDEX idx_data_deletions_tenant ON data_deletions(tenant_id,created_at DESC,id DESC);
CREATE TABLE data_cleanup_tasks (
 tenant_id uuid NOT NULL REFERENCES tenants(id),kind text NOT NULL CHECK(kind IN('report_pdf','execution_raw','search_finding')),
 object_id uuid NOT NULL,organization_id uuid,operation_id uuid,created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(tenant_id,kind,object_id),FOREIGN KEY(tenant_id,operation_id) REFERENCES data_deletions(tenant_id,id)
);
SELECT enable_tenant_rls('data_cleanup_tasks');
CREATE INDEX idx_data_cleanup_operation ON data_cleanup_tasks(tenant_id,operation_id);
CREATE INDEX idx_data_cleanup_org ON data_cleanup_tasks(tenant_id,organization_id);
ALTER TABLE assessments ADD COLUMN report_expired_at timestamptz, ADD COLUMN results_expired_at timestamptz,
 ADD CHECK(results_expired_at IS NULL OR status<>'running');
CREATE INDEX idx_retention_assessments ON assessments(tenant_id,created_at,id) WHERE status<>'running' AND results_expired_at IS NULL;
CREATE INDEX idx_retention_findings ON findings(tenant_id,last_seen_at,id);
CREATE INDEX idx_retention_endpoint ON endpoint_datasets(tenant_id,COALESCE(collected_at,last_attempt_at));

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Retention deletes data permanently; restore an encrypted backup instead of rolling back this migration.'; END $$;
-- +goose StatementEnd

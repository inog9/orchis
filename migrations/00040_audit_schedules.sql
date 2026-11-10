-- +goose Up
CREATE TABLE audit_schedules (
 id uuid PRIMARY KEY, tenant_id uuid NOT NULL REFERENCES tenants(id), organization_id uuid NOT NULL,
 name text NOT NULL CHECK(length(name) BETWEEN 1 AND 120), config jsonb NOT NULL,
 creation_key uuid NOT NULL, creation_hash text NOT NULL,
 paused boolean NOT NULL DEFAULT false, deleted_at timestamptz,
 next_run_at timestamptz NOT NULL, next_digest_at timestamptz NOT NULL,
 created_by uuid NOT NULL, run_as uuid NOT NULL, version integer NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(tenant_id,id), UNIQUE(tenant_id,creation_key),
 FOREIGN KEY(tenant_id,organization_id) REFERENCES organizations(tenant_id,id) ON DELETE CASCADE,
 FOREIGN KEY(tenant_id,created_by) REFERENCES users(tenant_id,id),
 FOREIGN KEY(tenant_id,run_as) REFERENCES users(tenant_id,id)
);
SELECT enable_tenant_rls('audit_schedules');
CREATE INDEX audit_schedules_due ON audit_schedules(tenant_id,next_run_at) WHERE NOT paused AND deleted_at IS NULL;
CREATE TABLE audit_schedule_runs (
 id uuid PRIMARY KEY, tenant_id uuid NOT NULL, schedule_id uuid NOT NULL,
 scheduled_at timestamptz NOT NULL, status text NOT NULL DEFAULT 'queued'
 CHECK(status IN ('queued','running','finished','incomplete','failed','canceled','blocked','missed')),
 scan_id uuid, run_as uuid NOT NULL, input jsonb NOT NULL, max_runtime_minutes integer NOT NULL, notify_changes boolean NOT NULL,
 reason text NOT NULL DEFAULT '', summary jsonb NOT NULL DEFAULT '{}',
 created_at timestamptz NOT NULL DEFAULT now(), started_at timestamptz, ended_at timestamptz,
 UNIQUE(tenant_id,id), UNIQUE(tenant_id,schedule_id,scheduled_at),
 FOREIGN KEY(tenant_id,run_as) REFERENCES users(tenant_id,id),
 FOREIGN KEY(tenant_id,schedule_id) REFERENCES audit_schedules(tenant_id,id) ON DELETE CASCADE,
 FOREIGN KEY(tenant_id,scan_id) REFERENCES assessments(tenant_id,id) ON DELETE SET NULL (scan_id)
);
SELECT enable_tenant_rls('audit_schedule_runs');
CREATE INDEX audit_schedule_runs_pending ON audit_schedule_runs(tenant_id,status) WHERE status IN ('queued','running');
CREATE INDEX audit_schedule_runs_history ON audit_schedule_runs(tenant_id,schedule_id,scheduled_at DESC);
-- +goose Down
DROP TABLE audit_schedule_runs;
DROP TABLE audit_schedules;

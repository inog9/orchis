-- +goose Up
CREATE TABLE scan_recommendations (
 id uuid PRIMARY KEY, tenant_id uuid NOT NULL, scan_id uuid NOT NULL, report_id uuid NOT NULL,
 status text NOT NULL DEFAULT 'queued' CHECK(status IN ('queued','running','ready','failed','not_needed')),
 input jsonb NOT NULL, output jsonb, error text NOT NULL DEFAULT '', model text NOT NULL DEFAULT '',
 claim_hash text, lease_until timestamptz, created_at timestamptz NOT NULL DEFAULT now(),
 started_at timestamptz, ended_at timestamptz,
 UNIQUE(tenant_id,id), UNIQUE(tenant_id,scan_id),
 FOREIGN KEY(tenant_id,scan_id) REFERENCES assessments(tenant_id,id) ON DELETE CASCADE,
 FOREIGN KEY(tenant_id,report_id) REFERENCES reports(tenant_id,id) ON DELETE CASCADE
);
SELECT enable_tenant_rls('scan_recommendations');
CREATE INDEX scan_recommendations_queue ON scan_recommendations(tenant_id,created_at) WHERE status IN ('queued','running');
-- +goose Down
DROP TABLE scan_recommendations;

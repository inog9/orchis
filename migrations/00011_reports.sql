-- +goose Up
CREATE TABLE reports (
 id uuid PRIMARY KEY,tenant_id uuid NOT NULL REFERENCES tenants(id),organization_id uuid NOT NULL,assessment_id uuid NOT NULL,
 version integer NOT NULL DEFAULT 1,snapshot jsonb NOT NULL,
 status text NOT NULL DEFAULT 'queued' CHECK(status IN('queued','rendering','ready','failed')),
 pdf_key text NOT NULL DEFAULT '',error text NOT NULL DEFAULT '',attempts integer NOT NULL DEFAULT 0,
 lease_until timestamptz,created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(tenant_id,id),UNIQUE(tenant_id,assessment_id),
 FOREIGN KEY(tenant_id,organization_id) REFERENCES organizations(tenant_id,id),
 FOREIGN KEY(tenant_id,assessment_id) REFERENCES assessments(tenant_id,id)
);
CREATE INDEX idx_reports_org ON reports(tenant_id,organization_id,created_at DESC,id DESC);
CREATE INDEX idx_reports_worker ON reports(tenant_id,status,lease_until);
SELECT enable_tenant_rls('reports');

ALTER TABLE finding_occurrences ADD COLUMN finding_snapshot jsonb;

-- +goose Up
CREATE TABLE organizations (
 id uuid PRIMARY KEY, tenant_id uuid NOT NULL REFERENCES tenants(id),
 name text NOT NULL CHECK(length(name) BETWEEN 1 AND 200),
 slug text NOT NULL CHECK(slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$' AND length(slug)<=100),
 description text NOT NULL DEFAULT '', industry text NOT NULL DEFAULT '', contact text NOT NULL DEFAULT '',
 version integer NOT NULL DEFAULT 1, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(tenant_id,id), UNIQUE(tenant_id,slug)
);
SELECT enable_tenant_rls('organizations');
ALTER TABLE assets ADD COLUMN organization_id uuid, ADD FOREIGN KEY(tenant_id,organization_id) REFERENCES organizations(tenant_id,id);
CREATE INDEX idx_assets_organization ON assets(tenant_id,organization_id);
ALTER TABLE agents ADD COLUMN organization_id uuid, ADD COLUMN instance_name text,
 ADD FOREIGN KEY(tenant_id,organization_id) REFERENCES organizations(tenant_id,id);
CREATE UNIQUE INDEX uk_agents_org_instance ON agents(tenant_id,organization_id,instance_name) WHERE status<>'revoked' AND instance_name IS NOT NULL;
CREATE INDEX idx_agents_organization ON agents(tenant_id,organization_id);
ALTER TABLE agent_enrollment_tokens ADD COLUMN organization_id uuid,
 ADD FOREIGN KEY(tenant_id,organization_id) REFERENCES organizations(tenant_id,id);
CREATE INDEX idx_enrollment_organization ON agent_enrollment_tokens(tenant_id,organization_id);
CREATE TABLE assessments (
 id uuid PRIMARY KEY, tenant_id uuid NOT NULL REFERENCES tenants(id), organization_id uuid NOT NULL,
 name text NOT NULL, profile text NOT NULL, idempotency_key text NOT NULL, request_hash text NOT NULL,
 status text NOT NULL DEFAULT 'running' CHECK(status IN ('running','finished','canceled')),
 created_by uuid NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), ended_at timestamptz,
 UNIQUE(tenant_id,id), UNIQUE(tenant_id,organization_id,idempotency_key),
 FOREIGN KEY(tenant_id,organization_id) REFERENCES organizations(tenant_id,id),
 FOREIGN KEY(tenant_id,created_by) REFERENCES users(tenant_id,id)
);
CREATE INDEX idx_assessments_org ON assessments(tenant_id,organization_id,created_at DESC,id DESC);
CREATE INDEX idx_assessments_creator ON assessments(tenant_id,created_by);
SELECT enable_tenant_rls('assessments');
CREATE TABLE assessment_targets (
 tenant_id uuid NOT NULL, assessment_id uuid NOT NULL, asset_id uuid NOT NULL,
 asset_name text NOT NULL, addresses jsonb NOT NULL, simulation_id uuid,
 skip_reason text NOT NULL DEFAULT '', PRIMARY KEY(tenant_id,assessment_id,asset_id),
 FOREIGN KEY(tenant_id,assessment_id) REFERENCES assessments(tenant_id,id),
 FOREIGN KEY(tenant_id,asset_id) REFERENCES assets(tenant_id,id),
 FOREIGN KEY(tenant_id,simulation_id) REFERENCES simulations(tenant_id,id),
 UNIQUE(tenant_id,simulation_id)
);
CREATE INDEX idx_assessment_targets_asset ON assessment_targets(tenant_id,asset_id);
SELECT enable_tenant_rls('assessment_targets');
UPDATE roles SET permissions=permissions || ARRAY['organization.read','assessment.read','report.read'] WHERE is_system;
UPDATE roles SET permissions=permissions || ARRAY['organization.write','assessment.run','report.generate'] WHERE is_system AND key IN ('admin','analyst');

-- +goose Up
CREATE TABLE notification_policies (
 tenant_id uuid PRIMARY KEY REFERENCES tenants(id),
 offline_minutes integer NOT NULL DEFAULT 15 CHECK(offline_minutes BETWEEN 2 AND 10080),
 version integer NOT NULL DEFAULT 1 CHECK(version>0), updated_at timestamptz NOT NULL DEFAULT now()
);
SELECT enable_tenant_rls('notification_policies');
ALTER TABLE alerts ADD COLUMN organization_id uuid,
 ADD CONSTRAINT alerts_organization_fk FOREIGN KEY(tenant_id,organization_id) REFERENCES organizations(tenant_id,id) ON DELETE CASCADE;
UPDATE alerts a SET organization_id=CASE a.resource_type
 WHEN 'organization' THEN (SELECT id FROM organizations WHERE tenant_id=a.tenant_id AND id=a.resource_id)
 WHEN 'agent' THEN (SELECT organization_id FROM agents WHERE tenant_id=a.tenant_id AND id=a.resource_id)
 WHEN 'asset' THEN (SELECT organization_id FROM assets WHERE tenant_id=a.tenant_id AND id=a.resource_id)
 WHEN 'finding' THEN (SELECT x.organization_id FROM findings f JOIN assets x ON x.tenant_id=f.tenant_id AND x.id=f.asset_id WHERE f.tenant_id=a.tenant_id AND f.id=a.resource_id)
 WHEN 'scan' THEN (SELECT organization_id FROM assessments WHERE tenant_id=a.tenant_id AND id=a.resource_id AND is_scan)
 WHEN 'report' THEN (SELECT organization_id FROM reports WHERE tenant_id=a.tenant_id AND id=a.resource_id)
 END WHERE a.resource_id IS NOT NULL;
UPDATE alerts SET audience_permission='finding.read' WHERE kind='new_critical_finding' AND resource_type='finding' AND audience_permission='alert.read';
CREATE INDEX alerts_org_created_idx ON alerts(tenant_id,organization_id,created_at DESC,id DESC);
ALTER TABLE reports ADD COLUMN render_generation integer NOT NULL DEFAULT 1 CHECK(render_generation>0);
ALTER TABLE agents ADD COLUMN offline_episode bigint NOT NULL DEFAULT 0 CHECK(offline_episode>=0),
 ADD COLUMN update_failure_episode bigint NOT NULL DEFAULT 0 CHECK(update_failure_episode>=0);
-- +goose Down
ALTER TABLE reports DROP COLUMN render_generation;
ALTER TABLE agents DROP COLUMN update_failure_episode,DROP COLUMN offline_episode;
ALTER TABLE alerts DROP COLUMN organization_id;
DROP TABLE notification_policies;

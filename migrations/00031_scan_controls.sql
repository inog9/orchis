-- +goose Up
CREATE TABLE scan_controls (
 id uuid PRIMARY KEY, tenant_id uuid NOT NULL REFERENCES tenants(id), organization_id uuid,
 paused boolean NOT NULL DEFAULT false, paused_by uuid, paused_at timestamptz,
 scan_window jsonb NOT NULL DEFAULT '{}', version integer NOT NULL DEFAULT 1 CHECK(version>0),
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(tenant_id,id),
 FOREIGN KEY(tenant_id,organization_id) REFERENCES organizations(tenant_id,id),
 FOREIGN KEY(tenant_id,paused_by) REFERENCES users(tenant_id,id),
 CHECK(organization_id IS NOT NULL OR scan_window='{}'::jsonb)
);
CREATE UNIQUE INDEX uk_scan_controls_global ON scan_controls(tenant_id) WHERE organization_id IS NULL;
CREATE UNIQUE INDEX uk_scan_controls_org ON scan_controls(tenant_id,organization_id) WHERE organization_id IS NOT NULL;
CREATE INDEX idx_scan_controls_organization ON scan_controls(tenant_id,organization_id);
CREATE INDEX idx_scan_controls_actor ON scan_controls(tenant_id,paused_by);
SELECT enable_tenant_rls('scan_controls');

ALTER TABLE simulation_targets ADD COLUMN scan_organization_id uuid,
 ADD FOREIGN KEY(tenant_id,scan_organization_id) REFERENCES organizations(tenant_id,id);
CREATE INDEX idx_simulation_targets_scan_org ON simulation_targets(tenant_id,scan_organization_id);
ALTER TABLE executions ADD COLUMN scan_organization_id uuid,
 ADD COLUMN scan_stop_reason text NOT NULL DEFAULT '' CHECK(scan_stop_reason IN ('','emergency_stop','outside_window','invalid_window','asset_removed')),
 ADD FOREIGN KEY(tenant_id,scan_organization_id) REFERENCES organizations(tenant_id,id);
CREATE INDEX idx_executions_scan_org ON executions(tenant_id,scan_organization_id,state);
-- Capture the organisation separately from mutable inventory membership. Historical snapshots stay frozen.
UPDATE simulation_targets t SET scan_organization_id=COALESCE(
 (SELECT a.organization_id FROM scan_jobs j JOIN assessments a ON a.tenant_id=j.tenant_id AND a.id=j.assessment_id WHERE j.tenant_id=t.tenant_id AND j.simulation_id=t.simulation_id),
 (SELECT a.organization_id FROM assessment_targets x JOIN assessments a ON a.tenant_id=x.tenant_id AND a.id=x.assessment_id WHERE x.tenant_id=t.tenant_id AND x.simulation_id=t.simulation_id),
 (SELECT a.organization_id FROM assets a WHERE a.tenant_id=t.tenant_id AND a.id=t.asset_id));
UPDATE executions e SET scan_organization_id=COALESCE(
 (SELECT a.organization_id FROM scan_jobs j JOIN assessments a ON a.tenant_id=j.tenant_id AND a.id=j.assessment_id WHERE j.tenant_id=e.tenant_id AND j.simulation_id=e.simulation_id),
 (SELECT a.organization_id FROM assessment_targets x JOIN assessments a ON a.tenant_id=x.tenant_id AND a.id=x.assessment_id WHERE x.tenant_id=e.tenant_id AND x.simulation_id=e.simulation_id),
 (SELECT t.scan_organization_id FROM simulation_targets t WHERE t.tenant_id=e.tenant_id AND t.simulation_id=e.simulation_id AND t.asset_id=e.target_asset_id),
 (SELECT a.organization_id FROM assets a WHERE a.tenant_id=e.tenant_id AND a.id=e.target_asset_id));
-- +goose StatementBegin
CREATE FUNCTION freeze_scan_target_organization() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 NEW.scan_organization_id := COALESCE(
  (SELECT a.organization_id FROM scan_jobs j JOIN assessments a ON a.tenant_id=j.tenant_id AND a.id=j.assessment_id WHERE j.tenant_id=NEW.tenant_id AND j.simulation_id=NEW.simulation_id),
  (SELECT a.organization_id FROM assessment_targets x JOIN assessments a ON a.tenant_id=x.tenant_id AND a.id=x.assessment_id WHERE x.tenant_id=NEW.tenant_id AND x.simulation_id=NEW.simulation_id),
  (SELECT a.organization_id FROM assets a WHERE a.tenant_id=NEW.tenant_id AND a.id=NEW.asset_id));
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER freeze_scan_target_organization BEFORE INSERT ON simulation_targets FOR EACH ROW EXECUTE FUNCTION freeze_scan_target_organization();
-- +goose StatementBegin
CREATE FUNCTION freeze_scan_execution_organization() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 NEW.scan_organization_id := COALESCE(
  (SELECT a.organization_id FROM scan_jobs j JOIN assessments a ON a.tenant_id=j.tenant_id AND a.id=j.assessment_id WHERE j.tenant_id=NEW.tenant_id AND j.simulation_id=NEW.simulation_id),
  (SELECT a.organization_id FROM assessment_targets x JOIN assessments a ON a.tenant_id=x.tenant_id AND a.id=x.assessment_id WHERE x.tenant_id=NEW.tenant_id AND x.simulation_id=NEW.simulation_id),
  (SELECT t.scan_organization_id FROM simulation_targets t WHERE t.tenant_id=NEW.tenant_id AND t.simulation_id=NEW.simulation_id AND t.asset_id=NEW.target_asset_id),
  (SELECT a.organization_id FROM assets a WHERE a.tenant_id=NEW.tenant_id AND a.id=NEW.target_asset_id));
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER freeze_scan_execution_organization BEFORE INSERT ON executions FOR EACH ROW EXECUTE FUNCTION freeze_scan_execution_organization();
-- +goose Down
DROP TRIGGER freeze_scan_execution_organization ON executions;
DROP FUNCTION freeze_scan_execution_organization();
DROP TRIGGER freeze_scan_target_organization ON simulation_targets;
DROP FUNCTION freeze_scan_target_organization();
ALTER TABLE executions DROP COLUMN scan_organization_id, DROP COLUMN scan_stop_reason;
ALTER TABLE simulation_targets DROP COLUMN scan_organization_id;
DROP TABLE scan_controls;

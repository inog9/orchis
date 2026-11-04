-- +goose Up
CREATE TABLE agent_rollout_policies (
 tenant_id uuid PRIMARY KEY REFERENCES tenants(id),
 mode text NOT NULL DEFAULT 'automatic' CHECK(mode IN('automatic','manual')),
 version integer NOT NULL DEFAULT 1 CHECK(version>0),updated_at timestamptz NOT NULL DEFAULT now()
);
SELECT enable_tenant_rls('agent_rollout_policies');
ALTER TABLE agents
 ADD COLUMN control_version integer NOT NULL DEFAULT 1 CHECK(control_version>0),
 ADD COLUMN paused boolean NOT NULL DEFAULT false,
 ADD COLUMN control_protocol integer NOT NULL DEFAULT 0 CHECK(control_protocol BETWEEN 0 AND 1),
 ADD COLUMN osquery_version text NOT NULL DEFAULT '' CHECK(length(osquery_version)<=32),
 ADD COLUMN diagnostic_code text NOT NULL DEFAULT '' CHECK(diagnostic_code IN('','update_failed','release_key_missing','collection_failed','uninstall_failed','uninstall_unsupported')),
 ADD COLUMN diagnostic_at timestamptz,
 ADD COLUMN requested_update_version text NOT NULL DEFAULT '' CHECK(length(requested_update_version)<=32),
 ADD COLUMN uninstall_id uuid,
 ADD COLUMN uninstall_status text NOT NULL DEFAULT 'none' CHECK(uninstall_status IN('none','queued','dispatched','failed','completed')),
 ADD COLUMN uninstall_requested_at timestamptz,
 ADD COLUMN uninstall_completed_at timestamptz;
-- +goose Down
ALTER TABLE agents DROP COLUMN uninstall_completed_at,DROP COLUMN uninstall_requested_at,DROP COLUMN uninstall_status,DROP COLUMN uninstall_id,DROP COLUMN requested_update_version,DROP COLUMN diagnostic_at,DROP COLUMN diagnostic_code,DROP COLUMN osquery_version,DROP COLUMN control_protocol,DROP COLUMN paused,DROP COLUMN control_version;
DROP TABLE agent_rollout_policies;

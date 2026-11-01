-- +goose Up
ALTER TABLE roles ADD COLUMN is_demo boolean NOT NULL DEFAULT false;
CREATE INDEX idx_roles_demo ON roles(tenant_id) WHERE is_demo;
ALTER TABLE users ADD COLUMN is_demo boolean NOT NULL DEFAULT false;
CREATE INDEX idx_users_demo ON users(tenant_id) WHERE is_demo;
ALTER TABLE api_tokens ADD COLUMN is_demo boolean NOT NULL DEFAULT false;
CREATE INDEX idx_api_tokens_demo ON api_tokens(tenant_id) WHERE is_demo;
ALTER TABLE password_reset_links ADD COLUMN is_demo boolean NOT NULL DEFAULT false;
CREATE INDEX idx_password_reset_links_demo ON password_reset_links(tenant_id) WHERE is_demo;
ALTER TABLE people ADD COLUMN is_demo boolean NOT NULL DEFAULT false;
CREATE INDEX idx_people_demo ON people(tenant_id) WHERE is_demo;
ALTER TABLE teams ADD COLUMN is_demo boolean NOT NULL DEFAULT false;
CREATE INDEX idx_teams_demo ON teams(tenant_id) WHERE is_demo;
ALTER TABLE asset_groups ADD COLUMN is_demo boolean NOT NULL DEFAULT false;
CREATE INDEX idx_asset_groups_demo ON asset_groups(tenant_id) WHERE is_demo;
ALTER TABLE action_presets ADD COLUMN is_demo boolean NOT NULL DEFAULT false;
CREATE INDEX idx_action_presets_demo ON action_presets(tenant_id) WHERE is_demo;
ALTER TABLE atomic_tests ADD COLUMN is_demo boolean NOT NULL DEFAULT false;
CREATE INDEX idx_atomic_tests_demo ON atomic_tests(tenant_id) WHERE is_demo;
ALTER TABLE integrations ADD COLUMN is_demo boolean NOT NULL DEFAULT false;
CREATE INDEX idx_integrations_demo ON integrations(tenant_id) WHERE is_demo;

-- Existing demo seeds must not keep non-local scan authorization.
UPDATE assets a SET testing_authorized=false, authorized_by=NULL, authorized_at=NULL
WHERE a.is_demo AND EXISTS(SELECT 1 FROM asset_addresses ad WHERE ad.tenant_id=a.tenant_id AND ad.asset_id=a.id AND ad.value<>'127.0.0.1');

-- +goose Down
ALTER TABLE integrations DROP COLUMN is_demo;
ALTER TABLE atomic_tests DROP COLUMN is_demo;
ALTER TABLE action_presets DROP COLUMN is_demo;
ALTER TABLE asset_groups DROP COLUMN is_demo;
ALTER TABLE teams DROP COLUMN is_demo;
ALTER TABLE people DROP COLUMN is_demo;
ALTER TABLE password_reset_links DROP COLUMN is_demo;
ALTER TABLE api_tokens DROP COLUMN is_demo;
ALTER TABLE users DROP COLUMN is_demo;
ALTER TABLE roles DROP COLUMN is_demo;

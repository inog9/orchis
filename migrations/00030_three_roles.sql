-- +goose Up
-- Snapshot old EFFECTIVE permissions before replacing built-ins; report access formerly
-- required all five permissions. Never widen a narrow custom account by migration.
CREATE TEMP TABLE role_permissions_before ON COMMIT DROP AS
 SELECT id,tenant_id,key,CASE WHEN permissions @> ARRAY['report.read','finding.read','organization.read','assessment.read','asset.read']::text[]
 THEN permissions ELSE array_remove(permissions,'report.read') END AS permissions FROM roles;
CREATE TEMP TABLE canonical_roles(key text,permissions text[]) ON COMMIT DROP;
INSERT INTO canonical_roles VALUES
('admin',ARRAY['ai.read','organization.read','assessment.read','report.read','atomic.read','asset.read','scenario.read','simulation.read','finding.read','people.read','agent.read','integration.read','alert.read','ai.generate','ai.approve','organization.write','assessment.run','report.generate','atomic.write','atomic.run','asset.write','scenario.write','simulation.run','people.write','ai.manage','atomic.delete','asset.delete','asset.authorize','scenario.delete','agent.manage','integration.manage','user.manage','settings.manage','audit.read']::text[]),
('analyst',ARRAY['ai.read','organization.read','assessment.read','report.read','atomic.read','asset.read','scenario.read','simulation.read','finding.read','people.read','agent.read','integration.read','alert.read','ai.generate','ai.approve','organization.write','assessment.run','report.generate','atomic.write','atomic.run','asset.write','scenario.write','simulation.run','people.write']::text[]),
('viewer',ARRAY['finding.read','report.read']::text[]);
INSERT INTO roles(id,tenant_id,key,name,permissions,is_system)
 SELECT gen_random_uuid(),t.id,c.key,initcap(c.key),c.permissions,true FROM tenants t CROSS JOIN canonical_roles c
 ON CONFLICT(tenant_id,key) DO UPDATE SET permissions=EXCLUDED.permissions,is_system=true,updated_at=now();
ALTER TABLE users ADD COLUMN role_review_required boolean NOT NULL DEFAULT false;
ALTER TABLE users ADD COLUMN previous_role_key text;
CREATE TEMP TABLE role_mapping ON COMMIT DROP AS
 SELECT old.id,old.tenant_id,old.key AS old_key,
 CASE WHEN old.permissions @> (SELECT permissions FROM canonical_roles WHERE key='admin') THEN 'admin'
 WHEN old.permissions @> (SELECT permissions FROM canonical_roles WHERE key='analyst') THEN 'analyst'
 WHEN old.permissions @> (SELECT permissions FROM canonical_roles WHERE key='viewer') THEN 'viewer'
 ELSE NULL END AS new_key FROM role_permissions_before old;
CREATE TEMP TABLE user_role_mapping ON COMMIT DROP AS
 SELECT u.id,u.tenant_id,m.old_key,m.new_key FROM users u JOIN role_mapping m ON m.tenant_id=u.tenant_id AND m.id=u.role_id;
UPDATE users u SET role_id=r.id,role_review_required=(m.new_key IS NULL),
 previous_role_key=CASE WHEN m.new_key IS NULL THEN m.old_key ELSE NULL END,
 status=CASE WHEN m.new_key IS NULL THEN 'disabled' ELSE u.status END,
 version=u.version+1,updated_at=now()
 FROM role_mapping m JOIN roles r ON r.tenant_id=m.tenant_id AND r.key=COALESCE(m.new_key,'viewer')
 WHERE u.tenant_id=m.tenant_id AND u.role_id=m.id;
UPDATE sessions s SET revoked_at=now() FROM users u WHERE s.tenant_id=u.tenant_id AND s.user_id=u.id AND u.role_review_required AND s.revoked_at IS NULL;
UPDATE api_tokens s SET revoked_at=now() FROM users u WHERE s.tenant_id=u.tenant_id AND s.user_id=u.id AND u.role_review_required AND s.revoked_at IS NULL;
INSERT INTO audit_log(id,tenant_id,action,resource_type,resource_id,detail)
 SELECT gen_random_uuid(),u.tenant_id,'user.role_migrated','user',u.id,
 jsonb_build_object('role',r.key,'review_required',u.role_review_required,'previous_role',m.old_key,'migration',30)
 FROM users u JOIN roles r ON r.tenant_id=u.tenant_id AND r.id=u.role_id
 JOIN user_role_mapping m ON m.tenant_id=u.tenant_id AND m.id=u.id;
DELETE FROM roles WHERE key NOT IN ('admin','analyst','viewer');
ALTER TABLE roles ADD CONSTRAINT roles_three_keys CHECK(key IN ('admin','analyst','viewer'));
-- Prevent an old binary's startup role sync from silently restoring broad Viewer access.
ALTER TABLE roles ADD CONSTRAINT roles_viewer_read_only CHECK(key<>'viewer' OR permissions <@ ARRAY['finding.read','report.read']::text[]);
ALTER TABLE users ADD CONSTRAINT users_role_review_disabled CHECK(NOT role_review_required OR status='disabled');
-- +goose Down
-- Restore the pre-upgrade database backup together with the old binary. Mapping is irreversible.
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Migration 30 requires restoring the pre-upgrade database backup for rollback'; END $$;
-- +goose StatementEnd

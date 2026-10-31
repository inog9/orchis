-- +goose Up
-- Remove historical guessed identities. Unknown requests now use a shared IP bucket.
-- +goose StatementBegin
DO $$ DECLARE tenant uuid; BEGIN
 FOR tenant IN SELECT id FROM tenants LOOP
  PERFORM set_config('app.tenant_id',tenant::text,true);
  DELETE FROM login_throttle lt WHERE lt.tenant_id=tenant
   AND NOT EXISTS(SELECT 1 FROM users u WHERE u.tenant_id=lt.tenant_id AND u.email=lt.email AND u.deleted_at IS NULL);
 END LOOP;
END $$;
-- +goose StatementEnd
CREATE INDEX idx_audit_tenant_at_id ON audit_log(tenant_id,at DESC,id DESC);
CREATE INDEX idx_audit_tenant_action_at ON audit_log(tenant_id,action,at DESC,id DESC);
-- +goose Down
DROP INDEX idx_audit_tenant_action_at;
DROP INDEX idx_audit_tenant_at_id;
-- Guessed identities are intentionally not restored.

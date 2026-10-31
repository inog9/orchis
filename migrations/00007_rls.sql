-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION enable_tenant_rls(tbl regclass) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
  EXECUTE format('ALTER TABLE %s ENABLE ROW LEVEL SECURITY', tbl);
  EXECUTE format('ALTER TABLE %s FORCE ROW LEVEL SECURITY', tbl);
  EXECUTE format('DROP POLICY IF EXISTS tenant_isolation ON %s', tbl);
  EXECUTE format(
    'CREATE POLICY tenant_isolation ON %s USING (tenant_id = NULLIF(current_setting(''app.tenant_id'', true), '''')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting(''app.tenant_id'', true), '''')::uuid)',
    tbl);
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
DO $$
DECLARE t record;
BEGIN
  FOR t IN
    SELECT c.table_name FROM information_schema.columns c
    JOIN information_schema.tables tb USING (table_schema, table_name)
    WHERE c.table_schema = 'public' AND c.column_name = 'tenant_id'
      AND tb.table_type = 'BASE TABLE' AND c.table_name NOT LIKE 'audit_log\_%'
  LOOP
    PERFORM enable_tenant_rls(quote_ident(t.table_name)::regclass);
  END LOOP;
END $$;
-- +goose StatementEnd

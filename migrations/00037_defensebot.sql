-- +goose Up
-- DefenseBot: new scan tool, and removal of the inventory-style queries that are no longer collected or shown.
ALTER TABLE scan_jobs DROP CONSTRAINT scan_jobs_tool_check;
ALTER TABLE scan_jobs ADD CONSTRAINT scan_jobs_tool_check CHECK(tool IN ('nmap','nuclei','hackbot','defensebot'));
-- "Stop collecting" must be real: drop stored datasets of queries that were removed or changed shape, and switch their approvals off.
DELETE FROM endpoint_datasets WHERE query_key IN ('osquery.inventory','osquery.users','osquery.network','osquery.network_ranges','osquery.firewall','osquery.packages');
UPDATE osquery_org_settings SET enabled=false, updated_at=now() WHERE query_key IN ('osquery.inventory','osquery.users','osquery.network','osquery.network_ranges');
DELETE FROM endpoint_collection_schedule WHERE query_key IN ('osquery.inventory','osquery.users','osquery.network','osquery.network_ranges');
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 RAISE EXCEPTION 'DefenseBot history requires restore-based rollback; restore a pre-upgrade backup with its matching release.';
END $$;
-- +goose StatementEnd

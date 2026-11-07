-- +goose Up
ALTER TABLE scan_jobs DROP CONSTRAINT scan_jobs_tool_check;
ALTER TABLE scan_jobs ADD CONSTRAINT scan_jobs_tool_check CHECK(tool IN ('nmap','nuclei','hackbot'));
ALTER TABLE scan_jobs DROP CONSTRAINT scan_jobs_status_check;
ALTER TABLE scan_jobs ADD CONSTRAINT scan_jobs_status_check CHECK(status IN ('waiting','running','finished','incomplete','failed','canceled'));
ALTER TABLE assessments DROP CONSTRAINT assessments_status_check;
ALTER TABLE assessments ADD CONSTRAINT assessments_status_check CHECK(status IN ('running','finished','incomplete','failed','canceled'));
ALTER TABLE executions ADD COLUMN hackbot_result jsonb, ADD COLUMN hackbot_progress jsonb;
-- Keep legacy outcome enums for chain compatibility; delivery remains explicit in hackbot_result.
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 RAISE EXCEPTION 'Hackbot history requires restore-based rollback; restore a pre-upgrade backup with its matching release.';
END $$;
-- +goose StatementEnd

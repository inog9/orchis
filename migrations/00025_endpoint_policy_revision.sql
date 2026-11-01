-- +goose Up
-- Revisi keputusan per query membatalkan hasil yang masih berjalan, termasuk cabut lalu setujui ulang.
ALTER TABLE osquery_org_settings ADD COLUMN version bigint NOT NULL DEFAULT 1 CHECK (version > 0);
ALTER TABLE executions ADD COLUMN endpoint_policy jsonb CHECK (endpoint_policy IS NULL OR jsonb_typeof(endpoint_policy)='object');

-- +goose Down
ALTER TABLE executions DROP COLUMN endpoint_policy;
ALTER TABLE osquery_org_settings DROP COLUMN version;

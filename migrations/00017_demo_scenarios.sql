-- +goose Up
-- Perluasan penanda demo: skenario (beserta langkah/edge/target lewat cascade) dan alert.
ALTER TABLE scenarios ADD COLUMN is_demo boolean NOT NULL DEFAULT false;
ALTER TABLE alerts    ADD COLUMN is_demo boolean NOT NULL DEFAULT false;
CREATE INDEX idx_scenarios_demo ON scenarios (tenant_id) WHERE is_demo;
CREATE INDEX idx_alerts_demo    ON alerts    (tenant_id) WHERE is_demo;

-- +goose Down
ALTER TABLE alerts    DROP COLUMN is_demo;
ALTER TABLE scenarios DROP COLUMN is_demo;

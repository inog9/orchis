-- +goose Up
ALTER TABLE demo_seed_state ADD COLUMN format_version integer NOT NULL DEFAULT 1;
ALTER TABLE simulations ADD COLUMN is_demo boolean NOT NULL DEFAULT false;
ALTER TABLE executions ADD COLUMN is_demo boolean NOT NULL DEFAULT false;
-- Synthetic executions are inert history. They cannot become queued or carry raw object keys.
ALTER TABLE executions ADD CONSTRAINT synthetic_execution_inert CHECK(NOT is_demo OR (state='ended' AND raw_output_key IS NULL));
CREATE INDEX idx_simulations_demo ON simulations(tenant_id) WHERE is_demo;
CREATE INDEX idx_executions_demo ON executions(tenant_id) WHERE is_demo;
-- +goose Down
ALTER TABLE demo_seed_state DROP COLUMN format_version;
ALTER TABLE executions DROP CONSTRAINT synthetic_execution_inert;
ALTER TABLE executions DROP COLUMN is_demo;
ALTER TABLE simulations DROP COLUMN is_demo;

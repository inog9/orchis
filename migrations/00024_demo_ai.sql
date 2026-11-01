-- +goose Up
ALTER TABLE ai_policies ADD COLUMN is_demo boolean NOT NULL DEFAULT false;
ALTER TABLE ai_templates ADD COLUMN is_demo boolean NOT NULL DEFAULT false;
ALTER TABLE ai_jobs ADD COLUMN is_demo boolean NOT NULL DEFAULT false;
ALTER TABLE ai_jobs DROP CONSTRAINT ai_jobs_status_check;
ALTER TABLE ai_jobs ADD CONSTRAINT ai_jobs_status_check CHECK(status IN('queued','running','review','completed','failed','canceled','approved','rejected'));
ALTER TABLE ai_jobs ADD CONSTRAINT synthetic_ai_inert CHECK(NOT is_demo OR status IN('review','completed','rejected','canceled'));
CREATE INDEX idx_ai_policies_demo ON ai_policies(tenant_id) WHERE is_demo;
CREATE INDEX idx_ai_templates_demo ON ai_templates(tenant_id) WHERE is_demo;
CREATE INDEX idx_ai_jobs_demo ON ai_jobs(tenant_id) WHERE is_demo;
-- +goose Down
ALTER TABLE ai_jobs DROP CONSTRAINT synthetic_ai_inert;
ALTER TABLE ai_jobs DROP COLUMN is_demo;
ALTER TABLE ai_templates DROP COLUMN is_demo;
ALTER TABLE ai_policies DROP COLUMN is_demo;
-- Forward-only deployment policy: rejected history is not rewritten by downgrade.

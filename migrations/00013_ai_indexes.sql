-- +goose Up
-- Indeks setiap foreign key menjaga operasi referensial tetap terbatas pada tenant terkait.
CREATE INDEX idx_ai_policies_consent ON ai_policies(tenant_id,consent_by);
CREATE INDEX idx_ai_templates_creator ON ai_templates(tenant_id,created_by);
CREATE INDEX idx_ai_jobs_report ON ai_jobs(tenant_id,report_id);
CREATE INDEX idx_ai_jobs_template ON ai_jobs(tenant_id,template_id);
CREATE INDEX idx_ai_jobs_creator ON ai_jobs(tenant_id,created_by);
CREATE INDEX idx_ai_jobs_approver ON ai_jobs(tenant_id,approved_by);
CREATE INDEX idx_ai_actions_asset ON ai_job_actions(tenant_id,asset_id);
CREATE INDEX idx_ai_actions_test ON ai_job_actions(tenant_id,atomic_test_id);
CREATE INDEX idx_ai_actions_execution ON ai_job_actions(tenant_id,execution_id);

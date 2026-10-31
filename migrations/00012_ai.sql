-- +goose Up
CREATE TABLE ai_policies (
 tenant_id uuid NOT NULL, organization_id uuid NOT NULL, version integer NOT NULL DEFAULT 1,
 enabled boolean NOT NULL DEFAULT false, planning boolean NOT NULL DEFAULT false, remediation boolean NOT NULL DEFAULT false,
 planning_model text NOT NULL DEFAULT '',remediation_model text NOT NULL DEFAULT '',
 secrets_enc bytea, consent_by uuid, consent_at timestamptz,
 max_input_bytes integer NOT NULL DEFAULT 32000 CHECK(max_input_bytes BETWEEN 1024 AND 64000),
 max_output_tokens integer NOT NULL DEFAULT 2000 CHECK(max_output_tokens BETWEEN 128 AND 4096),
 daily_tokens integer NOT NULL DEFAULT 20000 CHECK(daily_tokens BETWEEN 128 AND 1000000),
 daily_calls integer NOT NULL DEFAULT 10 CHECK(daily_calls BETWEEN 1 AND 1000),
 daily_cost_micros bigint NOT NULL DEFAULT 0 CHECK(daily_cost_micros BETWEEN 0 AND 100000000),
 input_rate_micros bigint NOT NULL DEFAULT 0 CHECK(input_rate_micros BETWEEN 0 AND 1000000000),
 output_rate_micros bigint NOT NULL DEFAULT 0 CHECK(output_rate_micros BETWEEN 0 AND 1000000000),
 updated_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(tenant_id,organization_id),
 FOREIGN KEY(tenant_id,organization_id) REFERENCES organizations(tenant_id,id),
 FOREIGN KEY(tenant_id,consent_by) REFERENCES users(tenant_id,id)
);
SELECT enable_tenant_rls('ai_policies');
CREATE TABLE ai_templates (
 id uuid PRIMARY KEY,tenant_id uuid NOT NULL REFERENCES tenants(id),kind text NOT NULL CHECK(kind IN('planning','remediation')),
 version integer NOT NULL, instructions text NOT NULL CHECK(length(instructions) BETWEEN 1 AND 6000),
 created_by uuid NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),UNIQUE(tenant_id,id),UNIQUE(tenant_id,kind,version),
 FOREIGN KEY(tenant_id,created_by) REFERENCES users(tenant_id,id)
);
SELECT enable_tenant_rls('ai_templates');
CREATE TABLE ai_jobs (
 id uuid PRIMARY KEY,tenant_id uuid NOT NULL, organization_id uuid NOT NULL, report_id uuid NOT NULL,
 kind text NOT NULL CHECK(kind IN('planning','remediation')), provider text NOT NULL CHECK(provider IN('deepseek','openai')),model text NOT NULL,
 policy_version integer NOT NULL, version integer NOT NULL DEFAULT 1,
 idempotency_key text NOT NULL CHECK(length(idempotency_key) BETWEEN 1 AND 120),request_hash text NOT NULL,
 status text NOT NULL DEFAULT 'queued' CHECK(status IN('queued','running','review','completed','failed','canceled','approved')),
 input jsonb NOT NULL,mapping jsonb NOT NULL,instructions text NOT NULL,prompt_version integer NOT NULL,template_id uuid,
 output jsonb,error text NOT NULL DEFAULT '',claim_hash text,lease_until timestamptz,
 reserved_tokens integer NOT NULL, reserved_cost_micros bigint NOT NULL, input_bytes integer NOT NULL,
 max_output_tokens integer NOT NULL, usage_tokens integer NOT NULL DEFAULT 0,
 created_by uuid NOT NULL,approved_by uuid,approved_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(tenant_id,id),UNIQUE(tenant_id,organization_id,idempotency_key),
 FOREIGN KEY(tenant_id,organization_id) REFERENCES organizations(tenant_id,id),
 FOREIGN KEY(tenant_id,report_id) REFERENCES reports(tenant_id,id),
 FOREIGN KEY(tenant_id,template_id) REFERENCES ai_templates(tenant_id,id),
 FOREIGN KEY(tenant_id,created_by) REFERENCES users(tenant_id,id),FOREIGN KEY(tenant_id,approved_by) REFERENCES users(tenant_id,id)
);
CREATE INDEX idx_ai_jobs_queue ON ai_jobs(tenant_id,status,created_at,id);
CREATE INDEX idx_ai_jobs_budget ON ai_jobs(tenant_id,organization_id,created_at);
SELECT enable_tenant_rls('ai_jobs');
CREATE TABLE ai_job_actions (
 tenant_id uuid NOT NULL, job_id uuid NOT NULL, ordinal integer NOT NULL, asset_id uuid NOT NULL,atomic_test_id uuid NOT NULL,execution_id uuid,
 PRIMARY KEY(tenant_id,job_id,ordinal),
 FOREIGN KEY(tenant_id,job_id) REFERENCES ai_jobs(tenant_id,id),FOREIGN KEY(tenant_id,asset_id) REFERENCES assets(tenant_id,id),
 FOREIGN KEY(tenant_id,atomic_test_id) REFERENCES atomic_tests(tenant_id,id),FOREIGN KEY(tenant_id,execution_id) REFERENCES executions(tenant_id,id)
);
SELECT enable_tenant_rls('ai_job_actions');
UPDATE roles SET permissions=permissions || ARRAY['ai.read'] WHERE is_system;
UPDATE roles SET permissions=permissions || ARRAY['ai.generate','ai.approve'] WHERE is_system AND key IN('admin','analyst');
UPDATE roles SET permissions=permissions || ARRAY['ai.manage'] WHERE is_system AND key='admin';

-- +goose Up
CREATE TABLE action_presets (
 id uuid PRIMARY KEY,tenant_id uuid NOT NULL REFERENCES tenants(id),name text NOT NULL CHECK(length(name) BETWEEN 1 AND 200),
 contract_key text NOT NULL,contract_version integer NOT NULL,params jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),version integer NOT NULL DEFAULT 1,
 UNIQUE(tenant_id,id),UNIQUE(tenant_id,name),FOREIGN KEY(tenant_id,contract_key,contract_version) REFERENCES contracts(tenant_id,key,version)
);
CREATE INDEX idx_presets_contract ON action_presets(tenant_id,contract_key,contract_version);
SELECT enable_tenant_rls('action_presets');

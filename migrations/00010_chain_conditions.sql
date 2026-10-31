-- +goose Up
ALTER TABLE scenario_step_edges ADD COLUMN rule jsonb NOT NULL DEFAULT '{}';
ALTER TABLE simulation_step_edges ADD COLUMN rule jsonb NOT NULL DEFAULT '{}';
ALTER TABLE simulation_targets ADD COLUMN addresses jsonb;
CREATE TABLE simulation_decisions (
 tenant_id uuid NOT NULL,simulation_id uuid NOT NULL,step_id uuid NOT NULL,asset_id uuid NOT NULL,
 status text NOT NULL,reason text NOT NULL,preview jsonb NOT NULL DEFAULT '{}',created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(tenant_id,step_id,asset_id),
 FOREIGN KEY(tenant_id,simulation_id) REFERENCES simulations(tenant_id,id),
 FOREIGN KEY(tenant_id,step_id) REFERENCES simulation_steps(tenant_id,id) ON DELETE CASCADE,
 FOREIGN KEY(tenant_id,asset_id) REFERENCES assets(tenant_id,id)
);
CREATE INDEX idx_decisions_simulation ON simulation_decisions(tenant_id,simulation_id);
CREATE INDEX idx_decisions_asset ON simulation_decisions(tenant_id,asset_id);
SELECT enable_tenant_rls('simulation_decisions');

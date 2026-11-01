-- +goose Up
-- Durable, tenant-scoped deletion intents survive a failed external cleanup.
CREATE TABLE demo_cleanup_tasks (
 tenant_id uuid NOT NULL REFERENCES tenants(id),
 kind text NOT NULL CHECK(kind IN('blob','search')),
 target text NOT NULL CHECK(length(target) BETWEEN 1 AND 300),
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(tenant_id,kind,target)
);
SELECT enable_tenant_rls('demo_cleanup_tasks');

-- +goose Down
DROP TABLE demo_cleanup_tasks;

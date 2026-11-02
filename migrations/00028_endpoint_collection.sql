-- +goose Up
CREATE TABLE endpoint_collection_schedule (
 tenant_id uuid NOT NULL, agent_id uuid NOT NULL, query_key text NOT NULL,
 atomic_test_id uuid NOT NULL, last_enqueued_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(tenant_id,agent_id,query_key),
 FOREIGN KEY(tenant_id,agent_id) REFERENCES agents(tenant_id,id),
 FOREIGN KEY(tenant_id,atomic_test_id) REFERENCES atomic_tests(tenant_id,id)
);
CREATE INDEX idx_endpoint_schedule_test ON endpoint_collection_schedule(tenant_id,atomic_test_id);
SELECT enable_tenant_rls('endpoint_collection_schedule');
-- +goose Down
DROP TABLE endpoint_collection_schedule;

-- +goose Up
ALTER TABLE agents DROP CONSTRAINT agents_control_protocol_check;
ALTER TABLE agents ADD CONSTRAINT agents_control_protocol_check CHECK(control_protocol BETWEEN 0 AND 3);
-- The execution itself is the acknowledgement and authoritative status. No SQL is stored in commands.
CREATE TABLE endpoint_collection_requests (
 tenant_id uuid NOT NULL, execution_id uuid NOT NULL, agent_id uuid NOT NULL, asset_id uuid NOT NULL,
 query_key text NOT NULL, requested_by uuid NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY (tenant_id,execution_id),
 FOREIGN KEY (tenant_id,execution_id) REFERENCES executions(tenant_id,id) ON DELETE CASCADE,
 FOREIGN KEY (tenant_id,agent_id) REFERENCES agents(tenant_id,id) ON DELETE CASCADE,
 FOREIGN KEY (tenant_id,asset_id) REFERENCES assets(tenant_id,id) ON DELETE CASCADE,
 FOREIGN KEY (tenant_id,requested_by) REFERENCES users(tenant_id,id)
);
CREATE INDEX idx_endpoint_collection_request_agent ON endpoint_collection_requests(tenant_id,agent_id,created_at);
SELECT enable_tenant_rls('endpoint_collection_requests');

-- +goose StatementBegin
CREATE FUNCTION notify_endpoint_dataset() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 PERFORM pg_notify('orchis_endpoint_data', json_build_object('tenant_id',NEW.tenant_id,'asset_id',NEW.asset_id,
   'query_key',NEW.query_key,'dataset_id',NEW.asset_id::text || ':' || NEW.query_key,
   'execution_id',NEW.execution_id,'request_id',NEW.execution_id,'collected_at',NEW.collected_at,
   'status',NEW.last_status)::text);
 RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER endpoint_dataset_notify AFTER INSERT OR UPDATE ON endpoint_datasets
 FOR EACH ROW EXECUTE FUNCTION notify_endpoint_dataset();

-- +goose StatementBegin
CREATE FUNCTION notify_endpoint_request() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE item endpoint_collection_requests%ROWTYPE;
BEGIN
 IF TG_TABLE_NAME='endpoint_collection_requests' THEN item := NEW;
 ELSE SELECT * INTO item FROM endpoint_collection_requests WHERE tenant_id=NEW.tenant_id AND execution_id=NEW.id;
 END IF;
 IF item.execution_id IS NOT NULL THEN
  PERFORM pg_notify('orchis_endpoint_data',json_build_object('tenant_id',item.tenant_id,'asset_id',item.asset_id,
   'query_key',item.query_key,'dataset_id',item.asset_id::text || ':' || item.query_key,
   'execution_id',item.execution_id,'request_id',item.execution_id,'status','changed')::text);
 END IF;
 RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER endpoint_request_notify AFTER INSERT ON endpoint_collection_requests
 FOR EACH ROW EXECUTE FUNCTION notify_endpoint_request();
CREATE TRIGGER endpoint_execution_notify AFTER UPDATE OF state ON executions
 FOR EACH ROW WHEN (OLD.state IS DISTINCT FROM NEW.state) EXECUTE FUNCTION notify_endpoint_request();

-- +goose Down
DROP TRIGGER endpoint_execution_notify ON executions;
DROP TRIGGER endpoint_request_notify ON endpoint_collection_requests;
DROP FUNCTION notify_endpoint_request();
DROP TRIGGER endpoint_dataset_notify ON endpoint_datasets;
DROP FUNCTION notify_endpoint_dataset();
DROP TABLE endpoint_collection_requests;
UPDATE agents SET control_protocol=CASE WHEN control_protocol=2 THEN 1 ELSE 0 END WHERE control_protocol IN (2,3);
ALTER TABLE agents DROP CONSTRAINT agents_control_protocol_check;
ALTER TABLE agents ADD CONSTRAINT agents_control_protocol_check CHECK(control_protocol BETWEEN 0 AND 1);

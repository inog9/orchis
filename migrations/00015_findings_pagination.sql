-- +goose Up
-- Match keyset ordering, including deterministic UUID tie-breaking. Previous
-- severity index omitted id and still sorted large timestamp tie groups.
CREATE INDEX idx_findings_page ON findings(tenant_id,last_seen_at DESC,id DESC);
DROP INDEX idx_findings_severity;
CREATE INDEX idx_findings_severity ON findings(tenant_id,severity,last_seen_at DESC,id DESC);

-- +goose Down
DROP INDEX idx_findings_page;
DROP INDEX idx_findings_severity;
CREATE INDEX idx_findings_severity ON findings(tenant_id,severity,last_seen_at DESC);

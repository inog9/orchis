-- +goose Up
-- Dokumen dan identitas pemberi persetujuan tidak diturunkan dari klaim agent.
ALTER TABLE osquery_org_settings
 ADD COLUMN consent_version text,
 ADD COLUMN consent_sha256 text CHECK(consent_sha256 IS NULL OR consent_sha256 ~ '^[0-9a-f]{64}$'),
 ADD COLUMN consented_by uuid,
 ADD COLUMN consented_at timestamptz,
 ADD FOREIGN KEY(tenant_id,consented_by) REFERENCES users(tenant_id,id);

CREATE INDEX idx_osquery_org_settings_consent_actor ON osquery_org_settings(tenant_id,consented_by);

-- +goose Down
ALTER TABLE osquery_org_settings DROP COLUMN consented_at, DROP COLUMN consented_by, DROP COLUMN consent_sha256, DROP COLUMN consent_version;

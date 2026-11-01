-- +goose Up
-- Persetujuan per organisasi untuk query osquery. Tanpa baris: berlaku bawaan katalog (sensitivitas rendah aktif, lainnya
-- perlu persetujuan admin). Query yang tidak disetujui diselesaikan server sebagai "dilewati" saat agent mengambilnya.
CREATE TABLE osquery_org_settings (
  tenant_id       uuid NOT NULL,
  organization_id uuid NOT NULL,
  query_key       text NOT NULL,
  enabled         boolean NOT NULL,
  updated_by      uuid,
  updated_at      timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, organization_id, query_key),
  FOREIGN KEY (tenant_id, organization_id) REFERENCES organizations (tenant_id, id) ON DELETE CASCADE
);
SELECT enable_tenant_rls('osquery_org_settings');

-- +goose Down
DROP TABLE osquery_org_settings;

-- +goose Up
-- Data endpoint dari osquery: hasil TERAKHIR per aset dan query bernama. Hasil dari agent tidak tepercaya; baris di sini sudah
-- disanitasi (kolom skema, panjang sel, redaksi) oleh internal/osquery. Hanya yang terakhir disimpan (tanpa riwayat).
CREATE TABLE endpoint_datasets (
  tenant_id        uuid NOT NULL,
  asset_id         uuid NOT NULL,
  query_key        text NOT NULL,
  rows             jsonb NOT NULL DEFAULT '[]',
  attributes       jsonb NOT NULL DEFAULT '{}',
  truncated        boolean NOT NULL DEFAULT false,
  collected_at     timestamptz,            -- waktu pengambilan hasil ok terakhir (rows/attributes milik pengambilan ini)
  osquery_version  text,
  execution_id     uuid,
  last_status      text NOT NULL CHECK (last_status IN ('ok','skipped','failed')),
  last_reason      text NOT NULL DEFAULT '',
  last_attempt_at  timestamptz NOT NULL DEFAULT now(),
  is_demo          boolean NOT NULL DEFAULT false,
  PRIMARY KEY (tenant_id, asset_id, query_key),
  FOREIGN KEY (tenant_id, asset_id) REFERENCES assets (tenant_id, id) ON DELETE CASCADE
);
SELECT enable_tenant_rls('endpoint_datasets');
CREATE INDEX idx_endpoint_datasets_demo ON endpoint_datasets (tenant_id) WHERE is_demo;

-- +goose Down
DROP TABLE endpoint_datasets;

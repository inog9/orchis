-- +goose Up
-- Baseline per aset untuk deteksi perubahan endpoint (port baru, software baru, akun baru)
-- yang dipakai engine blueteam.
CREATE TABLE endpoint_baselines (
  tenant_id uuid NOT NULL,
  asset_id  uuid NOT NULL,
  query_key text NOT NULL,
  rows      jsonb NOT NULL,
  rows_hash bytea NOT NULL,
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, asset_id, query_key)
);

-- +goose Down
DROP TABLE endpoint_baselines;

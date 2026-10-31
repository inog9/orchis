-- +goose Up
-- Penanda data dummy ("demo"): setiap baris yang dibuat oleh seeder demo diberi is_demo=true,
-- sehingga bisa dihitung dan dihapus bersih tanpa menyentuh data nyata (lihat internal/demo).
ALTER TABLE organizations ADD COLUMN is_demo boolean NOT NULL DEFAULT false;
ALTER TABLE assets        ADD COLUMN is_demo boolean NOT NULL DEFAULT false;
ALTER TABLE agents        ADD COLUMN is_demo boolean NOT NULL DEFAULT false;
ALTER TABLE findings      ADD COLUMN is_demo boolean NOT NULL DEFAULT false;
ALTER TABLE assessments   ADD COLUMN is_demo boolean NOT NULL DEFAULT false;
ALTER TABLE reports       ADD COLUMN is_demo boolean NOT NULL DEFAULT false;
ALTER TABLE tags          ADD COLUMN is_demo boolean NOT NULL DEFAULT false;
ALTER TABLE audit_log     ADD COLUMN is_demo boolean NOT NULL DEFAULT false;

CREATE INDEX idx_organizations_demo ON organizations (tenant_id) WHERE is_demo;
CREATE INDEX idx_assets_demo        ON assets        (tenant_id) WHERE is_demo;
CREATE INDEX idx_agents_demo        ON agents        (tenant_id) WHERE is_demo;
CREATE INDEX idx_findings_demo      ON findings      (tenant_id) WHERE is_demo;
CREATE INDEX idx_assessments_demo   ON assessments   (tenant_id) WHERE is_demo;
CREATE INDEX idx_reports_demo       ON reports       (tenant_id) WHERE is_demo;
CREATE INDEX idx_tags_demo          ON tags          (tenant_id) WHERE is_demo;
CREATE INDEX idx_audit_log_demo     ON audit_log     (tenant_id) WHERE is_demo;

-- Satu baris per tenant: menandai bahwa data demo pernah di-seed dan dengan seed berapa.
-- Dipakai untuk idempotensi (seed sama = tidak menduplikasi) dan untuk mencegah penimpaan diam-diam
-- (seed berbeda tanpa purge dulu akan ditolak oleh internal/demo).
CREATE TABLE demo_seed_state (
  tenant_id   uuid PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
  seed        integer NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);
SELECT enable_tenant_rls('demo_seed_state');

-- +goose Down
DROP TABLE demo_seed_state;
ALTER TABLE audit_log     DROP COLUMN is_demo;
ALTER TABLE tags          DROP COLUMN is_demo;
ALTER TABLE reports       DROP COLUMN is_demo;
ALTER TABLE assessments   DROP COLUMN is_demo;
ALTER TABLE findings      DROP COLUMN is_demo;
ALTER TABLE agents        DROP COLUMN is_demo;
ALTER TABLE assets        DROP COLUMN is_demo;
ALTER TABLE organizations DROP COLUMN is_demo;

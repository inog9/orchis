-- +goose Up
CREATE TABLE tags (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  name        text NOT NULL,
  color       text NOT NULL DEFAULT '#8C919A',
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, name),
  UNIQUE (tenant_id, id)
);

CREATE TABLE assets (
  id                   uuid PRIMARY KEY,
  tenant_id            uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  category             text NOT NULL CHECK (category IN (
                         'host','web_application','cloud_resource','container','network_device',
                         'mobile_device','iot_ot_device','identity','saas_application','ai_target','generic')),
  name                 text NOT NULL,
  description          text,
  platform             text CHECK (platform IN ('windows','linux','macos','ios','android','other')),
  arch                 text CHECK (arch IN ('x86_64','arm64','other')),
  criticality          text NOT NULL DEFAULT 'medium' CHECK (criticality IN ('low','medium','high','critical')),
  is_internet_facing   boolean NOT NULL DEFAULT false,
  testing_authorized   boolean NOT NULL DEFAULT false,   -- harus true agar boleh menjadi sasaran aksi apa pun
  authorized_by        uuid,
  authorized_at        timestamptz,
  source               text NOT NULL DEFAULT 'manual' CHECK (source IN ('manual','agent','csv','api')),
  external_ref         text,
  attributes           jsonb NOT NULL DEFAULT '{}',      -- skema per kategori, divalidasi aplikasi
  created_by           uuid,
  created_at           timestamptz NOT NULL DEFAULT now(),
  updated_at           timestamptz NOT NULL DEFAULT now(),
  version              integer NOT NULL DEFAULT 1,
  deleted_at           timestamptz,
  UNIQUE (tenant_id, id),
  CHECK (NOT testing_authorized OR (authorized_by IS NOT NULL AND authorized_at IS NOT NULL))
);
CREATE INDEX idx_assets_category ON assets (tenant_id, category) WHERE deleted_at IS NULL;
CREATE INDEX idx_assets_platform ON assets (tenant_id, platform, arch) WHERE deleted_at IS NULL;
CREATE INDEX idx_assets_updated  ON assets (tenant_id, updated_at DESC, id) WHERE deleted_at IS NULL;
CREATE INDEX idx_assets_attributes ON assets USING gin (attributes jsonb_path_ops);

-- Alamat yang menjadi sasaran aksi. Penjaga scope mencocokkan sasaran ke tabel ini.
CREATE TABLE asset_addresses (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL,
  asset_id    uuid NOT NULL,
  kind        text NOT NULL CHECK (kind IN ('ip','cidr','hostname','domain','url','mac')),
  value       text NOT NULL,                      -- sudah dinormalisasi (huruf kecil, tanpa spasi)
  is_primary  boolean NOT NULL DEFAULT false,
  created_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, asset_id, kind, value),
  FOREIGN KEY (tenant_id, asset_id) REFERENCES assets (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_asset_addresses_lookup ON asset_addresses (tenant_id, kind, value);

CREATE TABLE asset_tags (
  tenant_id  uuid NOT NULL,
  asset_id   uuid NOT NULL,
  tag_id     uuid NOT NULL,
  PRIMARY KEY (tenant_id, asset_id, tag_id),
  FOREIGN KEY (tenant_id, asset_id) REFERENCES assets (tenant_id, id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, tag_id)   REFERENCES tags (tenant_id, id)   ON DELETE CASCADE
);
CREATE INDEX idx_asset_tags_tag ON asset_tags (tenant_id, tag_id);

CREATE TABLE asset_groups (
  id           uuid PRIMARY KEY,
  tenant_id    uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  name         text NOT NULL,
  description  text,
  kind         text NOT NULL DEFAULT 'dynamic' CHECK (kind IN ('dynamic','static')),
  rule         jsonb NOT NULL DEFAULT '{}',        -- lihat skema aturan di bawah
  is_system    boolean NOT NULL DEFAULT false,     -- grup bawaan per OS/arsitektur
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now(),
  version      integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, name),
  UNIQUE (tenant_id, id)
);

-- Keanggotaan dihitung ulang saat aset atau aturan berubah (bukan dihitung tiap kueri).
CREATE TABLE asset_group_members (
  tenant_id       uuid NOT NULL,
  asset_group_id  uuid NOT NULL,
  asset_id        uuid NOT NULL,
  source          text NOT NULL DEFAULT 'rule' CHECK (source IN ('rule','manual')),
  PRIMARY KEY (tenant_id, asset_group_id, asset_id),
  FOREIGN KEY (tenant_id, asset_group_id) REFERENCES asset_groups (tenant_id, id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, asset_id)       REFERENCES assets (tenant_id, id)       ON DELETE CASCADE
);
CREATE INDEX idx_group_members_asset ON asset_group_members (tenant_id, asset_id);

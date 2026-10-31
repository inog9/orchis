-- +goose Up
CREATE TABLE integrations (
  id                 uuid PRIMARY KEY,
  tenant_id          uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  kind               text NOT NULL CHECK (kind IN ('collector','injector','executor')),
  key                text NOT NULL,                -- nmap | nuclei | manual | agent | mitre_importer
  name               text NOT NULL,
  description        text,
  software_version   text,
  is_builtin         boolean NOT NULL DEFAULT false,
  enabled            boolean NOT NULL DEFAULT true,
  status             text NOT NULL DEFAULT 'unknown' CHECK (status IN ('running','stopped','error','unknown')),
  config             jsonb NOT NULL DEFAULT '{}',
  secrets_enc        bytea,                        -- rahasia konfigurasi, terenkripsi
  key_id             text,
  last_heartbeat_at  timestamptz,
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now(),
  version            integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, key),
  UNIQUE (tenant_id, id)
);

CREATE TABLE integration_logs (
  id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  tenant_id       uuid NOT NULL,
  integration_id  uuid NOT NULL,
  level           text NOT NULL CHECK (level IN ('debug','info','warn','error')),
  message         text NOT NULL,
  at              timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (tenant_id, integration_id) REFERENCES integrations (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_integration_logs ON integration_logs (tenant_id, integration_id, at DESC);

-- Jenis temuan baku (global). Menambah jenis baru = menambah baris, bukan mengubah skema.
CREATE TABLE finding_types (
  key               text PRIMARY KEY,              -- open_port | port_scan | service | vulnerability | url | text
  name              text NOT NULL,
  default_severity  text NOT NULL DEFAULT 'info' CHECK (default_severity IN ('info','low','medium','high','critical'))
);
INSERT INTO finding_types (key, name, default_severity) VALUES
  ('open_port','Open port','info'), ('port_scan','Port scan','info'), ('service','Service','info'),
  ('vulnerability','Vulnerability','medium'), ('url','URL','info'), ('text','Text','info');

-- Kontrak = perjanjian server dengan injector/agent: input apa yang diterima, output apa yang dihasilkan.
CREATE TABLE contracts (
  id                       uuid PRIMARY KEY,
  tenant_id                uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  integration_id           uuid NOT NULL,
  key                      text NOT NULL,          -- contoh: nmap.tcp_connect, nuclei.scan
  version                  integer NOT NULL DEFAULT 1,   -- versi KONTRAK (bukan kontrol konkurensi); berubah bila skema input/output berubah
  name                     text NOT NULL,
  description              text,
  kind                     text NOT NULL CHECK (kind IN ('scan','vulnerability_scan','manual')),
  input_schema             jsonb NOT NULL,         -- JSON Schema untuk params
  output_types             text[] NOT NULL DEFAULT '{}',  -- kunci dari finding_types yang dapat dihasilkan
  platforms                text[] NOT NULL DEFAULT '{}',
  needs_agent              boolean NOT NULL DEFAULT false,
  default_timeout_seconds  integer NOT NULL DEFAULT 600 CHECK (default_timeout_seconds > 0),
  is_active                boolean NOT NULL DEFAULT true,
  created_at               timestamptz NOT NULL DEFAULT now(),
  updated_at               timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, key, version),
  UNIQUE (tenant_id, id),
  FOREIGN KEY (tenant_id, integration_id) REFERENCES integrations (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_contracts_integration ON contracts (tenant_id, integration_id);

-- MITRE ATT&CK (data global, diimpor berkala).
CREATE TABLE mitre_tactics (
  id          text PRIMARY KEY,                    -- TA0007
  stix_id     text UNIQUE,
  name        text NOT NULL,
  shortname   text NOT NULL,
  sort_order  integer NOT NULL DEFAULT 0
);

CREATE TABLE mitre_techniques (
  id             text PRIMARY KEY,                 -- T1046, T1021.002
  stix_id        text UNIQUE,
  name           text NOT NULL,
  description    text,
  parent_id      text REFERENCES mitre_techniques(id) ON DELETE CASCADE,
  tactic_ids     text[] NOT NULL DEFAULT '{}',
  platforms      text[] NOT NULL DEFAULT '{}',
  is_deprecated  boolean NOT NULL DEFAULT false,
  source_version text,
  updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_mitre_techniques_parent ON mitre_techniques (parent_id);
CREATE INDEX idx_mitre_techniques_tactics ON mitre_techniques USING gin (tactic_ids);

CREATE TABLE mitre_import_runs (
  id            uuid PRIMARY KEY,
  source        text NOT NULL,
  source_version text,
  status        text NOT NULL CHECK (status IN ('running','succeeded','failed')),
  techniques    integer,
  tactics       integer,
  error         text,
  started_at    timestamptz NOT NULL DEFAULT now(),
  finished_at   timestamptz
);

CREATE TABLE contract_techniques (
  tenant_id     uuid NOT NULL,
  contract_id   uuid NOT NULL,
  technique_id  text NOT NULL REFERENCES mitre_techniques(id) ON DELETE CASCADE,
  PRIMARY KEY (tenant_id, contract_id, technique_id),
  FOREIGN KEY (tenant_id, contract_id) REFERENCES contracts (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_contract_techniques_technique ON contract_techniques (technique_id);

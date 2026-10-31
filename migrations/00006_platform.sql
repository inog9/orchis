-- +goose Up
-- Tabel menampung keempat tipe agar collector (Prevention/Detection) dan tabletop (Human)
-- bisa ditambahkan nanti tanpa migrasi. MVP hanya menghasilkan 'vulnerability'.
CREATE TABLE expectations (
  id              uuid PRIMARY KEY,
  tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  execution_id    uuid NOT NULL,
  simulation_id   uuid,
  type            text NOT NULL CHECK (type IN ('vulnerability','prevention','detection','human')),
  status          text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','success','failed','expired')),
  expected_score  numeric NOT NULL DEFAULT 100,
  score           numeric,
  evidence        jsonb NOT NULL DEFAULT '{}',
  expires_at      timestamptz,
  evaluated_at    timestamptz,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, execution_id, type),
  FOREIGN KEY (tenant_id, execution_id)  REFERENCES executions (tenant_id, id)  ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, simulation_id) REFERENCES simulations (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_expectations_sim ON expectations (tenant_id, simulation_id, type, status);
CREATE INDEX idx_expectations_expiry ON expectations (expires_at) WHERE status = 'pending';

-- Notifikasi (hanya Alerts di MVP). Audiens ditentukan permission, bukan daftar pengguna.
CREATE TABLE alerts (
  id                   uuid PRIMARY KEY,
  tenant_id            uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  kind                 text NOT NULL,              -- new_critical_finding | test_failed | agent_offline | password_reset_requested ...
  severity             text NOT NULL DEFAULT 'info' CHECK (severity IN ('info','warning','critical')),
  title                text NOT NULL,
  body                 text,
  resource_type        text,
  resource_id          uuid,
  audience_permission  text NOT NULL DEFAULT 'alert.read',
  dedupe_key           text,                       -- mencegah alert ganda untuk kejadian yang sama
  created_at           timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, id)
);
CREATE UNIQUE INDEX uk_alerts_dedupe ON alerts (tenant_id, dedupe_key) WHERE dedupe_key IS NOT NULL;
CREATE INDEX idx_alerts_created ON alerts (tenant_id, created_at DESC);

CREATE TABLE alert_states (
  tenant_id     uuid NOT NULL,
  alert_id      uuid NOT NULL,
  user_id       uuid NOT NULL,
  read_at       timestamptz,
  dismissed_at  timestamptz,
  PRIMARY KEY (tenant_id, alert_id, user_id),
  FOREIGN KEY (tenant_id, alert_id) REFERENCES alerts (tenant_id, id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, user_id)  REFERENCES users (tenant_id, id)  ON DELETE CASCADE
);
CREATE INDEX idx_alert_states_user ON alert_states (tenant_id, user_id);

CREATE TABLE settings (
  tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  key         text NOT NULL,
  value       jsonb NOT NULL,
  updated_by  uuid,
  updated_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, key),
  FOREIGN KEY (tenant_id, updated_by) REFERENCES users (tenant_id, id) ON DELETE SET NULL (updated_by)
);
CREATE INDEX idx_settings_updated_by ON settings (tenant_id, updated_by);

-- Audit: hanya-tambah, dipartisi per bulan, tanpa kunci asing agar bertahan setelah data asal dihapus.
CREATE TABLE audit_log (
  id              uuid NOT NULL,
  tenant_id       uuid NOT NULL,
  at              timestamptz NOT NULL DEFAULT now(),
  actor_user_id   uuid,
  actor_agent_id  uuid,
  action          text NOT NULL,                  -- asset.create | simulation.launch | user.reset_link.create ...
  resource_type   text,
  resource_id     uuid,
  ip              inet,
  request_id      text,
  detail          jsonb NOT NULL DEFAULT '{}',
  PRIMARY KEY (id, at)
) PARTITION BY RANGE (at);
CREATE TABLE audit_log_default PARTITION OF audit_log DEFAULT;
CREATE INDEX idx_audit_tenant_at ON audit_log (tenant_id, at DESC);
CREATE INDEX idx_audit_actor ON audit_log (tenant_id, actor_user_id, at DESC);
CREATE INDEX idx_audit_resource ON audit_log (tenant_id, resource_type, resource_id);

-- Outbox: pesan ditulis dalam transaksi yang sama dengan perubahan datanya, lalu dikirim oleh relay.
CREATE TABLE outbox (
  id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  message_id      uuid NOT NULL UNIQUE,           -- id pesan untuk idempotensi di sisi penerima
  tenant_id       uuid NOT NULL,
  topic           text NOT NULL,                  -- exec.dispatch.nmap | chain.event | index.finding ...
  aggregate_type  text NOT NULL,
  aggregate_id    uuid NOT NULL,
  schema_version  integer NOT NULL DEFAULT 1,
  payload         jsonb NOT NULL,
  headers         jsonb NOT NULL DEFAULT '{}',
  created_at      timestamptz NOT NULL DEFAULT now(),
  available_at    timestamptz NOT NULL DEFAULT now(),
  attempts        integer NOT NULL DEFAULT 0,
  published_at    timestamptz,
  last_error      text
);
CREATE INDEX idx_outbox_pending ON outbox (available_at, id) WHERE published_at IS NULL;

-- Pekerjaan berkala: satu baris per (pekerjaan, interval) menjamin hanya satu instance yang menjalankannya.
CREATE TABLE job_runs (
  job       text NOT NULL,
  bucket    timestamptz NOT NULL,
  instance  text NOT NULL,
  ran_at    timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (job, bucket)
);

-- Penerima mencatat pesan yang sudah diproses, supaya pengiriman ulang tidak diproses dua kali.
CREATE TABLE processed_messages (
  consumer      text NOT NULL,
  message_id    uuid NOT NULL,
  processed_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (consumer, message_id)
);
CREATE INDEX idx_processed_messages_at ON processed_messages (processed_at);

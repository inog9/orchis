-- +goose Up
CREATE EXTENSION IF NOT EXISTS citext;

CREATE TABLE tenants (
  id          uuid PRIMARY KEY,
  name        text NOT NULL UNIQUE,
  created_at  timestamptz NOT NULL DEFAULT now(),
  deleted_at  timestamptz
);

CREATE TABLE roles (
  id           uuid PRIMARY KEY,
  tenant_id    uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  key          text NOT NULL,                      -- admin | analyst | viewer | kustom
  name         text NOT NULL,
  description  text,
  permissions  text[] NOT NULL DEFAULT '{}',       -- contoh: asset.write, simulation.run
  is_system    boolean NOT NULL DEFAULT false,     -- peran bawaan tidak bisa dihapus
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, key),
  UNIQUE (tenant_id, id)
);

CREATE TABLE users (
  id               uuid PRIMARY KEY,
  tenant_id        uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  email            citext NOT NULL,
  display_name     text NOT NULL,
  kind             text NOT NULL DEFAULT 'human' CHECK (kind IN ('human','service')),
  status           text NOT NULL DEFAULT 'active' CHECK (status IN ('active','disabled')),
  role_id          uuid NOT NULL,
  password_hash    text,                           -- argon2id; NULL untuk akun service
  last_login_at    timestamptz,
  last_login_ip    inet,
  locale           text,
  home_dashboard   text,
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now(),
  version          integer NOT NULL DEFAULT 1,
  deleted_at       timestamptz,
  UNIQUE (tenant_id, id),
  FOREIGN KEY (tenant_id, role_id) REFERENCES roles (tenant_id, id)
);
CREATE UNIQUE INDEX uk_users_email ON users (tenant_id, email) WHERE deleted_at IS NULL;
CREATE INDEX idx_users_role ON users (tenant_id, role_id);

-- Lima password terakhir, untuk aturan "tidak boleh sama dengan 5 sebelumnya".
CREATE TABLE password_history (
  id             uuid PRIMARY KEY,
  tenant_id      uuid NOT NULL,
  user_id        uuid NOT NULL,
  password_hash  text NOT NULL,
  created_at     timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (tenant_id, user_id) REFERENCES users (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_password_history_user ON password_history (tenant_id, user_id, created_at DESC);

-- Pembatasan percobaan login per email. Dicatat juga untuk email yang TIDAK terdaftar, sehingga
-- perilaku penguncian tidak membocorkan keberadaan akun.
CREATE TABLE login_throttle (
  tenant_id        uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  email            citext NOT NULL,
  failed_attempts  integer NOT NULL DEFAULT 0,
  first_failed_at  timestamptz,
  locked_until     timestamptz,                    -- 5 kegagalan -> terkunci 15 menit
  last_ip          inet,
  updated_at       timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, email)
);

CREATE TABLE sessions (
  id                   uuid PRIMARY KEY,
  tenant_id            uuid NOT NULL,
  user_id              uuid NOT NULL,
  token_hash           bytea NOT NULL UNIQUE,      -- hash dari token acak 256-bit; token asli hanya di cookie
  remember_me          boolean NOT NULL DEFAULT false,
  ip                   inet,
  user_agent           text,
  created_at           timestamptz NOT NULL DEFAULT now(),
  last_seen_at         timestamptz NOT NULL DEFAULT now(),
  idle_expires_at      timestamptz NOT NULL,       -- 30 menit tanpa aktivitas
  absolute_expires_at  timestamptz NOT NULL,       -- 30 hari bila remember_me
  revoked_at           timestamptz,
  FOREIGN KEY (tenant_id, user_id) REFERENCES users (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_sessions_user ON sessions (tenant_id, user_id);
CREATE INDEX idx_sessions_expiry ON sessions (idle_expires_at);

-- Link reset yang dibuat admin (tanpa SMTP). Token asli hanya tampil sekali.
CREATE TABLE password_reset_links (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL,
  user_id     uuid NOT NULL,
  created_by  uuid NOT NULL,
  token_hash  bytea NOT NULL UNIQUE,
  expires_at  timestamptz NOT NULL,                -- 30 menit
  used_at     timestamptz,
  created_at  timestamptz NOT NULL DEFAULT now(),
  CHECK (expires_at > created_at),
  FOREIGN KEY (tenant_id, user_id)    REFERENCES users (tenant_id, id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, created_by) REFERENCES users (tenant_id, id)
);
CREATE INDEX idx_reset_links_user ON password_reset_links (tenant_id, user_id);
CREATE INDEX idx_reset_links_created_by ON password_reset_links (tenant_id, created_by);

CREATE TABLE api_tokens (
  id            uuid PRIMARY KEY,
  tenant_id     uuid NOT NULL,
  user_id       uuid NOT NULL,
  name          text NOT NULL,
  token_prefix  text NOT NULL,                     -- 8 karakter pertama, untuk ditampilkan
  token_hash    bytea NOT NULL UNIQUE,
  last_used_at  timestamptz,
  expires_at    timestamptz,
  revoked_at    timestamptz,
  created_at    timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (tenant_id, user_id) REFERENCES users (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_api_tokens_user ON api_tokens (tenant_id, user_id);

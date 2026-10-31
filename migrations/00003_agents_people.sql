-- +goose Up
CREATE TABLE agents (
  id                 uuid PRIMARY KEY,
  tenant_id          uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  asset_id           uuid NOT NULL,
  hostname           text,
  os                 text NOT NULL CHECK (os IN ('windows','linux','macos')),
  arch               text NOT NULL CHECK (arch IN ('x86_64','arm64')),
  mode               text NOT NULL CHECK (mode IN ('service','user')),
  version            text NOT NULL,
  protocol_version   integer NOT NULL,              -- versi protokol agent (lihat protokol agent)
  status             text NOT NULL DEFAULT 'online' CHECK (status IN ('online','offline','revoked')),
  secret_hash        bytea NOT NULL,                -- hash kredensial per-agent; kredensial asli hanya ada di agent
  last_heartbeat_at  timestamptz,
  last_ip            inet,
  enrolled_at        timestamptz NOT NULL DEFAULT now(),
  revoked_at         timestamptz,
  revoked_by         uuid,
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, id),
  FOREIGN KEY (tenant_id, asset_id) REFERENCES assets (tenant_id, id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, revoked_by) REFERENCES users (tenant_id, id)
);
CREATE INDEX idx_agents_asset ON agents (tenant_id, asset_id);
CREATE INDEX idx_agents_revoked_by ON agents (tenant_id, revoked_by);
CREATE INDEX idx_agents_heartbeat ON agents (status, last_heartbeat_at);
-- Satu agent aktif per aset dan mode.
CREATE UNIQUE INDEX uk_agents_active ON agents (tenant_id, asset_id, mode) WHERE status <> 'revoked';

CREATE TABLE agent_enrollment_tokens (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  token_hash  bytea NOT NULL UNIQUE,
  os          text NOT NULL CHECK (os IN ('windows','linux','macos')),
  arch        text NOT NULL CHECK (arch IN ('x86_64','arm64')),
  mode        text NOT NULL CHECK (mode IN ('service','user')),
  created_by  uuid NOT NULL,
  expires_at  timestamptz NOT NULL,
  used_at     timestamptz,
  agent_id    uuid,                                 -- terisi saat dipakai
  created_at  timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (tenant_id, created_by) REFERENCES users (tenant_id, id),
  FOREIGN KEY (tenant_id, agent_id)   REFERENCES agents (tenant_id, id) ON DELETE SET NULL (agent_id)
);
CREATE INDEX idx_enrollment_created_by ON agent_enrollment_tokens (tenant_id, created_by);
CREATE INDEX idx_enrollment_agent ON agent_enrollment_tokens (tenant_id, agent_id);

-- Peserta latihan. TIDAK bisa login; akun login ada di tabel users.
CREATE TABLE people (
  id            uuid PRIMARY KEY,
  tenant_id     uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  first_name    text NOT NULL,
  last_name     text NOT NULL DEFAULT '',
  email         citext,
  organization  text,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now(),
  version       integer NOT NULL DEFAULT 1,
  deleted_at    timestamptz,
  UNIQUE (tenant_id, id)
);
CREATE UNIQUE INDEX uk_people_email ON people (tenant_id, email) WHERE deleted_at IS NULL AND email IS NOT NULL;

CREATE TABLE teams (
  id           uuid PRIMARY KEY,
  tenant_id    uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  name         text NOT NULL,
  description  text,
  kind         text NOT NULL DEFAULT 'internal' CHECK (kind IN ('internal','external')),
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now(),
  version      integer NOT NULL DEFAULT 1,
  deleted_at   timestamptz,
  UNIQUE (tenant_id, id)
);
CREATE UNIQUE INDEX uk_teams_name ON teams (tenant_id, name) WHERE deleted_at IS NULL;

CREATE TABLE team_members (
  tenant_id  uuid NOT NULL,
  team_id    uuid NOT NULL,
  person_id  uuid NOT NULL,
  PRIMARY KEY (tenant_id, team_id, person_id),
  FOREIGN KEY (tenant_id, team_id)   REFERENCES teams (tenant_id, id)  ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, person_id) REFERENCES people (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_team_members_person ON team_members (tenant_id, person_id);

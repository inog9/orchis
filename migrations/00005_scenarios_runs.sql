-- +goose Up
CREATE TABLE scenarios (
  id           uuid PRIMARY KEY,
  tenant_id    uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  name         text NOT NULL,
  description  text,
  severity     text NOT NULL DEFAULT 'medium' CHECK (severity IN ('low','medium','high','critical')),
  category     text,
  main_focus   text,
  status       text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','ready')),
  created_by   uuid,
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now(),
  version      integer NOT NULL DEFAULT 1,
  deleted_at   timestamptz,
  UNIQUE (tenant_id, id),
  FOREIGN KEY (tenant_id, created_by) REFERENCES users (tenant_id, id)
);
CREATE UNIQUE INDEX uk_scenarios_name ON scenarios (tenant_id, name) WHERE deleted_at IS NULL;
CREATE INDEX idx_scenarios_created_by ON scenarios (tenant_id, created_by);
CREATE INDEX idx_scenarios_updated ON scenarios (tenant_id, updated_at DESC, id) WHERE deleted_at IS NULL;

-- Perimeter sasaran chain: satu perimeter bersama untuk seluruh langkah.
CREATE TABLE scenario_targets (
  id              uuid PRIMARY KEY,
  tenant_id       uuid NOT NULL,
  scenario_id     uuid NOT NULL,
  asset_id        uuid,
  asset_group_id  uuid,
  CHECK ((asset_id IS NOT NULL)::int + (asset_group_id IS NOT NULL)::int = 1),
  FOREIGN KEY (tenant_id, scenario_id)    REFERENCES scenarios (tenant_id, id)    ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, asset_id)       REFERENCES assets (tenant_id, id)       ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, asset_group_id) REFERENCES asset_groups (tenant_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX uk_scenario_target_asset ON scenario_targets (scenario_id, asset_id) WHERE asset_id IS NOT NULL;
CREATE UNIQUE INDEX uk_scenario_target_group ON scenario_targets (scenario_id, asset_group_id) WHERE asset_group_id IS NOT NULL;
CREATE INDEX idx_scenario_targets_scenario ON scenario_targets (tenant_id, scenario_id);
CREATE INDEX idx_scenario_targets_asset ON scenario_targets (tenant_id, asset_id);
CREATE INDEX idx_scenario_targets_group ON scenario_targets (tenant_id, asset_group_id);

CREATE TABLE scenario_steps (
  id               uuid PRIMARY KEY,
  tenant_id        uuid NOT NULL,
  scenario_id      uuid NOT NULL,
  key              text NOT NULL,                 -- kunci stabil dalam scenario (dipakai UI dan impor)
  title            text NOT NULL,
  description      text,
  contract_id      uuid NOT NULL,
  contract_version integer NOT NULL,
  params           jsonb NOT NULL DEFAULT '{}',   -- divalidasi terhadap contracts.input_schema
  delay_seconds    integer NOT NULL DEFAULT 0 CHECK (delay_seconds >= 0),
  trigger_mode     text NOT NULL DEFAULT 'after_parents' CHECK (trigger_mode IN ('after_parents','on_finding','manual')),
  trigger          jsonb,                         -- aturan temuan, wajib bila trigger_mode = on_finding
  mappings         jsonb NOT NULL DEFAULT '[]',   -- memetakan nilai finding ke kunci input
  is_enabled       boolean NOT NULL DEFAULT true,
  position_x       numeric NOT NULL DEFAULT 0,
  position_y       numeric NOT NULL DEFAULT 0,
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, scenario_id, key),
  UNIQUE (tenant_id, id),
  CHECK (trigger_mode <> 'on_finding' OR trigger IS NOT NULL),
  FOREIGN KEY (tenant_id, scenario_id) REFERENCES scenarios (tenant_id, id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, contract_id) REFERENCES contracts (tenant_id, id)
);
CREATE INDEX idx_scenario_steps_scenario ON scenario_steps (tenant_id, scenario_id);
CREATE INDEX idx_scenario_steps_contract ON scenario_steps (tenant_id, contract_id);

CREATE TABLE scenario_step_edges (
  tenant_id        uuid NOT NULL,
  scenario_id      uuid NOT NULL,
  parent_step_id   uuid NOT NULL,
  child_step_id    uuid NOT NULL,
  PRIMARY KEY (tenant_id, parent_step_id, child_step_id),
  CHECK (parent_step_id <> child_step_id),
  FOREIGN KEY (tenant_id, scenario_id)   REFERENCES scenarios (tenant_id, id)      ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, parent_step_id) REFERENCES scenario_steps (tenant_id, id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, child_step_id)  REFERENCES scenario_steps (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_scenario_edges_scenario ON scenario_step_edges (tenant_id, scenario_id);
CREATE INDEX idx_scenario_edges_child ON scenario_step_edges (tenant_id, child_step_id);

CREATE TABLE scenario_teams (
  tenant_id    uuid NOT NULL,
  scenario_id  uuid NOT NULL,
  team_id      uuid NOT NULL,
  PRIMARY KEY (tenant_id, scenario_id, team_id),
  FOREIGN KEY (tenant_id, scenario_id) REFERENCES scenarios (tenant_id, id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, team_id)     REFERENCES teams (tenant_id, id)     ON DELETE CASCADE
);
CREATE INDEX idx_scenario_teams_team ON scenario_teams (tenant_id, team_id);

CREATE TABLE scenario_tags (
  tenant_id    uuid NOT NULL,
  scenario_id  uuid NOT NULL,
  tag_id       uuid NOT NULL,
  PRIMARY KEY (tenant_id, scenario_id, tag_id),
  FOREIGN KEY (tenant_id, scenario_id) REFERENCES scenarios (tenant_id, id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, tag_id)      REFERENCES tags (tenant_id, id)      ON DELETE CASCADE
);
CREATE INDEX idx_scenario_tags_tag ON scenario_tags (tenant_id, tag_id);

CREATE SEQUENCE simulation_number_seq;

CREATE TABLE simulations (
  id               uuid PRIMARY KEY,
  tenant_id        uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  number           bigint NOT NULL DEFAULT nextval('simulation_number_seq'),   -- tampil sebagai OP-0142
  scenario_id      uuid,                                  -- sumber; boleh dihapus lunak, riwayat tetap
  name             text NOT NULL,
  status           text NOT NULL DEFAULT 'draft' CHECK (status IN
                     ('draft','scheduled','running','paused','finished','canceled','failed')),
  scheduled_for    timestamptz,
  started_at       timestamptz,
  ended_at         timestamptz,
  timeout_seconds  integer NOT NULL DEFAULT 3600 CHECK (timeout_seconds > 0),
  safe_mode        boolean NOT NULL DEFAULT true,
  launched_by      uuid,
  canceled_by      uuid,
  cancel_reason    text,
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now(),
  version          integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  UNIQUE (tenant_id, number),
  FOREIGN KEY (tenant_id, scenario_id)  REFERENCES scenarios (tenant_id, id) ON DELETE SET NULL (scenario_id),
  FOREIGN KEY (tenant_id, launched_by)  REFERENCES users (tenant_id, id),
  FOREIGN KEY (tenant_id, canceled_by)  REFERENCES users (tenant_id, id)
);
CREATE INDEX idx_simulations_status ON simulations (tenant_id, status, created_at DESC, id);
CREATE INDEX idx_simulations_scenario ON simulations (tenant_id, scenario_id);
CREATE INDEX idx_simulations_launched_by ON simulations (tenant_id, launched_by);
CREATE INDEX idx_simulations_canceled_by ON simulations (tenant_id, canceled_by);
CREATE INDEX idx_simulations_schedule ON simulations (scheduled_for) WHERE status = 'scheduled';

-- Aset sasaran yang sudah diurai saat peluncuran (grup dinamis dibekukan pada saat itu).
CREATE TABLE simulation_targets (
  tenant_id      uuid NOT NULL,
  simulation_id  uuid NOT NULL,
  asset_id       uuid NOT NULL,
  PRIMARY KEY (tenant_id, simulation_id, asset_id),
  FOREIGN KEY (tenant_id, simulation_id) REFERENCES simulations (tenant_id, id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, asset_id)      REFERENCES assets (tenant_id, id)
);
CREATE INDEX idx_simulation_targets_asset ON simulation_targets (tenant_id, asset_id);

CREATE TABLE simulation_steps (
  id                uuid PRIMARY KEY,
  tenant_id         uuid NOT NULL,
  simulation_id     uuid NOT NULL,
  key               text NOT NULL,
  title             text NOT NULL,
  contract_id       uuid NOT NULL,
  contract_version  integer NOT NULL,
  params            jsonb NOT NULL DEFAULT '{}',
  delay_seconds     integer NOT NULL DEFAULT 0,
  trigger_mode      text NOT NULL CHECK (trigger_mode IN ('after_parents','on_finding','manual')),
  trigger           jsonb,
  mappings          jsonb NOT NULL DEFAULT '[]',
  is_enabled        boolean NOT NULL DEFAULT true,
  position_x        numeric NOT NULL DEFAULT 0,
  position_y        numeric NOT NULL DEFAULT 0,
  UNIQUE (tenant_id, simulation_id, key),
  UNIQUE (tenant_id, id),
  FOREIGN KEY (tenant_id, simulation_id) REFERENCES simulations (tenant_id, id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, contract_id)   REFERENCES contracts (tenant_id, id)
);
CREATE INDEX idx_simulation_steps_sim ON simulation_steps (tenant_id, simulation_id);
CREATE INDEX idx_simulation_steps_contract ON simulation_steps (tenant_id, contract_id);

CREATE TABLE simulation_step_edges (
  tenant_id       uuid NOT NULL,
  simulation_id   uuid NOT NULL,
  parent_step_id  uuid NOT NULL,
  child_step_id   uuid NOT NULL,
  PRIMARY KEY (tenant_id, parent_step_id, child_step_id),
  CHECK (parent_step_id <> child_step_id),
  FOREIGN KEY (tenant_id, simulation_id)  REFERENCES simulations (tenant_id, id)      ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, parent_step_id) REFERENCES simulation_steps (tenant_id, id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, child_step_id)  REFERENCES simulation_steps (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_sim_edges_sim ON simulation_step_edges (tenant_id, simulation_id);
CREATE INDEX idx_sim_edges_child ON simulation_step_edges (tenant_id, child_step_id);

CREATE TABLE atomic_tests (
  id                uuid PRIMARY KEY,
  tenant_id         uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  name              text NOT NULL,
  contract_id       uuid NOT NULL,
  contract_version  integer NOT NULL,
  params            jsonb NOT NULL DEFAULT '{}',
  target_asset_id   uuid,                         -- boleh kosong sampai target dipilih
  created_by        uuid,
  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now(),
  version           integer NOT NULL DEFAULT 1,
  deleted_at        timestamptz,
  UNIQUE (tenant_id, id),
  FOREIGN KEY (tenant_id, contract_id)     REFERENCES contracts (tenant_id, id),
  FOREIGN KEY (tenant_id, target_asset_id) REFERENCES assets (tenant_id, id) ON DELETE SET NULL (target_asset_id),
  FOREIGN KEY (tenant_id, created_by)      REFERENCES users (tenant_id, id)
);
CREATE INDEX idx_atomic_tests_contract ON atomic_tests (tenant_id, contract_id);
CREATE INDEX idx_atomic_tests_target ON atomic_tests (tenant_id, target_asset_id);
CREATE INDEX idx_atomic_tests_created_by ON atomic_tests (tenant_id, created_by);

-- Satu pengiriman tugas ke satu sasaran. Inti mesin chain.
CREATE TABLE executions (
  id                  uuid PRIMARY KEY,
  tenant_id           uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  simulation_id       uuid,
  simulation_step_id  uuid,
  atomic_test_id      uuid,
  contract_id         uuid NOT NULL,
  contract_version    integer NOT NULL,
  target_asset_id     uuid NOT NULL,
  target_address      text NOT NULL,              -- alamat yang benar-benar dituju (sudah lolos penjaga scope)
  executor_kind       text NOT NULL CHECK (executor_kind IN ('injector','agent','manual')),
  integration_id      uuid,
  agent_id            uuid,
  state               text NOT NULL DEFAULT 'ready' CHECK (state IN ('ready','running','ended')),
  outcome             text CHECK (outcome IN ('success','error','canceled','timeout','skipped')),
  params              jsonb NOT NULL DEFAULT '{}',   -- input akhir setelah mapping
  input_hash          bytea NOT NULL,                -- sidik jari input: mencegah eksekusi ganda
  not_before          timestamptz NOT NULL DEFAULT now(),
  attempt             integer NOT NULL DEFAULT 0,
  max_attempts        integer NOT NULL DEFAULT 1,
  ready_at            timestamptz NOT NULL DEFAULT now(),
  started_at          timestamptz,
  ended_at            timestamptz,
  exit_code           integer,
  error               text,
  raw_output_key      text,                          -- objek di MinIO
  lease_until         timestamptz,                   -- sewa saat running; reaper mengembalikan ke ready bila lewat
  claimed_by          text,
  created_at          timestamptz NOT NULL DEFAULT now(),
  updated_at          timestamptz NOT NULL DEFAULT now(),
  version             integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  CHECK ((simulation_id IS NOT NULL)::int + (atomic_test_id IS NOT NULL)::int = 1),
  CHECK ((state = 'ended') = (outcome IS NOT NULL)),
  CHECK (simulation_id IS NULL OR simulation_step_id IS NOT NULL),
  FOREIGN KEY (tenant_id, simulation_id)      REFERENCES simulations (tenant_id, id)      ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, simulation_step_id) REFERENCES simulation_steps (tenant_id, id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, atomic_test_id)     REFERENCES atomic_tests (tenant_id, id)     ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, contract_id)        REFERENCES contracts (tenant_id, id),
  FOREIGN KEY (tenant_id, target_asset_id)    REFERENCES assets (tenant_id, id),
  FOREIGN KEY (tenant_id, integration_id)     REFERENCES integrations (tenant_id, id),
  FOREIGN KEY (tenant_id, agent_id)           REFERENCES agents (tenant_id, id)
);
-- Idempotensi: satu langkah, satu sasaran, satu input = satu eksekusi.
CREATE UNIQUE INDEX uk_executions_idem ON executions (tenant_id, simulation_step_id, target_asset_id, input_hash)
  WHERE simulation_step_id IS NOT NULL;
-- Antrean pengambilan tugas (SELECT ... FOR UPDATE SKIP LOCKED).
CREATE INDEX idx_executions_ready ON executions (tenant_id, not_before) WHERE state = 'ready';
CREATE INDEX idx_executions_running ON executions (started_at) WHERE state = 'running';
CREATE INDEX idx_executions_lease ON executions (lease_until) WHERE state = 'running';
CREATE INDEX idx_executions_sim ON executions (tenant_id, simulation_id, state);
CREATE INDEX idx_executions_step ON executions (tenant_id, simulation_step_id);
CREATE INDEX idx_executions_atomic ON executions (tenant_id, atomic_test_id, created_at DESC);
CREATE INDEX idx_executions_contract ON executions (tenant_id, contract_id);
CREATE INDEX idx_executions_target ON executions (tenant_id, target_asset_id);
CREATE INDEX idx_executions_integration ON executions (tenant_id, integration_id);
CREATE INDEX idx_executions_agent ON executions (tenant_id, agent_id, state);

CREATE TABLE execution_logs (
  id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  tenant_id     uuid NOT NULL,
  execution_id  uuid NOT NULL,
  at            timestamptz NOT NULL DEFAULT now(),
  level         text NOT NULL CHECK (level IN ('debug','info','warn','error')),
  message       text NOT NULL,
  FOREIGN KEY (tenant_id, execution_id) REFERENCES executions (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_execution_logs ON execution_logs (tenant_id, execution_id, at);

CREATE TABLE findings (
  id                uuid PRIMARY KEY,
  tenant_id         uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  asset_id          uuid,
  type              text NOT NULL REFERENCES finding_types(key),
  value             text NOT NULL,
  value_hash        bytea NOT NULL,                 -- sha256 dari nilai yang sudah dinormalisasi
  title             text NOT NULL,
  severity          text NOT NULL DEFAULT 'info' CHECK (severity IN ('info','low','medium','high','critical')),
  details           jsonb NOT NULL DEFAULT '{}',
  first_seen_at     timestamptz NOT NULL DEFAULT now(),
  last_seen_at      timestamptz NOT NULL DEFAULT now(),
  occurrence_count  integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  FOREIGN KEY (tenant_id, asset_id) REFERENCES assets (tenant_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX uk_findings_asset ON findings (tenant_id, asset_id, type, value_hash) WHERE asset_id IS NOT NULL;
CREATE UNIQUE INDEX uk_findings_global ON findings (tenant_id, type, value_hash) WHERE asset_id IS NULL;
CREATE INDEX idx_findings_asset ON findings (tenant_id, asset_id);
CREATE INDEX idx_findings_type ON findings (tenant_id, type);
CREATE INDEX idx_findings_type_fk ON findings (type);          -- pendukung FK ke finding_types
CREATE INDEX idx_findings_severity ON findings (tenant_id, severity, last_seen_at DESC);

-- Setiap kali sebuah eksekusi menemukan finding itu lagi. Daftar finding per simulasi bersumber dari sini.
CREATE TABLE finding_occurrences (
  id             uuid PRIMARY KEY,
  tenant_id      uuid NOT NULL,
  finding_id     uuid NOT NULL,
  execution_id   uuid NOT NULL,
  simulation_id  uuid,
  seen_at        timestamptz NOT NULL DEFAULT now(),
  evidence_key   text,                              -- objek bukti di MinIO
  details        jsonb NOT NULL DEFAULT '{}',
  UNIQUE (tenant_id, finding_id, execution_id),
  FOREIGN KEY (tenant_id, finding_id)    REFERENCES findings (tenant_id, id)    ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, execution_id)  REFERENCES executions (tenant_id, id)  ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, simulation_id) REFERENCES simulations (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_occurrences_execution ON finding_occurrences (tenant_id, execution_id);
CREATE INDEX idx_occurrences_simulation ON finding_occurrences (tenant_id, simulation_id, seen_at DESC);

-- Aksi lanjutan yang dipicu sebuah finding (untuk kolom "Follow-up" dan attack path).
CREATE TABLE finding_triggers (
  tenant_id     uuid NOT NULL,
  finding_id    uuid NOT NULL,
  execution_id  uuid NOT NULL,
  PRIMARY KEY (tenant_id, finding_id, execution_id),
  FOREIGN KEY (tenant_id, finding_id)   REFERENCES findings (tenant_id, id)   ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, execution_id) REFERENCES executions (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_finding_triggers_execution ON finding_triggers (tenant_id, execution_id);

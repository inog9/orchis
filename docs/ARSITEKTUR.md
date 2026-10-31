# Arsitektur dan desain database: orchis-go

Dokumen ini adalah pondasi proyek. Skemanya diturunkan dari skema **live** OpenAEV 3.261023.0 (176 tabel, dibaca dari database deployment pada 2026-10-29), dipangkas sesuai cakupan MVP di [RENCANA.md](RENCANA.md).

Status: **implementasi berjalan.** Skema awal terdiri dari 52 tabel dan telah diuji pada PostgreSQL 17; penambahan Fase 9 dijelaskan di bagian 19–24. Berkas migrasi adalah sumber kebenaran untuk skema yang diterapkan. Aturan RLS, FK komposit, dan CHECK diuji dengan data.

| Bagian | Isi |
|---|---|
| 1–2 | Keputusan ringkas dan konvensi |
| 3–4 | Skema: akun dan akses, taksonomi, aset |
| 5–6 | Skema: agent, orang dan tim, integrasi, kontrak, MITRE |
| 7 | Skema: scenario, chain, simulasi, eksekusi, finding |
| 8 | Skema: expectation, notifikasi, pengaturan, audit, outbox, RLS |
| 9 | State machine |
| 10 | Catatan keputusan (ADR) |

Kontrak yang belum ditulis (ditulis sebelum fase yang membutuhkannya): kontrak injector (Fase 3), protokol agent (Fase 4), kontrak pesan antrean dan API (Fase 3 dan 1).

---

## 1. Keputusan ringkas

| Hal | Keputusan | Beda dari OpenAEV |
|---|---|---|
| Ukuran skema | 52 tabel (termasuk tabel penghubung) | OpenAEV 176 tabel |
| Tipe ID | `uuid` versi 7 (urut waktu), dibuat oleh aplikasi | `varchar` |
| Tenant | Kolom `tenant_id` di semua tabel bisnis; satu tenant aktif di MVP | Ditambahkan belakangan, dua mekanisme isolasi |
| Akun vs orang | `users` (login) dan `people` (peserta) terpisah | Digabung di satu tabel `users` |
| Mesin eksekusi | Satu: langkah chain | Dua mesin tumpang tindih |
| Data langkah | Kolom bertipe | Blob JSON `step_data` |
| Kontrak Nuclei | Satu kontrak berparameter | Satu baris per CVE (sekitar 4.800) |
| Attack path | Diturunkan dari eksekusi, finding, dan tepi chain | Tiga tabel `attackpath_*` terpisah |
| Kepemilikan chain | Kolom `kind` yang eksplisit | Penanda teks di deskripsi |
| Sumber kebenaran | PostgreSQL | Dua sumber (PostgreSQL dan Elasticsearch) |

---

## 2. Konvensi

### 2.1 Penamaan
- Tabel: `snake_case` jamak (`asset_groups`). Tabel penghubung: `<a>_<b>` (`team_members`).
- Kolom: `snake_case` **tanpa awalan nama entitas**. Cukup `name`, bukan `asset_name` seperti di OpenAEV. Pengguna kolom selalu lewat nama tabel.
- Kunci asing: `<entitas_tunggal>_id` (`asset_id`).
- Boolean: awalan `is_` atau `has_`. Waktu: akhiran `_at`. Durasi: akhiran satuan (`_seconds`, `_ms`).
- Constraint dan indeks: `pk_`, `uk_`, `fk_`, `ck_`, `idx_` diikuti nama tabel dan kolom.

### 2.2 Tipe dan nilai
- **ID:** `uuid` v7, dibuat di aplikasi (urut waktu, baik untuk indeks). Tidak ada `serial` kecuali nomor tampilan seperti `OP-0142`, yang memakai sequence terpisah.
- **Waktu:** semua `timestamptz`, disimpan dan dikirim dalam UTC (RFC 3339). Tidak ada `timestamp` tanpa zona.
- **Enum:** kolom `text` dengan `CHECK (... IN (...))`, bukan tipe enum PostgreSQL (mengubah enum PostgreSQL sulit). Konstanta Go mencerminkannya.
- **JSONB:** hanya untuk atribut fleksibel yang **divalidasi aplikasi** (atribut aset, skema kontrak, parameter langkah). Dilarang untuk relasi atau data yang perlu di-join. Setiap kolom JSONB punya skema terdokumentasi.
- **Teks bebas:** `text`, bukan `varchar(n)`. Batas panjang ditegakkan di aplikasi dan dengan `CHECK` bila perlu.
- **Rahasia:** disimpan terenkripsi (AES-256-GCM) di kolom `*_enc bytea` bersama `key_id text` agar kunci bisa dirotasi.

### 2.3 Kolom standar
Setiap tabel bisnis memiliki:

| Kolom | Tipe | Catatan |
|---|---|---|
| `id` | `uuid` PK | |
| `tenant_id` | `uuid NOT NULL` | FK ke `tenants` |
| `created_at` | `timestamptz NOT NULL DEFAULT now()` | |
| `updated_at` | `timestamptz NOT NULL DEFAULT now()` | Diperbarui trigger |
| `version` | `integer NOT NULL DEFAULT 1` | Hanya pada entitas yang bisa diedit; dipakai untuk kontrol konkurensi optimistis (respons 409 bila versi tidak cocok) |
| `deleted_at` | `timestamptz NULL` | Hanya pada entitas yang dirujuk riwayat (lihat 2.5) |

### 2.4 Tenant
- Semua tabel bisnis punya `tenant_id`. Pengecualian: data global (`mitre_*`, `tenants`).
- Setiap tabel yang menjadi tujuan kunci asing mendefinisikan `UNIQUE (tenant_id, id)`, dan kunci asing ditulis **komposit** `(tenant_id, x_id)`. Dengan begitu database sendiri menolak referensi lintas tenant.
- Keunikan nama selalu per tenant: `UNIQUE (tenant_id, name)`.
- **Row-Level Security (RLS)** diaktifkan di semua tabel bisnis dengan kebijakan `tenant_id = current_setting('app.tenant_id')::uuid`. Aplikasi menjalankan `SET LOCAL app.tenant_id` di awal tiap transaksi. Ini menjadi lapisan pengaman kedua bila ada kueri yang lupa menyaring tenant.
- MVP hanya punya satu tenant, tetapi semua mekanisme di atas sudah aktif sejak migrasi pertama.

### 2.5 Penghapusan dan retensi
- **Hapus lunak** (`deleted_at`) hanya untuk `users`, `people`, `teams`, `assets`, `scenarios`, karena riwayat simulasi merujuknya. Keunikan memakai indeks parsial `WHERE deleted_at IS NULL`.
- Data hasil jalan (simulasi, eksekusi, finding) tidak dihapus pengguna; dibersihkan oleh job retensi dengan batas yang bisa diatur.
- `audit_log` hanya-tambah, dipartisi bulanan.

### 2.6 Indeks
- **Setiap kunci asing punya indeks pendukung** (pelajaran dari OpenAEV: kunci asing tanpa indeks membuat penghapusan induk memindai seluruh tabel anak).
- Daftar yang dipaginasi memakai paginasi cursor (keyset); indeks dibuat sesuai urutan yang dipakai `(created_at, id)`.

### 2.7 Migrasi
- Alat: `goose`, berkas SQL, **hanya maju**. Satu perubahan per berkas, nama `YYYYMMDDHHMMSS_deskripsi.sql`.
- Berkas yang sudah diterapkan tidak boleh diubah.
- Perubahan merusak dilakukan dengan pola expand/contract: tambah kolom baru → isi → pindahkan pembaca → hapus kolom lama pada rilis berikutnya.

### 2.8 API dan kesalahan
- Prefiks `/api/v1`. JSON `snake_case`, ID berupa string, waktu RFC 3339 UTC.
- Daftar: `?limit=&cursor=` dan respons `{ "items": [], "next_cursor": null }`.
- Kesalahan memakai `application/problem+json`:
  `{ "type", "title", "status", "detail", "error_id": "err_7f3a91c2", "request_id", "fields": [{ "name", "message" }] }`
  `error_id` sama dengan yang ditampilkan di layar 500 pada desain.
- Skema API ditulis sebagai OpenAPI; tipe TypeScript frontend dibuat otomatis darinya.
- Pembuatan resource yang bisa terulang (misalnya menjalankan simulasi) menerima header `Idempotency-Key`.

### 2.9 Izin
Kode memeriksa **permission** (`asset.write`, `simulation.run`), tidak pernah nama peran. Peran hanyalah kumpulan permission. Dengan begitu peran kustom di masa depan tidak mengubah kode.

---

## 3. Skema: akun dan akses

Diturunkan dari `users`, `tokens`, `roles`, `roles_capabilities`, `tenants`, `user_events` di OpenAEV. Perubahan: akun login dipisah dari peserta, peran berupa kumpulan permission, sesi disimpan sebagai hash, riwayat password dan penguncian akun menjadi kolom/tabel eksplisit.

```sql
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
```

Catatan desain:
- Pesan kesalahan login selalu generik ("Email atau password salah") dan tidak menyebut sisa percobaan, karena angka itu hanya bisa ditampilkan untuk akun yang ada. Ini menyimpang dari desain `orchis.pen` (frame 11.1) demi tidak membocorkan keberadaan akun; layar terkunci (frame 11.2) tetap dipakai.
- Token API mewarisi izin pemiliknya; tidak ada izin sendiri.
- Aturan "minimal satu Admin aktif" dan "tidak menaikkan peran sendiri" ditegakkan di layanan, dengan tes.
- Admin pertama dibuat dari variabel lingkungan saat boot pertama; perintah `server reset-admin` untuk keadaan darurat.
- Seed peran bawaan: `admin`, `analyst`, `viewer` dengan permission sesuai matriks di RENCANA.md bagian 3.5.

---

## 4. Skema: taksonomi dan aset

Diturunkan dari `tags`, `assets`, `assets_tags`, `asset_groups`, `asset_groups_assets`. Perubahan utama: satu tabel `assets` dengan kolom inti bertipe, atribut tiap kategori di `attributes` (JSONB, divalidasi), alamat jaringan dinormalisasi ke `asset_addresses`, dan **otorisasi pengujian eksplisit** per aset.

```sql
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
```

Skema aturan grup dinamis (`asset_groups.rule`), divalidasi aplikasi:
```json
{ "mode": "and",
  "filters": [
    { "field": "platform", "operator": "eq", "values": ["windows"] },
    { "field": "arch",     "operator": "eq", "values": ["x86_64"] }
  ] }
```
- `mode`: `and` | `or`.
- `field` yang diizinkan: `category`, `platform`, `arch`, `criticality`, `tag`, `is_internet_facing`, `hostname`.
- `operator`: `eq`, `neq`, `in`, `not_in`, `contains`, `is_empty`, `is_not_empty`.

Atribut per kategori (`assets.attributes`), disimpan sebagai skema JSON di `docs/schemas/asset-<kategori>.json` dan divalidasi saat simpan:

| Kategori | Atribut utama |
|---|---|
| host | `os_version`, `is_eol`, `agent_state` (turunan), `mac_addresses` |
| web_application | `base_url`, `technologies`, `auth_required` |
| cloud_resource | `provider`, `service_type`, `region`, `account_ref` |
| container | `runtime`, `image`, `orchestrator`, `namespace` |
| network_device | `vendor`, `model`, `firmware`, `management_address` |
| mobile_device | `os_version`, `managed_by` |
| iot_ot_device | `protocol`, `vendor`, `firmware`, `zone` |
| identity | `directory`, `account_type`, `principal` |
| saas_application | `vendor`, `tenant_ref`, `sso_enabled` |
| ai_target | `provider`, `endpoint`, `model`, `modality`, `secret_ref` (rahasia disimpan terenkripsi, bukan di sini) |
| generic | bebas, dengan batas ukuran |

Aturan penjaga scope (dipakai semua jalur pengiriman tugas): sebuah aksi hanya boleh dikirim bila **setiap sasarannya** berupa aset dengan `testing_authorized = true`, belum dihapus, dan alamat sasaran ada di `asset_addresses` aset tersebut. Pemeriksaan dilakukan saat simulasi diluncurkan **dan** lagi saat tiap tugas dikirim.

---

## 5. Skema: agent, orang dan tim

Diturunkan dari `agents`, `asset_agent_jobs`, `users` (sisi peserta), `teams`, `users_teams`, `organizations`. Perubahan: agent punya kredensial sendiri (bukan token admin), pendaftaran memakai token sekali pakai, peserta tidak bisa login.

```sql
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
```

Tugas untuk agent **tidak** memakai tabel tersendiri seperti `asset_agent_jobs` di OpenAEV: agent mengambil baris `executions` yang ditujukan kepadanya (bagian 7).

---

## 6. Skema: integrasi, kontrak, dan MITRE

Diturunkan dari `injectors`, `injectors_contracts`, `collectors`, `executors`, `connector_instances`, `connector_instance_logs`, `attack_patterns`, `kill_chain_phases`. Perubahan: satu tabel `integrations` untuk ketiga jenis, kontrak berversi dengan skema input (JSON Schema) dan jenis output yang baku, MITRE sebagai data global.

```sql
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
```

Perbedaan dari OpenAEV pada kontrak: satu kontrak `nuclei.scan` menerima parameter (`template_ids`, `tags`, `severity`) lewat `input_schema`, bukan satu baris per CVE (OpenAEV memiliki sekitar 4.800 baris kontrak, hampir semuanya Nuclei). Daftar CVE yang bisa dipilih dibaca dari data template, bukan dari tabel kontrak.

---

## 7. Skema: scenario, chain, simulasi, eksekusi, finding

Diturunkan dari `scenarios`, `exercises`, `injects`, `injects_statuses`, `execution_traces`, `workflows`, `steps`, `conditions`, `findings`, `attackpath_*`. Ini bagian yang paling banyak diubah:

- **Scenario adalah chain.** Hanya satu mesin (chain), jadi tidak ada pembedaan "scenario time-based" vs "chain". Halaman *Scenarios* dan *Attack chains* adalah dua tampilan atas entitas yang sama.
- **Template dan jalan dipisah, dan jalan memakai salinan beku.** Saat simulasi diluncurkan, langkah dan tepi scenario disalin ke `simulation_steps` dan `simulation_step_edges`. Mengedit scenario sesudahnya tidak mengubah simulasi yang sudah berjalan.
- **Satu tabel `executions`** untuk setiap pengiriman tugas (menggantikan `injects` + `injects_statuses` + `steps` bertipe RUN + `execution_traces`).
- **Finding dideduplikasi**, setiap kemunculan dicatat terpisah.
- **Attack path tidak punya tabel.** Ia diturunkan dari `simulation_steps`, `simulation_step_edges`, `executions`, `findings`, dan `finding_triggers`.

### 7.1 Template

```sql
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
```

Skema JSONB:
- `scenario_steps.trigger`: `{ "mode": "and|or", "rules": [{ "finding_type": "open_port", "operator": "eq|neq|is_null|is_not_null|gt|gte|lt|lte|in|nin", "value": "445", "case_sensitive": true }] }`, maksimal 20 aturan.
- `scenario_steps.mappings`: `[{ "input_key": "target_port", "finding_type": "open_port" }]`, maksimal 20; `input_key` cocok `[A-Za-z0-9_.-]{1,255}` dan ada di `input_schema` kontrak.
- Validasi graf (di layanan, dengan tes): tanpa siklus, tanpa langkah yatim, maksimal 100 langkah, perimeter tidak kosong sebelum diluncurkan.

### 7.2 Simulasi (jalan)

```sql
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
```

### 7.3 Atomic test dan eksekusi

```sql
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
```

### 7.4 Finding

```sql
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
```

Catatan finding:
- Nilai dinormalisasi sebelum di-hash (huruf kecil untuk hostname, bentuk kanonik untuk `host:port`, CVE huruf besar).
- Satu finding yang sama di dua simulasi = satu baris `findings` dan dua baris `finding_occurrences`. Di daftar per simulasi, "ditemukan oleh" berasal dari `finding_occurrences → executions → simulation_steps`.
- Seed `finding_types`: `open_port`, `port_scan`, `service`, `vulnerability`, `url`, `text`.

---

## 8. Skema: expectation, notifikasi, pengaturan, audit, outbox

```sql
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
```

### 8.1 Pengaman tenant (Row-Level Security)

Diterapkan lewat fungsi `enable_tenant_rls(tabel)`. **Setiap migrasi yang membuat tabel ber-`tenant_id` wajib memanggilnya**; tes `TestEveryTenantTableHasRLS` menggagalkan build bila ada tabel yang terlewat.

```sql
CREATE FUNCTION enable_tenant_rls(tbl regclass) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
  EXECUTE format('ALTER TABLE %s ENABLE ROW LEVEL SECURITY', tbl);
  EXECUTE format('ALTER TABLE %s FORCE ROW LEVEL SECURITY', tbl);
  EXECUTE format('DROP POLICY IF EXISTS tenant_isolation ON %s', tbl);
  EXECUTE format(
    'CREATE POLICY tenant_isolation ON %s USING (tenant_id = NULLIF(current_setting(''app.tenant_id'', true), '''')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting(''app.tenant_id'', true), '''')::uuid)',
    tbl);
END $$;

DO $$
DECLARE t record;
BEGIN
  FOR t IN
    SELECT c.table_name FROM information_schema.columns c
    JOIN information_schema.tables tb USING (table_schema, table_name)
    WHERE c.table_schema = 'public' AND c.column_name = 'tenant_id'
      AND tb.table_type = 'BASE TABLE' AND c.table_name NOT LIKE 'audit_log\_%'
  LOOP
    PERFORM enable_tenant_rls(quote_ident(t.table_name)::regclass);
  END LOOP;
END $$;
```

Aturan operasional:
- Aplikasi tersambung dengan peran database `orchis_app` (bukan pemilik tabel). Peran pemilik hanya dipakai migrasi.
- Setiap transaksi memulai dengan `SET LOCAL app.tenant_id = '<uuid>'`. Tanpa itu, kueri tidak mengembalikan baris (gagal aman).
- Pekerjaan lintas tenant (misalnya job retensi) memakai peran terpisah `orchis_maintenance` dengan `BYPASSRLS`, dipakai secara eksplisit dan tercatat.
- `tenants`, `mitre_*`, dan `finding_types` bersifat global dan tidak memakai RLS.

---

## 9. State machine

Setiap transisi dilakukan dengan satu pernyataan SQL bersyarat (`UPDATE ... WHERE state = <asal>` dan memeriksa jumlah baris), sehingga transisi ganda atau terlambat otomatis ditolak. Versi kolom `version` naik di setiap transisi.

### 9.1 Eksekusi
```
            ┌─────────────── cancel ───────────────┐
            │                                       ▼
 [ready] ──claim──▶ [running] ──selesai/gagal──▶ [ended: success|error|timeout|canceled]
    │                   │ ▲
    │                   │ └── retry (attempt < max_attempts): kembali ke ready dengan not_before baru
    └─ skip (langkah dinonaktifkan / kondisi tidak terpenuhi) ──▶ [ended: skipped]
```
| Dari | Ke | Pemicu | Syarat |
|---|---|---|---|
| (baru) | ready | Chain membuat eksekusi | Penjaga scope lolos; input_hash belum ada |
| ready | running | Injector/agent mengambil tugas | `not_before <= now()`; `FOR UPDATE SKIP LOCKED` |
| running | ended(success/error) | Hasil diterima | Hanya dari pemilik tugas (agent_id/integration_id cocok) |
| running | ended(timeout) | Job pemeriksa | `started_at + timeout < now()` |
| running | ready | Retry | `attempt < max_attempts`, kegagalan sementara |
| ready/running | ended(canceled) | Pengguna menghentikan simulasi | |
| ready | ended(skipped) | Engine | Langkah dinonaktifkan |

Status akhir (`ended`) tidak pernah berubah lagi.

### 9.2 Simulasi
```
 [draft] ─schedule─▶ [scheduled] ─start─▶ [running] ⇄ pause/resume ⇄ [paused]
    │                    │                    │
    └──── cancel ────────┴──── cancel ────────┤
                                              ├─ semua eksekusi ended ──▶ [finished]
                                              ├─ timeout / kesalahan fatal ─▶ [failed]
                                              └─ stop pengguna ─────────▶ [canceled]
```
- `running → finished` bila tidak ada eksekusi `ready`/`running` dan tidak ada langkah yang masih bisa dipicu.
- Saat `paused`: tidak ada eksekusi baru yang di-claim; yang sedang berjalan diselesaikan.
- `finished`, `failed`, `canceled` bersifat akhir. Simulasi yang sudah berjalan tidak dapat diedit; yang `draft`/`scheduled` bisa.
- Saat `canceled`/`failed`: semua eksekusi `ready` menjadi `ended(canceled)`; yang `running` diberi sinyal berhenti.

### 9.3 Langkah (dihitung, bukan disimpan)
Status langkah pada simulasi adalah turunan dari eksekusinya, tidak ada kolom sendiri:
| Status | Aturan |
|---|---|
| waiting | Ada induk yang belum selesai, atau aturan trigger belum terpenuhi |
| queued | Ada eksekusi `ready` |
| running | Ada eksekusi `running` |
| done | Semua eksekusi `ended` dan minimal satu `success` |
| failed | Semua eksekusi `ended` tanpa `success` |
| skipped | Langkah dinonaktifkan |
| blocked | Induknya `failed` dan langkah ini tidak punya trigger alternatif |

Aturan join: sebuah langkah baru eligible bila **semua** induknya `done` atau `failed` (selesai terminal), dan, untuk `trigger_mode = on_finding`, aturan trigger terpenuhi oleh finding yang ada pada simulasi tersebut. Induk dengan banyak sasaran dianggap selesai bila seluruh eksekusinya sudah `ended`.

### 9.4 Agent
```
 [enroll token dipakai] ─▶ [online] ⇄ (heartbeat hilang > 3×interval) ⇄ [offline]
                              └─────────── revoke ──────────▶ [revoked]  (akhir)
```
Agent `revoked` ditolak di semua endpoint dan tidak bisa dipulihkan; pemulihan = pendaftaran ulang dengan token baru.

### 9.5 Integrasi
`unknown → running ⇄ stopped`, `running → error` bila heartbeat gagal lebih dari ambang; kembali `running` bila heartbeat pulih.

---

## 10. Catatan keputusan (ADR)

Format: konteks → keputusan → konsekuensi. Berkas lengkap per ADR akan dipindah ke `docs/adr/` saat kode dimulai; ringkasan di bawah menjadi sumber awalnya.

| # | Keputusan | Alasan | Konsekuensi |
|---|---|---|---|
| 001 | Server, agent, dan injector ditulis dengan **Go** | Satu bahasa, binary tunggal, cross-compile mudah; agent OpenAEV (Rust) membuktikan polanya, bukan bahasanya | Berbagi kode antar komponen; perlu penandatanganan kode untuk menghindari salah deteksi AV |
| 002 | **PostgreSQL satu-satunya sumber kebenaran**; Elasticsearch hanya indeks | Dua sumber kebenaran menimbulkan ketidaksinkronan yang sulit dipulihkan | Indeks dapat dibangun ulang kapan saja dari PostgreSQL |
| 003 | **Pola outbox** untuk semua pesan keluar (RabbitMQ dan indeks) | Menulis data dan mengirim pesan tidak bisa atomik tanpa outbox | Relay perlu dijalankan dan dipantau; penerima wajib idempoten (`processed_messages`) |
| 004 | **Satu mesin eksekusi** (chain) | OpenAEV memelihara dua mesin yang tumpang tindih | Scenario = chain; "time-based" tidak ada di MVP |
| 005 | Simulasi memakai **salinan beku** graf scenario | Mengedit scenario tidak boleh mengubah simulasi yang berjalan | Tabel `simulation_steps`/`edges` menggandakan data, dengan imbalan hasil yang konsisten dan kueri sederhana |
| 006 | `tenant_id` + **FK komposit** + **RLS** sejak migrasi pertama | Multi-tenancy yang ditambahkan belakangan menghasilkan dua mekanisme isolasi di OpenAEV | Setiap kueri berjalan dalam transaksi dengan `app.tenant_id`; sedikit overhead untuk keamanan ekstra |
| 007 | **UUIDv7** untuk semua ID | Urut waktu (baik untuk indeks), tidak bisa ditebak | Dibuat di aplikasi |
| 008 | **`users` dan `people` dipisah** | Akun login dan peserta latihan punya siklus hidup dan risiko berbeda | Dua tabel; peserta tidak punya kredensial |
| 009 | Enum = `text` + `CHECK`; jenis finding = tabel rujukan | Mengubah enum PostgreSQL mahal; menambah nilai harus murah | Konstanta Go dijaga sejalan lewat tes |
| 010 | **Kontrak berversi**; satu kontrak Nuclei berparameter | OpenAEV menyimpan sekitar 4.800 baris kontrak per CVE | Daftar CVE dibaca dari data template |
| 011 | Reset password lewat **link buatan admin**, tanpa SMTP | Keputusan produk: tanpa email | Alert "permintaan reset" untuk admin; perintah CLI darurat |
| 012 | MVP menampilkan **Vulnerability saja**; tipe expectation lain disiapkan di skema | Prevention/Detection butuh collector EDR/SIEM | Satu penanda fitur menyembunyikan tampilannya |
| 013 | **Penjaga scope tunggal** dan flag `testing_authorized` per aset | Aksi hanya boleh menyasar aset yang diotorisasi eksplisit | Setiap jalur pengiriman tugas wajib melewati fungsi yang sama; diperiksa saat peluncuran dan saat pengiriman |
| 014 | Izin berbasis **permission**, peran = kumpulan permission | Peran kustom nanti tidak mengubah kode | Tiga peran bawaan di-seed |
| 015 | Migrasi **hanya maju** dengan pola expand/contract | Rollback skema di produksi berbahaya | Perubahan merusak butuh dua rilis |
| 016 | Attack path **diturunkan**, tanpa tabel sendiri | Tiga tabel `attackpath_*` di OpenAEV menduplikasi data yang sudah ada | Kueri graf dibangun dari langkah, eksekusi, finding; dapat di-cache bila lambat |

---

## 11. Hasil spike (2026-10-29)

Kode percobaan ada di `spikes/` dan dibuang setelah pola dipindah ke kode produksi. Semua dijalankan pada mesin ini terhadap PostgreSQL 17, RabbitMQ 4.4, dan Elasticsearch 8.19 asli.

### 11.1 Penjadwal: lulus, dengan satu perubahan rencana
Pengujian: 4 instance berebut 2.000 tugas, pekerja yang mati setelah klaim, go-quartz pada 3 instance, dan latensi.

| Hasil | Nilai |
|---|---|
| Tugas diklaim ganda | 0 dari 2.000 (selesai dalam 169 ms) |
| Tugas milik pekerja yang mati | Dikembalikan oleh reaper (lease habis) dan diselesaikan pekerja lain, 100 dari 100 |
| Pekerjaan berkala (go-quartz, 3 instance) | 6 eksekusi dalam 6 detik, bukan 18, berkat satu baris `job_runs` per interval |
| Latensi tugas jatuh tempo → terambil | Rata-rata 110 ms, terburuk 116 ms (polling 200 ms) |

**Keputusan:** penjadwalan eksekusi dan simulasi terjadwal memakai PostgreSQL (`FOR UPDATE SKIP LOCKED` dengan lease dan reaper). **go-quartz hanya dipakai untuk pekerjaan berkala** (sweeper heartbeat, retensi, impor MITRE, pemeriksa timeout), dengan pengaman satu-eksekusi-per-interval lewat tabel `job_runs`. go-quartz tidak menyimpan jadwal secara permanen dan tidak mendukung klaster, jadi sumber kebenaran jadwal tetap di tabel (`simulations.scheduled_for`, `executions.not_before`).

Tabel dan kolom yang diperlukan sudah dimasukkan ke skema: `job_runs` (bagian 8) serta `lease_until` dan `claimed_by` pada `executions` (bagian 7.3). Reaper mengembalikan baris `running` yang `lease_until`-nya sudah lewat.

### 11.2 Outbox → RabbitMQ → Elasticsearch: lulus
| Hasil | Nilai |
|---|---|
| Relay mati setelah mengirim 300 pesan tanpa menandai terkirim | Tidak ada yang hilang; 300 duplikat sengaja muncul (2.301 diterima untuk 2.000 unik) |
| Penerima idempoten | Tepat 2.000 diproses; `processed_messages` berisi 2.000 ID unik |
| Pesan beracun | Masuk DLQ (1), tidak memblokir antrean |
| Indeks ES | 2.000 dokumen; dihapus lalu dibangun ulang penuh dari PostgreSQL dengan bulk, 2.000 lagi |
| Throughput | Sekitar 175 pesan/detik dengan satu penulisan ES per pesan (bulk akan jauh lebih cepat) |

Aturan yang terbukti perlu: klaim outbox dalam transaksi pendek **tanpa I/O jaringan**, kirim ke RabbitMQ di luar transaksi dengan *publisher confirms*, tandai terkirim di transaksi kedua; ID dokumen ES deterministik (ID agregat) sebagai pengaman kedua.

### 11.3 Agent lintas OS: lulus di Linux, belum terbukti di Windows dan macOS
Agent Go statis (CGO mati) dipasang sebagai service systemd sungguhan. 21 pemeriksaan lulus:

- Pendaftaran dengan token sekali pakai; config berizin 600.
- Heartbeat dan eksekusi tugas.
- Update sah 1.0.0 → 1.1.0: proses diganti, service tetap aktif.
- Ditolak: rilis bertanda tangan palsu, downgrade, dan biner bertanda tangan sah tetapi rusak (uji asap).
- Kredensial yang dicabut dikenali.
- Kompilasi 6 target (linux/windows/darwin × amd64/arm64), ukuran 5,7–6,4 MB.

**Bug yang ditemukan spike dan wajib diingat:** setelah `rename` biner yang sedang berjalan, `os.Executable()` di Linux menunjuk ke nama baru (`.old`). Jalur biner harus dihitung sekali saat start. Tanpa itu update menimpa berkas yang salah dan service rusak. Pelajaran tambahan: biner baru wajib diuji asap (`version`) sebelum menggantikan yang lama, dan tanda tangan mengikat versi + OS/arsitektur + hash (bukan hash saja).

**Temuan library:** `kardianos/service` menulis `RestartSec=120` secara permanen di unit systemd. Pemulihan crash jadi 2 menit. Pilihan: tulis unit sendiri untuk Linux (OpenAEV memakai 60 dtk), atau panggil restart eksplisit pada update (sudah dilakukan di spike).

**Yang belum terbukti:** perilaku service di Windows (SCM) dan macOS (launchd), termasuk mengganti biner yang berjalan di Windows (rename, bukan timpa). Keduanya hanya dikompilasi di sini. Harus diuji di mesin sungguhan sebelum Fase 4 dinyatakan selesai.

### 11.4 Dampak pada ADR
- **ADR-017:** Penjadwalan eksekusi dan simulasi terjadwal memakai PostgreSQL dengan lease; go-quartz hanya untuk pekerjaan berkala.
- **ADR-018:** Update agent: ditarik saat heartbeat, tanda tangan Ed25519 atas (versi, OS/arsitektur, hash), tolak downgrade, uji asap sebelum pasang, rename dua langkah, restart eksplisit.

---

## 12. Fase 2: aset, grup, tag, impor (sudah dibangun)

### 12.1 API (semua di `/api/v1`, wajib login, izin per endpoint)
| Metode dan jalur | Izin | Keterangan |
|---|---|---|
| `GET /assets` | `asset.read` | Pencarian nama, filter kategori, paginasi cursor |
| `GET /assets/{id}` | `asset.read` | Alamat, tag, dan grup anggota |
| `POST /assets` | `asset.write` | `testing_authorized=true` butuh `asset.authorize` |
| `PATCH /assets/{id}` | `asset.write` | Wajib mengirim `version`; salah = 409. Hanya kolom yang dikirim yang diubah |
| `DELETE /assets/{id}` | `asset.delete` | Hapus lunak; keluar dari semua grup |
| `POST /assets/import/preview` | `asset.write` | Saran pemetaan kolom dan validasi; tidak menulis apa pun |
| `POST /assets/import` | `asset.write` | Satu transaksi; baris tak valid ditolak kecuali `skip_invalid` |
| `GET /tags`, `POST /tags` | baca / `asset.write` | Nama unik per tenant (huruf besar-kecil tidak dibedakan di pencarian aturan) |
| `GET /asset-groups`, `GET /asset-groups/{id}` | `asset.read` | Detail memuat 50 anggota pertama |
| `POST /asset-groups/preview` | `asset.read` | Berapa aset yang cocok, tanpa menyimpan |
| `POST /asset-groups`, `PUT /asset-groups/{id}` | `asset.write` | `PUT` wajib `version` |
| `DELETE /asset-groups/{id}` | `asset.delete` | Grup bawaan: 409 |

### 12.2 Keputusan tambahan
| # | Keputusan | Alasan | Konsekuensi |
|---|---|---|---|
| 019 | Keanggotaan grup **dihitung ulang secara sinkron** di transaksi yang sama dengan perubahan aset (`groups.RecomputeAll`) | Daftar anggota selalu konsisten dengan aset; tidak ada keadaan setengah jalan | Biaya naik seiring jumlah grup. Batas yang wajar: puluhan grup dan ribuan aset. Jika terlampaui, pindah ke penghitungan ulang berbasis antrean (outbox) |
| 020 | **Kontrol konkurensi optimistis** lewat kolom `version` pada aset dan grup | Dua orang mengedit aset yang sama tidak boleh saling menimpa diam-diam | Klien wajib mengirim `version`; UI menawarkan "muat versi terbaru" saat 409 |
| 021 | Aturan grup **tidak pernah disusun dari teks pengguna**: kolom dan operator berasal dari daftar putih, nilai hanya lewat parameter SQL | Mencegah injeksi SQL pada fitur yang membangun kueri dinamis | Tes memastikan nilai berbahaya tidak muncul di teks SQL |
| 022 | Aset hasil impor **tidak pernah** berstatus `testing_authorized` | Otorisasi pengujian adalah keputusan keamanan terpisah per aset | Setelah impor, admin mengotorisasi aset satu per satu |
| 023 | Impor CSV **divalidasi ulang di server** saat komit, tidak mempercayai hasil pratinjau | Klien bisa dimanipulasi | Pratinjau dan komit memakai fungsi analisis yang sama |

### 12.3 Aturan grup (yang dibangun)
Kolom: `category`, `platform`, `arch`, `criticality`, `tag`, `is_internet_facing`, `hostname`. Operator: `eq`, `neq`, `in`, `not_in`, `contains` (hanya `hostname`), `is_empty`, `is_not_empty`. Mode `and` atau `or`, maksimal 10 aturan. Grup bawaan: `All endpoints` (platform tidak kosong) dan satu grup untuk tiap kombinasi Linux/macOS/Windows × arm64/x86_64.

### 12.4 Batasan yang diketahui
- Pencarian aset hanya berdasarkan **nama** (bukan alamat IP). Pencarian alamat akan ditambahkan bersama indeks yang sesuai.
- Grup bertipe `static` (anggota manual) belum didukung; kolom `kind` sudah menyiapkannya.
- Daftar grup tidak dipaginasi (maksimal 200); respons tetap berbentuk `{items, next_cursor}` agar kompatibel nanti.
- Nilai sel CSV yang diawali `=`, `+`, `-`, `@` disimpan apa adanya. Bila nanti ada **ekspor** CSV, nilai itu harus dinetralkan agar tidak dieksekusi sebagai rumus.

---

## 13. Fase 3: atomic test, injector, finding (sudah dibangun)

### 13.1 Alur
```
UI/API  POST /atomic-tests/{id}/run
   │  1 transaksi: penjaga scope -> INSERT executions(ready) + INSERT outbox(exec.dispatch) + audit
   ▼
outbox ──Relay (publisher confirm)──▶ RabbitMQ exchange orchis.events ──▶ antrean exec.nmap | exec.nuclei
                                                                               │
Injector (container, TANPA akses database)  ◀──────────────────────────────────┘
   │ POST /api/v1/injector/executions/{id}/claim     (ready->running, lease, penjaga scope DIPERIKSA ULANG)
   │ jalankan nmap/nuclei (tanpa shell, argumen dari daftar putih, timeout, grup proses)
   │ POST .../heartbeat tiap 10 dtk                   (perpanjang lease; balasan {cancel:true} bila dibatalkan)
   │ POST .../result                                  (running->ended + finding, satu transaksi)
   ▼
PostgreSQL (findings, finding_occurrences, executions, execution_logs) + MinIO (keluaran mentah)
   └─ outbox index.finding ──Relay──▶ antrean index.finding ──Indexer──▶ Elasticsearch (salinan; bisa dibangun ulang)
```
Server, relay, indexer, dan sweeper berjalan dalam satu proses (`server serve`). Injector berupa biner terpisah (`cmd/injector`, image `deploy/injector/Dockerfile`) yang hanya berbicara HTTP ke server dan AMQP ke RabbitMQ.

### 13.2 API (di bawah `/api/v1`)
| Endpoint | Izin | Catatan |
|---|---|---|
| `GET /contracts` | atomic.read | Kontrak + status injector (`online` bila heartbeat < 60 dtk) |
| `GET/POST /atomic-tests`, `GET/PATCH/DELETE /atomic-tests/{id}` | atomic.read / atomic.write / atomic.delete | PATCH memakai `version` (409 bila bentrok); DELETE = soft delete |
| `POST /atomic-tests/{id}/run` | atomic.run | 202; 422 bila aset belum diotorisasi/alamat ditolak; 409 bila masih berjalan |
| `GET /executions/{id}`, `POST /executions/{id}/cancel` | atomic.read / atomic.run | Detail memuat log dan finding |
| `GET /findings`, `GET /findings/{id}` | finding.read | Filter `type`, `severity`, `asset_id`, `q`; kursor; `search` = `elasticsearch`/`database`/`none` |
| `POST /injector/heartbeat`, `.../executions/{id}/claim\|heartbeat\|result` | token bearer injector | Cookie diabaikan; CSRF tidak berlaku (header Authorization tidak bisa dipalsukan lintas situs) |

Perintah CLI: `server injector-token <nmap\|nuclei>` (token baru, yang lama langsung hangus; hanya hash SHA-256 yang disimpan), `server reindex` (bangun ulang Elasticsearch dari PostgreSQL).

### 13.3 Keputusan
| # | Keputusan | Alasan | Konsekuensi |
|---|---|---|---|
| 024 | Injector **tidak punya akses database**; semua lewat API dengan token per-integrasi | Injector menjalankan alat yang memproses data dari jaringan target; bila dibobol, ia tidak boleh membaca atau menulis data tenant | Server yang menentukan sasaran, aset, dan jenis finding yang sah; injector hanya melapor |
| 025 | Parameter aksi **tidak pernah menjadi argumen mentah**: profil/port/severity/tag divalidasi daftar putih atau pola ketat, target ditaruh setelah `--`; tanpa shell | Mencegah injeksi argumen (mis. `-oN /etc/cron.d/x`, `--script`) | Paket `contract` dipakai server dan injector (aturan sama di dua sisi); fitur baru = tambah profil, bukan teks bebas |
| 026 | **Penjaga scope berlapis**: (1) saat Run, (2) saat Claim (otorisasi bisa dicabut setelah antre), (3) di dalam injector | Satu pemeriksaan bisa kedaluwarsa atau dilewati | Selalu ditolak: link-local (169.254.0.0/16, metadata cloud), multicast, 0.0.0.0, nama yang salah satu hasil DNS-nya terlarang; loopback hanya dengan opt-in `ORCHIS_ALLOW_LOOPBACK_TARGETS` (dev/tes); rentang CIDR belum bisa dipindai |
| 027 | Finding diikat ke **aset sasaran eksekusi di sisi server**, bukan ke aset yang disebut injector; jenis finding dibatasi `output_types` kontrak | Injector yang dibobol tidak bisa menempelkan temuan palsu ke aset lain | Finding di luar kontrak ditolak dan dicatat di log eksekusi |
| 028 | Finding **dedup** per (aset, jenis, nilai ter-normalisasi); tiap eksekusi menambah satu `finding_occurrences` | Scan berulang tidak boleh menggandakan temuan | `occurrence_count` naik; duplikat dalam satu batch dihitung sekali |
| 029 | Request/response mentah Nuclei **tidak diteruskan** ke `details` | Bisa berisi kredensial atau data sensitif dari target | Hanya metadata template dan `matched-at`; keluaran mentah disimpan di MinIO bila perlu diselidiki |
| 030 | **Lease + sweeper** (ticker 15 dtk, idempoten) untuk eksekusi macet; ready yang menggantung > 5 menit diterbitkan ulang | Injector mati tidak boleh membuat eksekusi `running` selamanya; pesan bisa hilang | go-quartz **tidak dipakai di sini**: sweep bersifat idempoten sehingga tidak perlu deduplikasi antar-instance (go-quartz tetap untuk pekerjaan terjadwal seperti impor MITRE) |
| 031 | Pencarian teks: **Elasticsearch menyaring ID, PostgreSQL mengurutkan dan memberi data** | ES bisa tertinggal atau mati; data tampil harus selalu benar dan terikat tenant/RLS | ES mati -> jatuh ke `ILIKE` di PostgreSQL (dengan `%`/`_` di-escape); respons menyebut sumbernya (`search`) |

### 13.4 Batasan yang diketahui
- Hanya satu tenant per proses server (token injector dicari pada tenant server). Multi-tenant nyata butuh pencarian token lintas tenant dengan peran khusus.
- **DNS rebinding**: nama host diperiksa saat Run/Claim/injector, tetapi alat menerjemahkannya lagi saat memindai. Risiko sisa; mitigasi nyata = kebijakan egress jaringan pada container injector (jangan beri akses ke jaringan metadata/manajemen).
- Pembatalan menghentikan proses paling lambat ~10 detik (interval heartbeat); status di UI langsung `Canceled`.
- Pesan yang ditolak masuk antrean `orchis.dead` tetapi belum ada UI/pemantauannya.
- Daftar aset sasaran di dialog diambil dari 100 aset pertama (yang diotorisasi). Perlu pencarian di dialog bila aset > 100.
- Hanya satu run aktif per atomic test; belum ada penjadwalan berkala.
- Token injector disimpan sebagai hash di `integrations.config`; belum ada UI untuk membuat/memutar token (CLI saja).
- Windows/macOS: injector berjalan di Linux saja (container).

---

## 14. Home (sudah dibangun)

`GET /api/v1/home` (wajib login) mengembalikan ringkasan baca-saja dalam satu transaksi read-only. **Setiap bagian hanya terisi bila pemanggil punya izin bacanya** (`assets` ← asset.read, `findings` dan `most_exposed` ← finding.read, `atomic`, `latest_runs`, `injectors` ← atomic.read); bagian yang tidak berhak bernilai `null`/kosong dan UI menyembunyikannya.

| # | Keputusan | Alasan |
|---|---|---|
| 032 | Home hanya menampilkan angka yang **benar-benar ada datanya**. Tile Scenarios, Simulations, dan Injects dari desain **tidak ditampilkan** sampai Fase 5–6 | Deretan nol palsu menyesatkan; desain asli mengasumsikan fitur yang belum ada |
| 033 | "Exposure posture" memakai **kerentanan saja** (sesuai keputusan MVP: Vulnerability only). Tingkat paparan diturunkan dari severity tertinggi: Critical → High exposure, High → Elevated, Medium → Moderate, selainnya Low, tanpa data → "No data yet" | Ringkasan cepat, bukan skor resmi; sebaran severity tidak mencampur port terbuka (info) dengan kerentanan |
| 034 | Finding milik **aset yang sudah dihapus** tidak dihitung di Home dan tidak muncul di daftar Findings (datanya tetap di database) | Angka Home dan daftar Findings harus selalu sama |
| 035 | Data kosong → panel **"Get started"** (tambah aset → otorisasi → jalankan atomic test), bukan deretan nol | Frame 2.3 desain |

Batasan: ringkasan dihitung langsung dari tabel pada tiap permintaan (cukup untuk puluhan ribu temuan; bila lebih, perlu tabel agregat). Halaman menyegarkan diri tiap 30 detik, atau 3 detik selama ada run aktif.

---

## 15. Fase 5–6: scenario, chain, simulasi (sudah dibangun)

### 15.1 Model
- **Scenario = chain** (satu entitas, dua tampilan: halaman *Scenarios* dan *Attack chains*). Graf = langkah (`scenario_steps`) + tepi (`scenario_step_edges`) + perimeter sasaran (`scenario_targets`: aset atau grup).
- **Simulasi = salinan beku** (ADR-005): saat peluncuran langkah, tepi, dan sasaran disalin; grup dinamis diurai menjadi aset saat itu juga. Mengedit scenario sesudahnya tidak memengaruhi simulasi yang berjalan (ada tes).
- **Mesin chain** ada dalam dua lapis: `internal/chain` (aturan murni tanpa database: validasi DAG + `Evaluate`) dan `internal/simulation` (penyimpanan, peluncuran eksekusi, jeda/batal). Status langkah **dihitung**, tidak disimpan (§9.3).

### 15.2 Aturan chain
| Aturan | Perilaku |
|---|---|
| Join | Langkah baru dievaluasi setelah **semua** induknya berstatus akhir (done/failed/skipped/blocked) |
| `after_parents` | Jalan bila semua induk `done` (induk yang dinonaktifkan dianggap lolos). Bila ada induk failed/blocked/dilewati → `blocked` (tidak pernah jalan) |
| `on_finding` | Setelah semua induk berstatus akhir: jalan bila salah satu induk menemukan finding yang cocok (`type`, `values` persis, `min_severity`); kalau tidak → `skipped`. Finding pemicu dicatat di `finding_triggers` |
| Banyak sasaran | Satu eksekusi per aset sasaran per langkah. Langkah `done` bila ada eksekusi yang sukses; `failed` bila tidak ada |
| Jeda antar langkah | `delay_seconds`: eksekusi dibuat dengan `not_before`, dan pesan outbox baru bisa diterbitkan pada waktu itu (`available_at`) |
| Simulasi selesai | Tidak ada langkah yang menunggu, antre, atau berjalan, dan tidak ada yang perlu diluncurkan → `finished` (walau ada langkah failed) |
| Batas | Maks. 100 langkah, 400 tepi, tanpa siklus; manual trigger dan mapping nilai finding ke parameter **belum tersedia** (ditolak validasi) |

### 15.3 API (di bawah `/api/v1`)
| Endpoint | Izin |
|---|---|
| `GET/POST /scenarios`, `GET/PATCH/DELETE /scenarios/{id}` | scenario.read / scenario.write / scenario.delete |
| `PUT /scenarios/{id}/targets`, `PUT /scenarios/{id}/graph` | scenario.write (kunci optimistik `version`, 409 bila bentrok; graf divalidasi penuh dan param dinormalisasi per kontrak) |
| `POST /scenarios/{id}/launch` | simulation.run (202) |
| `GET /simulations`, `GET /simulations/{id}`, `GET /simulations/{id}/attack-path` | simulation.read |
| `POST /simulations/{id}/pause\|resume\|cancel` | simulation.run |
| `GET /findings?simulation_id=` | finding.read |

### 15.4 Keputusan
| # | Keputusan | Alasan | Konsekuensi |
|---|---|---|---|
| 036 | **Evaluasi chain murni, dipisah dari penyimpanan** (`chain.Evaluate`) | Aturan join/trigger adalah inti produk; harus bisa diuji tuntas dan deterministik | Penyimpanan hanya memuat graf + eksekusi + finding lalu menjalankan hasil evaluasi |
| 037 | Chain **maju dalam transaksi yang sama** dengan hasil eksekusi (hook `AfterEnd`) | Crash di antara "hasil tersimpan" dan "langkah berikut dibuat" akan membuat chain macet | Hook dipanggil dari Complete, penolakan Claim, dan sweep timeout; ticker 15 dtk hanya jaring pengaman. Advance idempoten (kunci baris simulasi + indeks idempotensi eksekusi) |
| 038 | **Preflight peluncuran**: semua sasaran harus diotorisasi, punya alamat yang lolos penjaga scope untuk tiap langkah aktif, dan injectornya aktif | Kegagalan harus tampak sebelum jalan, bukan di tengah simulasi | 422 yang menyebut nama aset/langkahnya; penjaga tetap diperiksa lagi saat Claim (ADR-026) |
| 039 | Pemicu `on_finding` hanya melihat finding dari **induk langsung** | Hasil harus final begitu semua induk selesai (deterministik); finding cabang lain yang datang belakangan tidak boleh mengubah keputusan | Untuk memicu dari cabang lain, jadikan cabang itu induk (join) |
| 040 | **Jeda** menahan Claim (409) tanpa membuang pekerjaan: saat `resume`, dispatch eksekusi ready **diterbitkan ulang** | Injector membuang pesan yang ditolak; tanpa penerbitan ulang, eksekusi tertinggal | Eksekusi yang sedang berjalan boleh selesai saat dijeda, tetapi tidak ada langkah baru yang dibuat |
| 041 | **Batal/timeout simulasi**: eksekusi ready/running → `ended(canceled)`; injector menghentikan prosesnya lewat heartbeat (≤10 dtk) | Kill switch (prinsip keamanan platform) | Langkah yang seluruh eksekusinya dibatalkan tampil `Canceled`; anak-anaknya `Blocked` |
| 042 | Attack path **diturunkan** (ADR-016): aksi → sasaran → temuan → aksi lanjutan; tepi redundan dibuang (aksi lanjutan tidak menggambar ulang sasaran; aksi yang dipicu temuan tidak digandakan tepi `then`) | Graf yang terbaca lebih berharga daripada graf lengkap | Maks. 100 temuan paling parah; `truncated` memberi tahu |

### 15.5 Batasan yang diketahui
- Peluncuran selalu **langsung** berjalan: `scheduled`/`draft` simulasi dan penjadwalan berkala belum ada (kolom sudah disiapkan).
- Tidak ada drag-and-drop pada workspace: tata letak dihitung otomatis (lapisan = jarak terpanjang dari akar), kolom `position_x/y` belum dipakai.
- Pemicu hanya **menggerbang** langkah; nilai finding belum bisa diteruskan sebagai parameter (mis. port yang ditemukan Nmap belum otomatis menjadi target Nuclei).
- Belum ada *chokepoint*, "jalur A–D", statistik MITRE, dan ekspor laporan pada detail simulasi.
- Simulasi dengan banyak aset × banyak langkah menjalankan semua eksekusi sebuah langkah sekaligus (tanpa batas konkurensi per injector selain satu pekerjaan per proses injector).
- Mengubah/menghapus scenario tidak menghentikan simulasi yang sedang berjalan darinya.

---

## 16. Fase 7: akun, pengaturan, orang, MITRE, notifikasi (sudah dibangun)

### 16.1 API baru (di bawah `/api/v1`)
| Area | Endpoint | Izin |
|---|---|---|
| Akun | `GET/POST /users`, `PATCH /users/{id}`, `POST /users/{id}/reset-link`, `GET /roles` | user.manage |
| Publik | `POST /auth/forgot`, `POST /auth/reset/check`, `POST /auth/reset` | tanpa login |
| Profil | `PATCH /me`, `POST /me/password`, `GET/POST /me/tokens`, `DELETE /me/tokens/{id}` | login |
| Integrasi | `GET /integrations`, `PATCH /integrations/{id}` (aktif/nonaktif), `POST /integrations/{id}/token` | integration.read / integration.manage |
| Pengaturan | `GET /settings` (nama, versi; komponen hanya untuk settings.manage), `PATCH /settings` | login / settings.manage |
| Tag | `PATCH/DELETE /tags/{id}` | asset.write / asset.delete |
| Orang | `GET/POST/PATCH/DELETE /people`, `/teams`, `PUT /teams/{id}/members` | people.read / people.write |
| MITRE | `GET /mitre/matrix`, `GET /mitre/techniques/{id}`, `POST /mitre/import` | scenario.read / settings.manage |
| Alert | `GET /alerts`, `POST /alerts/{id}/read\|dismiss`, `POST /alerts/read-all` | alert.read |

### 16.2 Keputusan
| # | Keputusan | Alasan | Konsekuensi |
|---|---|---|---|
| 043 | **Reset password lewat link buatan admin** (ADR-011 diwujudkan): token acak 256-bit, hanya hash SHA-256 yang disimpan, berlaku 30 menit, sekali pakai, **satu link aktif per akun** (yang baru menghapus yang lama) | Tanpa SMTP, admin yang menyerahkan link; link bocor/ganda harus sesempit mungkin | Memakai link: semua sesi akun berakhir, penguncian login dibersihkan, password tidak boleh sama dengan 5 sebelumnya (termasuk yang sekarang). Akun baru dibuat TANPA password dan baru bisa masuk lewat link |
| 044 | **Lupa password**: respons `202` yang sama untuk email ada/tidak ada; bila akun ada, admin mendapat alert (maks. satu per akun per jam) | Tidak membocorkan keberadaan akun; tidak bisa dipakai membanjiri admin | Alert hanya terlihat oleh pemegang `user.manage` |
| 045 | **Token API** (`oat_…`, maks. 10 aktif per pengguna, masa berlaku 1–365 hari): hak = **peran pemilik saat ini**; header `Authorization` mengalahkan cookie; CSRF dilewati hanya untuk bearer `oat_` | Skrip butuh akses tanpa peramban; mengubah peran/menonaktifkan akun harus langsung memengaruhi token | Rahasia tampil sekali; daftar hanya menampilkan awalan; belum ada cakupan (scope) per token |
| 046 | **Sesi yang berakhir di tengah pemakaian** (akun dinonaktifkan, sesi dicabut): UI mendengar 401 dari jalur non-`/auth/` lalu kembali ke login | Tanpa ini pengguna terjebak di layar "gagal memuat" | Jalur `/auth/` dikecualikan: 401 di sana berarti password/token salah |
| 047 | **Pagar admin**: tidak boleh mengubah peran/status diri sendiri; admin aktif terakhir tidak boleh hilang; perubahan peran **mengunci semua admin aktif berurutan menurut id** sebelum memeriksa | Dua admin yang saling menurunkan serentak tidak boleh meninggalkan platform tanpa admin; urutan tetap mencegah deadlock | Dibuktikan deterministik dengan tes yang menahan kunci admin lain. Menonaktifkan akun mencabut semua sesinya, dan sesi itu tidak hidup lagi saat akun diaktifkan kembali |
| 048 | **Integrations**: token injector dibuat/diputar dari UI (rahasia sekali), aktif/nonaktif, log terbaru (sambungan, rotasi, aktif/nonaktif) | Sebelumnya hanya lewat CLI | Heartbeat pertama setelah >90 detik senyap dicatat sebagai "connected"; injector nonaktif ditolak dan `Run` ke injector nonaktif 409 |
| 049 | **Settings**: hanya nama platform yang dapat diubah; kesehatan komponen (PostgreSQL, Elasticsearch, RabbitMQ, MinIO) diperiksa paralel dengan batas 3 detik; komponen yang tidak dikonfigurasi ditandai `not_configured` (bukan "Running"); detail galat internal tidak dikirim ke klien | Jujur tentang apa yang aktif tanpa membocorkan topologi | Parameter lain ditambahkan hanya bila ada kode yang benar-benar membacanya |
| 050 | **Impor MITRE**: unduh STIX Enterprise, urai streaming, validasi (taktik > 0, teknik ≥ 10, batas ukuran), simpan dalam **satu transaksi**; teknik yang hilang dari sumber ditandai deprecated (tidak dihapus); klaim berseri lewat kunci advisory (satu impor pada satu waktu); pemeriksaan go-quartz tiap 12 jam, impor bila data > 7 hari, satu klaim per hari lewat `job_runs`; impor manual berjalan di latar belakang (202) | Sumber eksternal tidak boleh merusak data bila gagal/terpotong; beberapa instance tidak boleh mengunduh serentak | `ORCHIS_MITRE_URL` (`off` untuk lingkungan tanpa internet); kontrak bawaan ditautkan ke T1046 (Nmap) dan T1595.002 (Nuclei) |
| 051 | **Matriks MITRE**: sebuah teknik "dilatih" hanya bila ada eksekusi **sukses** dari aksi yang tertaut; sub-teknik menandai induknya; data ATT&CK bersama, status latihan per tenant | Cakupan yang jujur (eksekusi gagal tidak dihitung) | Hanya matriks Enterprise; Mobile/ICS belum |
| 052 | **Alert** ditulis dalam transaksi yang sama dengan kejadiannya dengan `dedupe_key`: temuan kritis baru, scan gagal/timeout, simulasi selesai/gagal karena waktu, permintaan reset password; audiens = permission; status baca/singkir **per pengguna** | Kejadian tidak boleh hilang atau berganda; pengguna tidak boleh melihat alert yang bukan haknya | Belum ada retensi otomatis; alert "agent offline" menunggu Fase 4 |
| 053 | **Palet perintah (⌘/Ctrl+K)** dan pintasan "g lalu huruf" hanya memuat tujuan yang boleh dibuka pengguna; pintasan tidak aktif saat mengetik atau saat dialog terbuka | Aksesibilitas dan produktivitas tanpa menampilkan fitur yang akan ditolak server | |

### 16.3 Batasan yang diketahui
- **Impor CSV orang** dan **log audit di UI** belum ada (tabel dan API tulis audit sudah ada).
- Peran tetap tiga (Admin/Analyst/Viewer); halaman Roles hanya-baca.
- Token API berhak penuh seperti pemiliknya; belum ada pembatasan per token.
- Notifikasi memakai polling 20 detik (tanpa WebSocket); belum ada pembersihan alert lama.
- Data MITRE hanya Enterprise ATT&CK; belum ada pemetaan per inject/aksi di luar dua kontrak bawaan, dan belum ada halaman statistik per taktik.
- Satu tenant per proses server (lihat ARSITEKTUR 13.4).

---

## 17. Fase 4: agent Go (sudah dibangun)

### 17.1 Alur
```
Konsol (Assets → Agents → Install agent)   POST /agents/enrollment-tokens  → token oen_… (sekali pakai, 60 menit) + perintah pemasangan
Endpoint:  curl -H "Authorization: Bearer oen_…" /api/v1/agent/download     → biner untuk platform token itu
           sha256sum -c  (checksum dari konsol)  →  orchis-agent enroll  →  install/start (service) atau run (user)
Agent ──POST /agent/enroll {token, hostname, os, arch, version}──▶ server membuat/memakai ulang ASET + kredensial per agent (oag_…)
Agent ──POST /agent/heartbeat (15 dtk)──▶ {poll_seconds, latest_version?}      keluar-saja: tidak ada koneksi masuk ke endpoint
Agent ──POST /agent/executions/claim (polling)──▶ tugas baku | 204
Agent ──POST /agent/executions/{id}/result──▶ finding (terikat ke aset agent itu, jenis dibatasi kontrak)
```
Aksi agent (`needs_agent`): `agent.listening_ports` (soket TCP yang mendengarkan; Linux dari `/proc/net/tcp*`, macOS/Windows dari `netstat -an`) dan `agent.system_info` (OS, kernel, arsitektur, nama host). Keduanya **hanya-baca, tanpa parameter** (parameter selain `{}` ditolak), dan agent hanya mengenal kunci aksi baku itu: server tidak bisa mengirim perintah, dan kunci lain dijawab galat.

### 17.2 API baru (di bawah `/api/v1`)
| Endpoint | Izin / autentikasi |
|---|---|
| `GET /agents`, `POST /agents/enrollment-tokens`, `GET /agents/enrollment-tokens/{id}`, `POST /agents/{id}/revoke` | agent.read / agent.manage |
| `POST /agent/enroll` | token pendaftaran di badan permintaan |
| `POST /agent/heartbeat`, `POST /agent/executions/claim`, `…/{id}/heartbeat`, `…/{id}/result` | `Authorization: Bearer oag_…` |
| `GET /agent/download` | `oag_…` (memperbarui diri) atau `oen_…` yang belum terpakai; platform diambil dari kredensial, bukan parameter |

Jalur `/agent/` dikecualikan dari aturan CSRF karena tidak pernah memakai cookie.

### 17.3 Keputusan
| # | Keputusan | Alasan | Konsekuensi |
|---|---|---|---|
| 054 | **Agent hanya koneksi keluar (polling)**; tugas diambil dari baris `executions` yang ditujukan kepadanya (tanpa antrean RabbitMQ) | Endpoint di belakang NAT/firewall; tidak ada port masuk | Latensi tugas ≤ `poll_seconds` (5 dtk). Eksekusi agent tidak membuat pesan outbox |
| 055 | **Token pendaftaran** `oen_…`: hanya hash disimpan, sekali pakai (UPDATE bersyarat atomik), kedaluwarsa 60 menit, terikat platform & mode | Token bocor sesempit mungkin | Pendaftaran gagal karena platform salah **tidak** menghanguskan token. Unduhan biner dengan token di header (bukan URL, agar tidak masuk log) |
| 056 | **Kredensial per agent** `oag_…` (hash di database), bukan token admin; pencabutan permanen | Satu endpoint bocor tidak boleh membuka yang lain | Dicabut = 401 di semua endpoint, eksekusinya dibatalkan, agent menghapus rahasia dari disk dan **diam** (tidak keluar, agar service manager tidak menghidupkannya terus). Pemulihan = token baru |
| 057 | **Hanya nama host yang dipercaya dari agent**; alamat IP tidak diambil | Alamat menentukan apa yang boleh dipindai injector; tidak boleh ditentukan endpoint | Aset hasil agent dibuat `testing_authorized=false`. Memasang ulang di mesin yang sama memakai kembali asetnya (riwayat temuan tetap) |
| 058 | **Update diri bertanda tangan** (ADR-018 diwujudkan): rilis ditandatangani Ed25519 atas (versi, OS/arsitektur, SHA-256); kunci publik **tertanam di biner**; tolak downgrade; uji asap sebelum pasang; rename dua langkah; restart otomatis | Server dan jalur unduhan tidak perlu dipercaya penuh | Server hanya menyajikan rilis dan tidak memegang kunci privat. Agent tanpa kunci tertanam tidak memperbarui diri. Rilis ditandatangani kunci lain ditolak dan biner tetap utuh |
| 059 | **Alat rilis** `cmd/agentrelease`: bangun lintas platform, tanam kunci publik, tandatangani, tulis `manifest.json` **terakhir dan atomik**; server memuat ulang manifest bila berubah dan menolak manifest cacat (path traversal, hash/tanda tangan hilang) | Manifest yang menunjuk berkas yang belum ada tidak boleh pernah terlihat | `ORCHIS_AGENT_RELEASE_DIR`; tanpa rilis, pemasangan agent dinonaktifkan dengan pesan jelas |
| 060 | **Aksi agent di rantai simulasi**: peluncuran ditolak di muka bila aset tanpa agent; bila agent hilang sebelum langkah dimulai, langkah gagal dengan alasan jelas (simulasi tetap berakhir); agent yang offline membuat eksekusi menunggu sampai agent kembali atau simulasi kehabisan waktu | Kegagalan terlihat; tidak ada simulasi yang menggantung | Atomic test langsung menolak (409) bila agent offline |
| 061 | **Agent offline**: sweeper menandai agent yang senyap > 90 detik dan membuat satu alert per episode senyap; status di daftar dihitung dari detak terakhir, bukan kolom yang bisa basi | Pemberitahuan tanpa banjir | Heartbeat berikutnya memulihkan status |

### 17.4 Pembuktian lintas OS (jujur)
| Platform | Status |
|---|---|
| Linux x86_64 | **Terbukti berjalan**: e2e memasang agent dari perintah di konsol, mendaftar, menjalankan aksi, memperbarui diri, menolak rilis berkunci lain, dan dicabut. Pemasangan sebagai service systemd terbukti di spike (ARSITEKTUR 11.3) |
| Linux arm64, macOS (x86_64/arm64), Windows (x86_64/arm64) | **Hanya terbukti ter-build** (cross-compile, CGO mati) dan parser keluaran `netstat` diuji dengan contoh. Belum dijalankan di mesin sungguhan: pemasangan service (launchd, SCM), `netstat`, dan restart setelah update di Windows belum teruji |

### 17.5 Batasan yang diketahui
- Mode "user" di macOS/Linux memakai opsi `UserService` kardianos (tidak diuji); di Windows mode user = jalankan `orchis-agent run` secara manual.
- Satu agent aktif per aset dan mode (indeks unik); belum ada pengelompokan banyak agent per aset.
- Update berjalan di semua agent begitu rilis baru dipublikasikan (belum ada bertahap/cincin).
- Biner diunduh lewat HTTP(S) server yang sama: keaslian bergantung pada checksum yang tampil di konsol (pada instalasi pertama) dan tanda tangan (pada update). Gunakan HTTPS di produksi.
- Aksi agent baru dua dan hanya-baca; belum ada eksekusi perintah/skrip di endpoint (sengaja di luar MVP).

---

## 18. Fase 8 (bagian 1): responsif dan aksesibilitas (sudah dibangun)

Ruang lingkup desain: tampilan **≥ 834 px** (frame 13); di bawah itu tidak didukung dan tidak diuji.

### 18.1 Cara diuji (otomatis, `e2e/flow7.mjs`)
Data contoh yang realistis (aset, grup, tim, scenario berisi rantai tiga langkah, dua simulasi, temuan kritis, token) dibuat lewat API, lalu **semua** halaman, 10 dialog, 3 menu/popover, dan 3 halaman publik dibuka pada **1280 × 800** dan **834 × 1112**:
| Pemeriksaan | Aturan |
|---|---|
| axe-core | WCAG 2.0/2.1/2.2 A dan AA **ditambah** praktik terbaik; nol temuan di semua tingkat keparahan |
| Gulir mendatar | Halaman tidak boleh bergulir mendatar; tabel/kanvas lebar boleh, tetapi hanya di dalam wadahnya sendiri |
| Header | Judul halaman tidak boleh terbungkus lebih dari 2 baris; dock aksi tidak boleh membengkak karena teks terbungkus |
| Dock aksi | Konten paling bawah tidak boleh tertutup dock setelah digulir ke dasar |
| Dialog | Harus muat di lebar layar, tidak terpotong di atas, dan dapat digulir bila lebih tinggi dari layar |
| Papan ketik | Tab melewati ≥ 8 elemen per halaman dan **semuanya** punya indikator fokus; "Skip to content" menjadi tab pertama dan memindahkan fokus ke `<main>` |
Pemeriksaan ini sendiri diuji dengan merusak CSS (aturan fokus kolom isian dan aturan header sempit dihapus): keduanya terdeteksi.

### 18.2 Perbaikan yang dihasilkan
- Daftar notifikasi memakai markup yang tidak sah (`<li>` di dalam `<div>`): satu `<li>` per alert.
- Wilayah yang bisa digulir (tabel, kanvas chain, attack path, matriks) kini dapat difokus dengan papan ketik dan punya nama.
- Attack path bukan lagi `role="img"` (yang menyembunyikan teks simpul dari pembaca layar) melainkan grup berteks.
- Landmark: `<main>` dan tautan lompat; panel rincian memakai region (bukan `aside` bersarang); dock aksi menjadi region bernama.
- Layar 834 px: header halaman membungkus dengan benar (judul tidak lagi terjepit tombol aksi), dock aksi satu baris, kotak pencarian menyusut.
- Indikator fokus untuk kolom isian dan kotak pencarian (sebelumnya hanya perubahan warna garis tepi).

### 18.3 Batasan jujur
- Pemeriksaan otomatis menangkap sebagian masalah aksesibilitas saja. **Belum ada** uji manual dengan pembaca layar (NVDA/VoiceOver), uji zoom 200%/400%, mode kontras tinggi, atau tema terang.
- Sentuhan (touch) di tablet belum diuji; ukuran target diperiksa axe (WCAG 2.2) tetapi bukan dengan perangkat nyata.
- Hanya Chromium yang dipakai; Firefox dan Safari belum.
- Graf chain dan attack path adalah SVG/posisi absolut: pembaca layar mendapat teks simpul dan ringkasan jumlah simpul/koneksi, bukan penjelajahan graf yang interaktif.

## 19. Fase 9.1: organisasi dan assessment

Organisasi bukan tenant: seluruh relasinya tetap dibatasi `tenant_id`, RLS, dan FK komposit. Aset lama bernilai `organization_id=NULL` (Unassigned). API organisasi mendukung daftar berkursor, create, detail, update berversi, dan pemindahan/otorisasi aset massal yang atomik. Otorisasi membutuhkan `asset.authorize`; pemasangan agent tidak memberikannya.

| ADR | Keputusan | Alasan |
|---|---|---|
| 062 | Assessment organisasi mengelompokkan satu simulasi beku per aset | Engine lama menggabungkan hasil antar-aset; pemisahan ini menjaga kegagalan dan pemicu per aset tanpa mengubah scenario lama |
| 063 | Token enrollment menentukan organisasi; nama instance terpisah dari hostname | Endpoint tidak boleh memilih organisasi sendiri; beberapa agent pada satu mesin memakai direktori dan service berbeda |
| 064 | Keanggotaan, alamat, profil dan versi aksi assessment dibekukan saat peluncuran | Perubahan inventaris tidak diam-diam mengubah cakupan run; Claim tetap memeriksa otorisasi dan scope terkini |
| 065 | Aset tanpa izin/alamat layak dicatat sebagai skipped; assessment menerima Idempotency-Key | Coverage terlihat jujur dan pengulangan permintaan tidak memindai dua kali |

API baru: `/api/v1/organizations`, `/organizations/{id}`, `PUT /organizations/{id}/assets`, `GET/POST /organizations/{id}/assessments`, `GET /assessments/{id}`, dan `POST /assessments/{id}/cancel`. Hak baru `organization.read/write`, `assessment.read/run`, `report.read/generate` disinkronkan ke peran bawaan. Assessment membutuhkan pula izin baca aset dan menjalankan simulasi. Filter aset `organization_id=unassigned` atau UUID; pendaftaran memakai `organization_id` pada token dan `enroll --name` untuk instance.

Batasan: maksimal 1000 aset per assessment, 100 aset per mutasi membership, dan daftar agent lama maksimal 500. Simulasi memakai batas waktu bawaan satu jam termasuk antrean; organisasi besar memerlukan kapasitas injector yang sesuai dan belum dibuktikan lewat uji volume produksi. Pemindaian memerlukan injector yang dapat menjangkau target; agent tidak menjadi relay Nmap/Nuclei. Windows/macOS tetap baru terbukti ter-build. Jadwal assessment dan API AI ditunda. Kegagalan scan tidak berarti aman; status assessment finished hanya berarti pekerjaan sudah terminal.

## 20. Fase 9.2: katalog aksi deklaratif

ADR 066: definisi tepercaya berversi ada di `internal/contract/catalog.json`; setiap definisi memilih adapter yang sudah dikenal. Skema tertutup mendeskripsikan jenis, enum, pola, batas, default, label, bantuan dan dependensi tampilan. Server dan injector memvalidasi skema yang sama; adapter tetap memeriksa aturan lintas parameter. Menambah definisi adapter yang ada memerlukan distribusi katalog ke server/injector, bukan perubahan formulir. Tidak tersedia registrasi shell bebas. Injector baru menolak versi kontrak yang tidak didukung.

ADR 067: formulir atomic test dan editor langkah memakai satu renderer skema. `/api/v1/contracts` mengembalikan skema, kategori, versi serta status integrasi. `/api/v1/action-presets` menyediakan daftar dan penyimpanan preset tervalidasi dengan izin atomic.read/write. Preset bawaan Quick exposure check, Full TCP + services dan Web CVEs disemai tanpa menimpa perubahan pengguna. Profil template Nuclei hanya memilih direktori tetap exposure/cves/misconfig dalam image; pengguna tidak dapat memberi path.

Batasan: definisi baru memerlukan build/distribusi; adapter baru tetap membutuhkan implementasi dan peninjauan kode. Daftar preset dibatasi 200; pembaruan/penghapusan preset belum tersedia. Semua pemeriksaan scope tetap berlaku.

## 21. Fase 9.3: kondisi dan pemetaan per aset

ADR 068: edge menyimpan AST JSON terbatas (`condition`) dengan cabang match/else/failure. Operator: AND, OR, EQ, NEQ, IS_NULL, IS_NOT_NULL, GT, GTE, LT, LTE, IN, NIN, DEPEND_ON. Field hanya finding_count, max_severity (0–4), finding_types, open_ports dan outcome. AST dibatasi 12 tingkat dan 128 node. Join menunggu semua induk terminal. Else memerlukan induk sukses dan kondisi false; failure berlaku untuk induk gagal. Scenario tanpa aturan baru mempertahankan perilaku historis.

ADR 069: graf dengan kondisi/mapper dievaluasi terpisah per aset; hasil induk dari aset atau run lain tidak boleh menjadi masukan. Mapper open_ports→ports hanya untuk Nmap custom (maksimal 50 port); web_urls→target membentuk URL dari host aset beku dan port web teramati (80,443,8000,8080,8443), satu eksekusi per URL. Tidak menerima hostname/URL arbitrer dari finding. Argumen tetap melewati normalisasi kontrak dan tiga penjaga scope. Semua eksekusi sebuah langkah harus sukses sebelum cabang sukses dipilih.

ADR 070: alamat sasaran simulasi baru dibekukan; simulation_decisions merekam alasan dan pratinjau pemetaan. `POST /api/v1/simulations/{id}/dry-run` hanya membaca hasil induk dan mengevaluasi tanpa membuat eksekusi. Editor menyediakan JSON kondisi terstruktur, pilihan cabang dan mapper; detail simulasi menyediakan dry-run dan pratinjau. Nilai input yang gagal dipetakan menghasilkan kegagalan eksplisit, bukan argumen mentah.

Batasan: kondisi/mapper belum memiliki pembuat AST visual; formulir JSON divalidasi server. Profil web mapper membatasi port seperti di atas; layanan HTTP pada port lainnya memerlukan target eksplisit. Scenario lama tetap memakai agregasi historis; hanya graf yang memakai aturan baru mendapat semantik per-aset. Data run lama tanpa snapshot alamat memakai fallback alamat inventaris.

## 22. Fase 9.4: snapshot laporan organisasi

ADR 071: satu laporan versi 1 per assessment terminal. Worker membekukan snapshot dalam transaksi, terpisah dari render PDF. Snapshot menyertakan cakupan, aset dilewati beserta alasan, hasil tiap aksi/parameter/versi/waktu, temuan, ringkasan severity dan paparan, remediasi placeholder, serta perbandingan terhadap laporan sebelumnya. Finding occurrence baru menyimpan payload historis agar scan baru tidak mengganti metadata hasil run lama. Snapshot lama tidak diregenerasi oleh retry PDF.

ADR 072: halaman cetak HTML mandiri yang sama dipakai print preview, ekspor HTML, dan PDF Playwright. Tidak ada resource eksternal atau skrip target: HTML di-escape, JavaScript/service worker dimatikan, browser offline, seluruh permintaan dibatalkan, dan CSP ketat. Bukti hanya field metadata yang diizinkan, terpotong dan disunting dari pola rahasia/email/URL berkredensial. Request/response mentah dan isi berkas hasil target tidak disertakan. CSV melindungi formula spreadsheet.

ADR 073: status PDF queued/rendering/ready/failed terpisah dari assessment. Klaim memakai lease 2 menit dan SKIP LOCKED; proses render dibatasi 60 detik. Worker yang mati dapat dipulihkan (maksimal tiga klaim), retry eksplisit menggunakan snapshot yang sama. PDF disimpan di MinIO dan diunduh melalui API yang memeriksa tenant serta permission; tidak ada tautan bucket publik. Worker snapshot dicatat di audit.

API `/api/v1/reports` mendukung filter organization_id/assessment_id dan cursor; detail `/reports/{id}`, ekspor `/reports/{id}/export/pdf|html|json|csv`, print preview HTML dengan `?preview=1`, retry `/reports/{id}/retry`. Selain report.read/generate, handler memeriksa organization.read, assessment.read, asset.read, finding.read. Daftar dan detail tersedia di UI. Perbandingan mengecualikan aset yang dilewati/gagal atau mempunyai target/aksi/versi/parameter berbeda; tidak menyatakan temuan teratasi berdasarkan kegagalan scan.

Runtime renderer: `npm --prefix deploy/report ci` dan browser Chromium Playwright yang sesuai harus tersedia. `ORCHIS_REPORT_RENDER_SCRIPT` dapat menentukan lokasi skrip saat layout deployment berbeda. Di lingkungan pengembangan renderer dapat memakai instalasi Playwright e2e yang sudah ada. `node deploy/report/render.test.mjs` membuktikan blokir HTTP dengan listener lokal. MinIO wajib untuk PDF; HTML/JSON/CSV tetap tersedia bila render/storage gagal.

Batasan: belum ada jadwal laporan atau AI. MITRE disertakan dari relasi kontrak-teknik atau metadata temuan yang tersedia; tidak diinferensikan. Redaksi berbasis allowlist dan pola tidak menjamin mendeteksi semua jenis rahasia arbitrer; keluaran mentah sengaja dikecualikan. Renderer berjalan sebagai proses anak dengan egress browser diblokir; isolasi container khusus renderer belum disediakan. Input HTML renderer dibatasi 16 MiB, PDF/unduhan 32 MiB; laporan sangat besar dapat gagal merender dengan status yang dapat dicoba ulang. Uji volume produksi tetap bagian sisa Fase 8.

## 23. Fase 9.5: Home, aset dan navigasi

ADR 074: Home menambah kartu organisasi dengan cakupan aset/otorisasi, paparan dari snapshot terakhir, ringkasan perbandingan, status assessment dan tautan aksi. Ini penyesuaian desain dari `.pen` agar organisasi menjadi pintu masuk utama; tema, tipografi, dock dan komponen dasar tetap mengikuti desain aplikasi. Satu kueri agregat memuat maksimal 12 organisasi, tanpa kueri per kartu. Ringkasan organisasi hanya tersedia bila permission organization.read, assessment.read, report.read, asset.read dan finding.read semuanya dimiliki.

Dock kiri kini memiliki Organisations, Assets, Reports dan Actions. Palet perintah memakai pintasan `g o`, `g a`, `g r`, `g l` tanpa mengubah pintasan lama. Assets menyediakan tampilan Unassigned dengan filter server, termasuk pagination. Form assessment ditata ulang dalam grid responsif dan mendukung seluruh organisasi atau aset yang dipilih; urutan severity laporan selalu Critical, High, Medium, Low, Info.

Batasan: beranda menampilkan 12 organisasi terbaru, daftar lengkap tetap di Organisations. Tren berupa perbandingan snapshot yang sebanding, bukan skor risiko formal. Pengujian browser tetap Chromium; platform lain dan pembaca layar manual belum dibuktikan.


## 24. Verifikasi Fase 9 dan penutupan

Pemeriksaan rilis `E2E_SCRIPT=flow8.mjs make check` lulus (status 0): go vet, seluruh tes Go, typecheck, **107 tes frontend**, build, dan **67 pemeriksaan Fase 9**. Termasuk dua agent nyata, assessment seluruh organisasi dan pilihan satu aset, Nmap/Nuclei sungguhan, satu laporan per assessment, ekspor PDF/HTML/JSON/CSV, snapshot tetap sesudah run berikutnya, serta perbandingan cakupan yang sebanding. Semua halaman/dialog/editor baru lolos axe dan pemeriksaan overflow pada 1280/834 px; tidak ada error JavaScript. Pintasan baru g a/o/r/l diuji melalui browser.

Putaran `make check` default dengan seluruh delapan flow sebelumnya juga lulus. Putaran terakhir pada kode beku commit `b5fab31`: **lulus, status 0**; log `/tmp/orchis-phase9-complete-check.log`, status `/tmp/orchis-phase9-complete-check.status`. Perubahan sesudah commit itu hanya dokumentasi dan script reproduksi mutasi, tanpa perubahan runtime.

PDF contoh E2E (dua aset diuji, satu dilewati) dirender ke PNG dan seluruh empat halamannya ditinjau: A4, teks utuh, tidak ada orphan footer/heading, nomor halaman, tagged PDF. Screenshot Home, organisasi, laporan, dan editor kondisi juga ditinjau manual.

Perbaikan yang ditemukan melalui pengujian: pesan validasi port dipulihkan agar spesifik; nama aksesibel kartu organisasi disederhanakan; fixture mutasi otorisasi diberi alamat valid agar tidak lolos karena alasan yang berbeda; snapshot diuji ulang setelah sumber bukti berubah; metadata kosong, orphan footer dan heading cetak diperbaiki melalui tinjauan PNG. Home memakai label Vulnerabilities untuk seluruh jenis kerentanan (tidak mengklaim semuanya CVE), dan Atomic runs untuk metrik atomic.

Bukti yang sudah diperoleh: mutasi otorisasi/kondisi/snapshot terdeteksi dan dipulihkan; perintah reproduksi `python3 e2e/mutation-phase9.py` menyalin sumber ke work/, menguji baseline, memeriksa kegagalan tes yang tepat, memulihkan sumber dan menguji ulang tanpa mengubah checkout aktif; nol egress HTTP pada renderer; enam target agent ter-build; dua instance Linux amd64 berjalan nyata. Windows/macOS/arm64 belum diuji berjalan. Dua instance baru diuji dalam mode user; instalasi service bernama belum dibuktikan langsung. Penjadwalan, API AI, dan sisa pengerasan Fase 8 tetap di luar pekerjaan ini. Instalasi review lama tidak dimigrasikan atau direstart oleh sesi ini.


## 25. Audit UX menyeluruh Fase 9.5 (30 Oktober 2026)

Alur utama UI adalah organisation → aset/agent → alamat dan izin pengujian → assessment → report. User tidak perlu mempelajari model eksekusi internal untuk menjalankan assessment perusahaan. Inventaris semua route, dialog, pilihan desain, putaran audit dan bukti ada di `result.txt`. Bahasa UI tetap Inggris; komentar dan dokumentasi kerja Indonesia.

| ADR | Keputusan | Alasan dan konsekuensi |
|---|---|---|
| 075 | Nav berlabel tetap, dikelompokkan Start/Test/Results/Manage; tooltip tujuan saat hover/fokus, `aria-describedby`, Escape | Pengguna mengenali tugas tanpa menebak ikon. Tooltip ditempatkan di area sidebar sehingga tidak menutupi isi halaman; label/purpose/permission memakai satu sumber `navigation.ts` dengan palet. |
| 076 | Actions dan Single tests menjadi satu area bertab; Scenarios dan chain editor satu area; MITRE tetap di palet/reference | Menjadi sembilan tujuan utama dari sebelas ikon lama sekaligus menambahkan Settings yang mudah ditemukan. Atomic/chain API, route lama dan pintasan tetap tersedia. Nama manusiawi Single tests mengganti Atomic testing; Scenario chains adalah editor scenario yang sama. |
| 077 | Checklist Home selalu memakai satu organisasi yang dipilih dan data API nyata | Selesai langkah 3 membutuhkan sekurangnya satu aset berizin dan lolos guard alamat, langkah 4 assessment nyata, langkah 5 PDF ready. Tidak memakai jumlah definisi test sebagai bukti scan. Custom role tanpa hak baca tidak diberi data/aksi. Tampilan DB kosong mengutamakan langkah berikutnya dan mengganti deretan nol dengan state no-results. |
| 078 | `GET /api/v1/organizations/{id}/readiness` hanya membaca, memerlukan organization.read dan asset.read | Memanggil helper keputusan target yang sama dengan Launch. Alamat/otorisasi diperiksa ulang pada peluncuran/claim/injector; preview bukan izin atau janji jangkauan. RLS/tenant dijaga, maksimal 1000 detail, total tetap dihitung. Tidak membuat assessment/execution atau memberikan hak baru. |
| 079 | Konteks halaman/dialog, relasi aset-organisasi dan pesan perbaikan worker/PDF/agent | Mengajarkan fungsi saat dibutuhkan. PageAccess menahan render route terlarang dan menjelaskan hak yang diperlukan; server tetap otoritatif. Token instalasi tidak mengotorisasi scanning; worker container berbeda dari endpoint agent. Retry PDF mempertahankan snapshot dan menyediakan ekspor lain. |
| 080 | Shortcut kondisi umum dengan editor JSON advanced tetap tersedia | Pengguna memilih hasil ditemukan/tidak ditemukan atau parent sukses; branch/join/mapper diberi penjelasan. Bukan visual builder seluruh AST; guard dan evaluasi per aset tetap sama. |
| 081 | Aksi cepat, pencarian dan notifikasi di header alih-alih dock yang menutupi konten; sidebar menandai route detail/alias dan halaman baru kembali ke atas | Tinjauan 834 menemukan kartu/field tertutup dock lama. Header tidak fixed, pintasan Ctrl/⌘K tetap global; pemeriksaan layout membedakan header normal dan overlay fixed. Target link report diberi jarak, avatar memakai token ink, toolbar membungkus, label sr-only tabel dibatasi containing block scroller. |

Batasan: checklist pemilih maksimal 100 organisasi (tautan daftar berhalaman tetap ada). Readiness membaca maksimal 1000 target dan mengulang tiap 30 detik; perubahan scope/alamat sesudahnya bisa mengubah hasil Run. Status worker berbasis heartbeat dan enabled, tidak membuktikan jangkauan endpoint. Network scan tidak membutuhkan agent, local checks membutuhkannya. Fase 10/jadwal/produksi Fase 8 tetap terpisah. E2E memproksikan tugas pengguna baru; studi kegunaan dengan manusia belum dilakukan. Instalasi review lama tidak direstart atau dimigrasikan otomatis.

Verifikasi akhir pada runtime `862511f`, tes `dd989c2`: `make check` default seluruh sembilan flow lulus status 0; 119 tes frontend, flow9 283 lulus/0 gagal, Go vet/test/typecheck/build lulus. Lima putaran audit: 9 Major/7 Minor baseline, 4 Major/2 Minor tambahan putaran kedua, satu Major cache pada ketiga, satu Major placeholder temuan aset dan satu Minor notifikasi pada keempat, nol Blocker/Major/Minor produk baru pada kelima. Replay 5a gagal pada pengukuran teks intrinsik meski screenshot membaik; asersi mengukur ruang yang tersedia lalu seluruh suite diulang pada 5b. Bukti log `/tmp/orchis-ux-round5b.log`, status `/tmp/orchis-ux-round5b.status = 0`. flow7 mencakup 116 pemeriksaan (29 halaman, 23 dialog, 3 menu, 3 publik × dua viewport), 0 temuan axe/responsif, rahasia dimask. Tiga mutasi Fase 9 dan empat mutasi UX terdeteksi/dipulihkan; renderer nol HTTP. Audit per halaman, bukti historis gagal, screenshot dan keterbatasan ada di `result.txt`. Tidak ada kode OpenAEV yang disalin atau dipakai sebagai referensi implementasi dalam audit UX ini.


Tambahan ADR 077/078: perubahan izin, alamat dan membership harus menginvalidasi readiness, daftar aset organisasi dan Home segera setelah mutasi berhasil, bukan mengandalkan polling 30 detik. flow9 memeriksa hasil panel dalam 5 detik; reproduksi sebelum fix gagal dan sesudahnya lulus. Tidak mengubah izin backend atau keputusan scope, hanya kesegaran pembacaan UI.

Tambahan ADR 079/081: panel Findings detail aset memakai API existing `GET /api/v1/findings?asset_id=...&limit=5`, hanya dengan finding.read. Loading/error/retry/empty dipisahkan; placeholder statis yang mengklaim kosong dihapus. Link menuju daftar mempertahankan asset_id pada filter dan cursor. Tombol Dismiss notifikasi berukuran isi dengan flex text; flow7 menguji ruang baca di kedua viewport. Runtime fix `1feb371`, `862511f`. Verifikasi akhir putaran 5b lulus; rincian di result.txt.

## 26. Fase 10: perencanaan dan suplemen AI yang ditinjau manusia

Pengguna menyetujui kelanjutan setelah audit UX dan memilih DeepSeek untuk rencana uji, OpenAI untuk remediasi, serta **API palsu lokal untuk seluruh tes**. Persetujuan implementasi bukan persetujuan mengirim data ke provider nyata. Fitur masuk melalui organisasi dan laporan, tanpa menambah tujuan nav global. Spesifikasi, endpoint, bukti dan batas lengkap: `docs/FASE10-HASIL.md`.

| ADR | Keputusan | Alasan dan konsekuensi |
|---|---|---|
| 082 | Model sebagai pengusul check/advice dari laporan beku; AI mati per organisasi secara default | DeepSeek melengkapi engine Nmap/Nuclei lewat rencana allowlist; OpenAI mengisi suplemen remediasi. Tidak ada executor shell, tool LLM atau pengubahan severity/temuan oleh model. |
| 083 | Proyeksi payload terstruktur dengan alias | Hanya tipe observasi, severity, CVE yang cocok pola, alias dan katalog tepercaya. Nama, alamat, judul bebas, nilai dan raw evidence tidak dikirim. Mapping ID/label lokal tidak masuk body provider. Semua observasi harus dipetakan tepat sekali atau hasil ditolak. Rujukan lokal pada ekspor menghubungkan alias kembali ke laporan/aset/temuan sumber. |
| 084 | Consent/tujuan/model/limits dikendalikan `ai.manage`, versioned per organisasi | GET tidak mengaktifkan apa pun. Setiap perubahan membatalkan queued/running/review; late result ditolak. Data yang sudah keluar tidak dapat ditarik kembali. `ai.read/generate/approve/manage` tetap memeriksa permission laporan/aset/finding terkait. |
| 085 | `ai_policies.secrets_enc`: AES-256-GCM, nonce acak, AAD tenant/organisasi | Master key 32 byte dari environment terpisah DB, provider key write-only. API tidak mengembalikan rahasia. Deployment tanpa master/worker key tidak dapat mengaktifkan atau membuat request baru; emergency disable tetap tersedia. Rekey massal belum otomatis. |
| 086 | Antrean PostgreSQL, reservasi sebelum enqueue, satu klaim per organisasi, tanpa retry provider otomatis | Policy row menserialkan budget 24 jam dan klaim; `SKIP LOCKED` disusul pemeriksaan ulang snapshot setelah lock. Cooldown 10 detik, lease 90 detik, request 60 detik. Restart menandai lease hilang gagal karena biaya lama mungkin belum diketahui. Reservasi tidak direfund. Key sama/body sama idempoten, body beda 409. |
| 087 | DeepSeek Chat Completions JSON; OpenAI Responses `store:false` dengan structured output strict; validasi server tetap otoritatif | Endpoint provider tetap, tanpa tools/redirect, ukuran body/response dibatasi. Body yang akan dikirim dibentuk sebelum enqueue untuk perhitungan byte/token/cost. Model ID dan tarif aktual harus dipilih admin, bukan diasumsikan dari harga lama. Parameter kondisional provider strict boleh berupa null dan dikanonisasi menjadi field yang dihilangkan sebelum NormalizeVersion; field/flag lain tetap ditolak. Server dan image worker harus dirilis bersama. |
| 088 | Approval manusia membuat saved tests, executions dan outbox secara atomik dengan `execution.Service.QueueTest` | Helper memindahkan isi Run lama tanpa melemahkan guard. Normalisasi katalog berversi, membership organisasi dan izin/alamat/scope diperiksa ulang; Claim/injector tetap menjaga scope. Approval berulang tidak menambah scan. Cancel remaining checks mengakhiri ready/running; hasil akhir tetap ada. |
| 089 | Template append-only, instruksi/input/output direkam; suplemen AI tidak menimpa snapshot report | Saran tidak mengklaim bukti terkonfirmasi. HTML/PDF memakai template cetak yang sama dengan escaping, footer advisory dan renderer offline. JSON menyimpan provenance, CSV menangkal formula. PDF AI dirender on-demand; PDF assessment tetap di MinIO. |
| 090 | Worker/proxy Go statis dalam image scratch, non-root, read-only, tanpa capabilities atau shell | Worker memakai jaringan internal dan proxy TLS allowlist dua provider; target dihubungi injector lama. Uji loopback container memakai host networking khusus tes, disertai uji egress Docker internal terpisah. Compose memerlukan aplikasi server pada network internal yang sama; deployment review/produksi belum dilakukan. |
| 091 | Verifikasi memakai provider palsu, PostgreSQL dan scanner nyata, mutasi, screenshot dua viewport | Membuktikan kontrol aplikasi tanpa biaya/pengiriman data nyata. Tidak membuktikan kualitas model, model ID akun, billing, retensi provider, jaringan produksi atau scale. Hasil akhir dan log ada di dokumen fase; keterbatasan tidak ditutupi oleh tes hijau. |

Struktur: `internal/ai`, `cmd/{ai-worker,ai-proxy}`, `deploy/ai`, `migrations/00012_ai.sql` dan `00013_ai_indexes.sql`, `AIPage.tsx`, `flow10.mjs`, `ai-egress.mjs`, `mutation-ai.py`. Advice-only tanpa proposed checks berstatus completed, tanpa approval/dispatch aktif. Semua tabel tenant memakai RLS dan FK komposit dengan indeks pendukung. Model hanya dapat menghasilkan Nmap/Nuclei versi yang didukung dan parameter yang lolos skema yang sama dengan injector.

Batas konservatif: 500 observasi, 64.000 byte body, 128-4096 output tokens, 20 proposed checks. Payload/token lebih kecil dapat menolak laporan; tidak ada pemetaan parsial diam-diam. Budget memakai tarif batas atas administratif, bukan billing provider terverifikasi. Masa depan: batching, model nyata, key rotation, deployment network dan pengerasan produksi perlu pekerjaan tersendiri. Tidak ada AI live, scheduler atau remediasi endpoint otomatis pada milestone ini.

Verifikasi akhir Fase 10: **LULUS**, status 0 pada runtime `f08acc4`: Go vet/test, typecheck/build, 124 tes frontend (16 berkas), seluruh sepuluh flow E2E, egress AI 6/0, preflight cleanup SIGTERM, 12 mutasi terdeteksi/dipulihkan (3 Fase 9 + 4 UX + 5 AI), dan renderer tanpa akses HTTP. Bukti `/tmp/orchis-phase10-complete.log` dan `.status`. Perubahan tata cetak berikutnya diuji ulang dengan flow10 68/0, egress 6/0, tes Go AI/server dan renderer; PDF contoh satu halaman ditinjau. Opsi harness `E2E_AMQP_URL` memungkinkan vhost disposable terpisah dari instalasi review; flow3 menguji jalur tersebut (38/0). Commit tata cetak `b2f1455`, opsi harness `ae31b7a`; pengujian terakhir terarah, tanpa mengulang seluruh suite sesudah perubahan kecil ini. Rincian cakupan commit dan percobaan gagal ada di `FASE10-HASIL.md`. Berikutnya migrasi **00014**, bagian **27+**, ADR **092+**, E2E `flow11.mjs`.

## 27. Gerbang inti T0: kegagalan jangkauan dan perbandingan (31 Oktober 2026)

ADR 092: exit code scanner nol tidak cukup untuk menyatakan cakupan selesai. Nuclei dapat keluar 0 tanpa hasil pada endpoint mati. Injector memeriksa respons HTTP sebelum dan sesudah scan; kegagalan menjadi error execution, aset incomplete, dan tidak memenuhi syarat perbandingan resolved. Probe HEAD hanya memakai sasaran beku yang disetujui; semua status HTTP membuktikan respons, tanpa membaca/menyimpan body, mengikuti redirect, atau memakai proxy lingkungan. DNS untuk koneksi probe diperiksa seluruhnya dengan scope guard dan dial memakai IP yang sudah diperiksa. Permintaan HTTP probe memakai deadline 10 detik dan context tugas/lease cancellation. Pemeriksaan DNS pada scope guard tetap memakai resolver kontrak yang sudah ada; pembuktian T0 memakai loopback, bukan DNS/jaringan produksi. Sertifikat self-signed boleh diamati karena ini pemeriksaan keamanan, bukan autentikasi pengguna.

ADR 093: Nmap mempertahankan keadaan port tertutup/filtered pada XML internal (tanpa `--open`). Parser tetap menghasilkan finding hanya untuk port open dan service yang terdeteksi. Root XML salah, host hilang/down/timeout, atau seluruh port tanpa respons menjadi error/inconclusive; closed merupakan respons yang sah, tidak menjadi finding. Ini menghindari menyamakan semua port tertutup dengan endpoint yang tak dapat dihubungi. Semua guard Run/Claim/injector dan validasi parameter tetap berlaku.

`flow11.mjs` menambahkan gerbang UI dari DB baru tanpa seed: dua instance Linux, alamat/otorisasi melalui UI, empat aset (dua dilewati), scan Nmap/Nuclei nyata pada fixture loopback, empat ekspor, scan pembanding setelah fixture diperbaiki, kegagalan satu endpoint tanpa menghentikan aset lain, worker unavailable, agent revoked, serta retry PDF yang menjaga snapshot dan jumlah executions. Hasil fixture diberi nama LOCAL FIXTURE; tidak ada finding sintetis yang dihitung sebagai scan nyata. Pengujian memakai dua variabel antrean pada vhost khusus: `ORCHIS_TEST_AMQP_URL` untuk pipeline Go dan `E2E_AMQP_URL` untuk harness browser. Flow10 tetap pengujian AI; nomor tersebut tidak ditimpa untuk seeder.

Batas pembuktian: probe HTTP sebelum/sesudah tidak membuktikan setiap template individual menjangkau setiap jalur, dan layanan bisa berubah di antara pengamatan. Filter seluruh port menghasilkan kesimpulan konservatif incomplete, bukan klaim kerentanan atau aman. Scan TCP merespons pada port tertutup tidak membuktikan layanan web tersedia. Bukti T0 adalah otomasi tugas pengguna dan tinjauan gambar; studi pengguna manusia, produksi/HTTPS, seed/purge lengkap, MITRE penuh dan osquery tetap tahap berikutnya, sesudah review manual T0. Tidak ada perubahan pada OpenAEV yang berjalan atau panggilan AI eksternal.

ADR 094: laporan membedakan nol kerentanan pada sekurangnya satu aset dengan seluruh metode sukses (`No observed vulnerabilities`) dari tidak adanya cakupan lengkap (`No completed scan coverage`). Severity kerentanan tertinggi tetap menentukan exposure bila ada. Jumlah total/fully tested/skipped/incomplete dihitung dari snapshot yang sama di UI dan HTML/PDF, tanpa menulis ulang snapshot lama. Metode kosong atau satu metode gagal tidak dihitung fully tested meskipun status aset keliru finished. Alasan kegagalan ditampilkan sebelum disclosure metodologi. Tabel cetak memberi lebar eksplisit dan mempertahankan kata status utuh.

ADR 095: `e2e/renderer-fixture.mjs` hanya dipilih oleh harness flow11 pada server disposable. Flag file sementara memaksa proses renderer keluar nonzero; retry kedua memakai renderer produk asli. Aplikasi produksi tidak mengenal flag fixture. Tes membuktikan status gagal dari proses nyata, snapshot tidak berubah dan executions tidak bertambah. Pause/unpause injector hanya pada container `orchis-e2e-nmap-*` milik run; umur heartbeat dimajukan di database disposable agar tes offline tidak menunggu satu menit. Pemulihan menunggu heartbeat nyata baru. Tes checklist menunggu kelima langkah lengkap dalam 5 detik; tidak menyamakan satu item selesai dengan lima respons sudah siap.

Bukti penutup T0 dicatat terpisah di `result-infra.txt` dan `PROGRESS.md` (catatan kerja, tidak untuk cabang produk bersih). Gerbang manual bagian 0C.6 `PROMPT-INFRA-DUMMY.md` tetap berlaku: T1 dan push belum boleh dimulai sebelum pengguna mereview T0. Berikutnya bagian 28+, ADR 096+, migrasi tetap 00014, E2E flow12.

Verifikasi T0: kode/tes beku `f7cee76`, full `/tmp/orchis-infra-t0-final.log` dan `.status=0` pada 31 Oktober 2026. Go vet/test, typecheck/build, 126 tes web/17 berkas, seluruh 11 flow E2E (flow9 283/0, flow11 98/0), cleanup SIGTERM dan egress AI 6/0 lulus. Total 16 mutasi (3 Fase 9, 4 UX, 5 AI, 4 core) terdeteksi/restored; renderer tanpa HTTP. Audit tiga putaran mencatat perbaikan dan kegagalan harness; putaran 3b serta final tanpa Blocker/Major baru. Seluruh 26 screenshot core-round3 dan 12 halaman PDF dari tiga report ditinjau sebagai gambar, dengan bentuk data yang diulang pada full core-final. Snapshot tetap setelah assessment berikutnya dan retry PDF. Belum ada persetujuan manual untuk T1; tidak ada push atau deployment review/produksi.


## 28. Fondasi deployment T1 (31 Oktober 2026)

T0 disetujui pengguna melalui jawaban continue. ADR 096: compose produksi terpisah
mempunyai state/secret/volume/project sendiri; tidak mengubah compose dev, review,
atau OpenAEV. Build server multi-stage mencakup web dan Go; runtime statis non-root,
rootfs read-only. PostgreSQL owner hanya untuk job migrasi, app tidak BYPASSRLS,
maintenance terpisah. Migrasi dan provisioning token eksplisit, dependency selesai
sebelum API. JSON secret per layanan dibaca melalui ORCHIS_ENV_FILE (berkas privat,
batas ukuran, hanya namespace aplikasi), sehingga nilai tidak masuk argumen image.
Kunci enkripsi AI dibaca dari berkas terpisah, tidak diberikan kepada scanner.

ADR 097: server produksi mengirim HTML laporan ke renderer HTTP internal dengan
token acak dan tanpa redirect/proxy lingkungan. Kanal dibatasi ukuran/waktu; satu
child renderer bersamaan. Browser tetap offline/JS mati/service worker diblokir,
dan network Docker renderer internal. Dev mempertahankan proses Node lokal.
Retry PDF tetap menggunakan snapshot dan lease yang sama, tanpa scan ulang.

ADR 098: proxy memakai network khusus dan IP tetap yang dipercaya /32 oleh server;
CIDR dipilih setup tanpa benturan network Docker yang sudah ada. XFF hanya dibaca
dari peer tepercaya, dari kanan sampai hop tidak dipercaya. Rantai salah kembali
ke alamat peer. Header client tidak menentukan otorisasi scope. Default TLS CA
lokal, loopback HTTPS dan cookie Secure; ACME publik membutuhkan domain/DNS nyata.
Panduan operator dan batas pembuktian: OPERASI.md. T1 masih berjalan; tidak ada push.


## 29. Akses dan CI lokal T1

ADR 099: limiter endpoint publik aktif pada mode produksi sebelum decoding body,
10 percobaan login/reset per IP per 60 detik, enrollment 20. Registry RAM dibatasi
4096 bucket; IP diambil setelah proxy tepercaya, bukan dari header client bebas.
Bucket reset saat proses restart; deployment ini satu API, bukan limiter cluster.
Lock akun tetap di PostgreSQL dan bertahan setelah restart. Advisory transaction
lock menjaga hitungan percobaan pertama yang serentak, termasuk saat baris throttle
belum ada. Akun tidak dikenal memakai bucket IP; email tebakan tidak dipersistenkan.
Migrasi 00014 membersihkan identitas tebakan lama dengan konteks tenant satu per
satu, tanpa menonaktifkan RLS. Logging parameter SQL produksi dinonaktifkan.

Login berhasil/gagal/terkunci dan logout dicatat atomik bersama mutasi sesi/throttle.
Jika audit gagal, sesi baru tidak di-commit. GET /api/v1/audit membutuhkan audit.read,
keyset cursor at/id dan tenant eksplisit. API hanya mengeluarkan metadata serta
alasan auth allowlist, bukan detail payload audit mentah. Settings → Audit log
menyediakan filter event dan paginasi; nilai dari pengguna di-escape oleh React.
Tes Go mencakup privacy/tenant/permission/pagination/concurrency/rollback/limiter
sebelum kerja kredensial. Empat tes UI dan 38 pemeriksaan browser lulus; axe nol
pada1280/834, tanpa overflow, empat screenshot ditinjau (fixture lokal terpisah).


ADR 100: CI lokal tunggal memakai vhost RabbitMQ disposable untuk Go dan browser,
kemudian menghapus hanya vhost milik run. make ci menjalankan govulncheck v1.9.0
yang dipatok, audit seluruh lockfile npm, make check (vet/test/typecheck/vitest/
build/semua E2E), guard egress renderer dan batas chunk JavaScript500000 byte.
Halaman di-load per route; Notifications tetap eager karena dipakai Shell. Chunk
terbesar turun dari610kB ke303kB, tanpa menaikkan ambang warning Vite. Tidak ada
workflow cloud. Go minimum proyek dan image build dipatok1.26.10: pemindaian awal
1.26.8 menemukan13 advisory stdlib yang terjangkau, lalu seluruhnya hilang pada
patch resmi. Tidak mengubah Go global OS. Hasil0 reachable/imported; tiga advisory
pada modul yang tidak terjangkau tetap dicatat, bukan diklaim tidak ada.
Caddy mengikuti rilis resmi2.11.7 dengan digest dipatok. Sumber patch:
https://pkg.go.dev/vuln/GO-2026-6809 dan https://go.dev/dl/.

Koreksi ADR096/100: readiness awal hanya membuktikan proses worker. Pemeriksaan
heartbeat kemudian menemukan generator token tanpa prefix oij_ sehingga401.
Format token dipusatkan pada execution.ValidInjectorToken untuk job provision;
generator memakai prefix resmi. Ops readiness menunggu heartbeat terautentikasi
untuk kedua injector enabled, dan menghormati pilihan disabled admin. Tes format
serta startup nyata membuktikan perbaikan. Tidak melonggarkan scope/izin aset.

Koreksi ADR100: boundary Suspense ditempatkan di Outlet Shell dan Settings, sehingga
kerangka navigasi tidak hilang ketika modul halaman dimuat. SettingsPage tetap
eager sebagai layout. CI awal498dd9c menemukan tab-permission dan fokus skip-link
yang hilang saat boundary sebelumnya menutupi seluruh aplikasi. Setelah perbaikan,
Flow7lulus116 pemeriksaan/axe0 dan dua asersi fokus; Flow5lulus50/0. Harness
revokasi menerima redirect yang sudah terjadi dari polling, tetap memverifikasi
login ditolak. Pemeriksaan fokus menunggu landmark navigasi terautentikasi, lalu
menekan Tab tanpa klik; asersi posisi/fokus skip-link dan Enter tetap sama.
Chunk terbesar304417byte, di bawah batas500000. Full CI akan diulang.

Provisioning juga harus mempertahankan hasil rotasi: hanya hash kosong atau hash
yang sama dapat diterapkan ulang. Bila admin sudah merotasi token melalui API,
berkas worker lama menyebabkan conflict eksplisit dan tidak mengaktifkan kembali
token lama. Tes database membuktikan old token tetap401 dan current token tetap
valid setelah provisioning ulang. Operator harus menyelaraskan secret worker
dengan token baru sebelum restart.


## 30. Backup konsisten dan pembuktian restore

ADR 101: backup memakai pustaka age1.3.2 berlisensi BSD dengan kunci acak256-bit
terpisah dari arsip. ZIP streaming menggabungkan pg_dump, objek MinIO, state TLS,
secret dan rilis agent; helper non-network/read-only berbatas sumber daya. Tidak
membuat enkripsi sendiri. KDF penerimaan dibatasi agar header palsu tidak memicu
alokasi tak terbatas. Dump memakai role maintenance, bukan role aplikasi RLS.
Writer dihentikan dan pekerjaan nonterminal/paused ditolak karena antrean broker
tidak dipindahkan. Ini keputusan backup saat idle yang eksplisit, bukan janji
pemulihan pesan aktif. Layanan sebelumnya berjalan dipulihkan pada jalur error.

ADR 102: restore wajib ke project/state/volume kosong. Arsip diautentikasi penuh,
anggota ZIP/tar dibatasi, link/traversal/special file ditolak sebelum init target.
Snapshot JSON dan PDF diverifikasi hash-nya; database, secret dan objek dipulihkan,
Elasticsearch dibangun ulang. State baru mempertahankan alamat proxy terpilih dan
kunci backup baru. Operator menyimpan kunci sumber di luar arsip. Retensi default
lokal7 hari dapat diubah dan tidak menggantikan kebijakan organisasi/off-host.

Flow12 memisahkan project sumber/restore dan hanya menghapus resource berlabel
run. Target scanner adalah fixture HTTP internal tanpa port publik; kedua scanner
memakai jaringan backend-only. Nuclei tetap memakai manifest terkurasi image dan
validasi scope/otorisasi tetap aktif. Bukti manual restore membandingkan satu
assessment/laporan, tiga aset, dua temuan dan hash PDF byte-identik; tidak membuat
scan pengganti. Keluaran fixture bukan hasil keamanan pelanggan. Lisensi age dan
hpke disertakan pada docs/licenses serta image runtime.


## 31. Observabilitas internal

ADR 103: telemetry menggunakan format teks Prometheus dengan map nama/label tetap,
bearer token terpisah dan listener9101 internal. Tidak menambah label dari ID,
hostname, target, error atau query. Middleware mencatat kelas status serta sum/count
latensi. Injector memperbarui heartbeat setelah server menerima autentikasi, dan
menghitung scan selesai/gagal; worker AI memperbarui setelah claim200/204. Tidak
menganggap proses hidup sebagai bukti worker masih diterima server.

Kolektor server memakai transaksi read-only tenant/RLS, interval15 detik serta
batas waktu. Job stale memakai lease kedaluwarsa atau ready/queued lebih dari5menit;
DB-size adalah ukuran instalasi. RabbitMQ dibaca pasif tanpa mengonsumsi pesan;
ready dan consumer dipisahkan, bukan mengklaim ukuran unacked. Kegagalan kolektor
ditandai sehingga gauge terakhir tidak disalahartikan sebagai data baru. Disk
filesystem state diukur dari mount direktori status; status backup aman ditulis
atomik tanpa kunci. Restore lama menginisialisasi konfigurasi metrik yang belum ada
sambil mempertahankan credential bisnis.

ADR 104: monitoring opsional diprovisi melalui compose.ops.yml/profileops. Prometheus
hanya internal; Grafana memakai port loopback dan jaringan UI terpisah. Dashboard,
datasource dan delapan rule disimpan di repo; tidak ada panggilan layanan telemetry
atau instalasi plugin otomatis. Secret UID65534/472 sesuai image yang diperiksa.
Histori metrik bukan bagian snapshot bisnis. Alertmanager/notifikasi tetap Nanti.
Rule unit memakai promtool; probe nyata memeriksa empat target/401 anonim/heartbeat,
dashboard dan datasource. Rilis image dipatok digest setelah pull resmi.


ADR 105: jalur kunci/keluaran agent-release dapat ditentukan operator di luar repo.
Inisialisasi kunci memakai O_EXCL dan fsync; kunci yang sudah ada wajib regular,
privat, berukuran terbatas dan valid. Galat akses tidak berubah menjadi penggantian
kunci. Tes mengunci perilaku ini. Proof opt-in membaca manifest aktual, memverifikasi
hash/tanda tangan keenam platform, menolak digest termutasi dan menjalankan version
Linux amd64. Platform lain hanya build; akar kepercayaan tidak dirotasi diam-diam.


## 32. Pagination dan bukti volume

ADR 106: indeks findings menyertakan tenant,last_seen_at DESC,id DESC, dan varian
severity. ID tie-breaker mengikuti cursor deterministik. Pengukuran200000 baris
menemukan sorting luas pada indeks lama. Indeks baru membuat daftar106ms→1.58ms,
tetapi percobaan kedua mengungkap regresi fallback substring165ms→596ms. Fallback
kini mematerialisasi hanya ID/timestamp hasil filter sebelum page, lalu mengambil
baris/evidence halaman. Hasil akhir150ms; tidak menonaktifkan planner global,
menambah extension privileged, atau membatasi hasil secara diam-diam. ES tetap
opsional dan hasilnya tetap divalidasi ulang melalui tenant/RLS PostgreSQL.

Studi opt-in memakai50 organisasi/5000 aset/200000 temuan/50 snapshot sintetis,
context dan disk budget, dengan marker resource unik dan cleanup finally. Angka
layanan lengkap serta EXPLAIN representatif dipisahkan; bukan benchmark concurrent
atau klaim SLA. Detail rentang dan keterbatasan ada di OPERASI. Tidak mengisi cakupan
trained dari fixture volume.


## 33. Rotasi master AI yang dapat dilanjutkan

ADR 107: re-enkripsi provider credentials adalah operasi CLI operator, bukan API.
Writer dihentikan saat idle; Service.Rekey memakai transaksi tenant, lock advisory,
row lock dan audit metadata fingerprint/count. Kunci/credential tidak menjadi
argumen bernilai rahasia atau log. Kesalahan satu ciphertext membatalkan seluruh
perubahan; tenant lain tidak berubah. Version policy naik tanpa menambah consent.

Filesystem dan DB tidak satu transaksi. Jurnal privat berisi fingerprint kunci baru
dan daftar layanan lama memblokir startup/backup selama jendela perubahan. Commit
DB mendahului promosi key atomik; retry menerima ciphertext di bawah kunci baru
untuk melanjutkan setelah commit. Kunci sebelumnya disimpan privat. Precheck aktif
menolak sebelum stop; race pekerjaan sebelum stop dipulihkan tanpa menyentuh DB.
Galat setelahnya ditahan untuk roll-forward. Tes meniru kegagalan promosi setelah
DB commit, penolakan kunci baru berbeda, rollback ciphertext rusak, queued job dan
isolasi tenant. Flow12 memakai hanya credential palsu/disabled lalu menguji restore
master/ciphertext tanpa memanggil provider.

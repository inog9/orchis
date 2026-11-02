# orchis-go

Orchis adalah platform validasi keamanan self-hosted dengan backend Go dan frontend React.
Alurnya sederhana:

**Organisasi → Agent/Endpoint atau Asset → Scan (Nmap/Nuclei) → Findings → Reports (PDF/JSON)**

Keputusan desain dan batasan dicatat di [docs/ARSITEKTUR.md](docs/ARSITEKTUR.md).
Panduan operasi, upgrade, dan backup ada di [docs/OPERASI.md](docs/OPERASI.md).

## Status

- Alur utama (organisasi, agent, asset, scan, findings, reports) berjalan dan diuji end-to-end secara lokal.
- Scan Nmap dan Nuclei dijalankan oleh injector di server Orchis, bukan oleh agent.
- Koleksi data endpoint (osquery) hanya terbukti di Linux amd64. macOS, Windows, dan arsitektur lain tampil sebagai *unsupported*.
- Fitur lanjutan (Chains, Single tests/Atomic, AI, MITRE) tersembunyi secara bawaan.
- Integrasi AI hanya diuji dengan API palsu lokal; provider nyata belum diuji.
- HTTPS publik dan backup off-host belum dibuktikan.

## Alur utama

Menu utama: **Organisations, Simulations, Findings, Assets, Reports, Settings**.
Detail organisasi memiliki tab Overview, Agents/Endpoints, Assets, Scans, Findings, dan Reports.

1. **Organisasi.** Buat organisasi, lalu pasang agent atau tambahkan asset satu per satu.
   Asset lama tanpa organisasi tetap tersedia sebagai *Unassigned*.
2. **Otorisasi.** Pendaftaran agent tidak memberi izin scan. Admin menetapkan alamat target
   dan mengotorisasi pengujian per asset atau per rentang jaringan.
3. **Scan.** Tombol Scan (`/simulations/new`) memilih organisasi, beberapa asset dan/atau
   rentang dari satu agent, lalu Nmap/Nuclei dengan preset Quick atau Standard. Target berjalan
   paralel dengan batas konkurensi; kegagalan satu target tidak menghentikan target lain.
4. **Findings dan Assets.** Keduanya dapat difilter per organisasi (`?org=<uuid>` di URL;
   filter dilakukan di server). Tab organisasi memakai filter yang terkunci.
5. **Reports.** Snapshot beku per organisasi, diekspor sebagai PDF atau JSON.

### Scan

- Injector berjalan di server Orchis, jadi rentang jaringan agent harus dapat dijangkau dari
  server. Rentang yang tidak terjangkau tercatat gagal; hasil gagal tidak berarti host aman.
- Approval rentang memerlukan admin, consent endpoint yang berlaku, dan data interface/routes
  dari agent yang berhasil dalam 24 jam terakhir.
- Host yang ditemukan bisa ditambahkan sebagai asset, tetapi tidak otomatis boleh diuji.
- Batas bawaan: 256 alamat per scan dan 4 job aktif per tenant.

| Variabel | Rentang | Bawaan |
|---|---|---|
| `ORCHIS_SCAN_MAX_HOSTS` | 1–4096 | 256 |
| `ORCHIS_SCAN_CONCURRENCY` | 1–32 | 4 |
| `ORCHIS_INJECTOR_CONCURRENCY` | 1–32 | 4 per injector |

### Data endpoint (osquery)

osquery bagian dari agent, bukan pilihan tool scan. Buka **Organisations → Agents/Endpoints →
pilih endpoint → Configure endpoint collection**. Semua query yang didukung OS endpoint dipilih
sebagai usulan awal, tetapi checkbox persetujuan tetap kosong sampai admin menyetujuinya; semua
query, termasuk sensitivitas rendah, memerlukan consent.

Setelah consent dan otorisasi asset, koleksi berjalan berkala (bawaan 15 menit;
`ORCHIS_ENDPOINT_COLLECTION_SECONDS`, 60–86400). Isi persetujuan ada di
[docs/ENDPOINT-CONSENT.md](docs/ENDPOINT-CONSENT.md); versi, hash, pemberi persetujuan, dan
waktunya disimpan. Admin dapat menghapus dataset terakhir dari detail asset.

### Reports

PDF dan JSON berasal dari snapshot yang sama. JSON memakai skema `orchis.report.v1`
([docs/REPORT-SCHEMA.md](docs/REPORT-SCHEMA.md)). PDF dirender tanpa akses jaringan keluar;
bila gagal, **Retry PDF generation** mencoba ulang tanpa menjalankan scan lagi. Ekspor HTML/CSV
sudah dihapus.

### Fitur lanjutan

`/scenarios` dan halaman Actions dihapus dari UI; data dan API lama tetap ada. Chains,
Single tests/Atomic, AI, dan MITRE disembunyikan. Set `ORCHIS_ADVANCED_FEATURES=true` lalu
restart server untuk membukanya lewat palet perintah (izin API dan consent tetap berlaku).
Flag ini tidak mengembalikan Scenarios atau Actions.

## Menjalankan (pengembangan)

```bash
make dev-up                     # PostgreSQL di 127.0.0.1:15432
make build                      # frontend + server
./bin/server migrate
ORCHIS_WEB_DIR=$PWD/web/dist ORCHIS_ADMIN_EMAIL=admin@orchis.local ./bin/server serve
```

Bila `ORCHIS_ADMIN_PASSWORD` kosong, password admin pertama dibuat acak dan dicetak sekali di log.
Server tidak menjalankan migrasi sendiri saat start; jalankan `./bin/server migrate` setiap upgrade.
Akun baru dibuat admin di **Settings → Users** dan mendapat link sekali pakai (30 menit) untuk
memilih password.

### Scanner (Nmap/Nuclei)

```bash
make dev-up-full                 # + RabbitMQ, Elasticsearch, MinIO
make injector-image              # image nmap + nuclei
export ORCHIS_AMQP_URL=amqp://orchis:orchis@127.0.0.1:15672/ ORCHIS_ES_URL=http://127.0.0.1:19200 \
  ORCHIS_S3_ENDPOINT=127.0.0.1:19000 ORCHIS_S3_ACCESS_KEY=orchis ORCHIS_S3_SECRET_KEY=orchis-dev-secret
./bin/server serve &
TOKEN=$(./bin/server injector-token nmap)        # ulangi untuk nuclei
docker run -d --network host -e ORCHIS_SERVER_URL=http://127.0.0.1:8090 -e ORCHIS_INJECTOR_TOKEN=$TOKEN \
  -e ORCHIS_AMQP_URL=$ORCHIS_AMQP_URL orchis-injector:dev --kind nmap
```

Token injector juga bisa dibuat dari **Settings → Integrations**. Alamat loopback ditolak
kecuali `ORCHIS_ALLOW_LOOPBACK_TARGETS=true` (hanya untuk pengembangan).

### Agent endpoint

```bash
make agent-release VERSION=1.0.0          # bangun + tandatangani semua platform ke dist/agent
make osquery-fetch                        # artefak osquery yang dipatok (diverifikasi SHA-256)
go run ./cmd/agenttool -file deploy/osquery/osquery-5.24.0_1.linux_x86_64.tar.gz \
  -key dist/release.key -out dist/agent   # sajikan osquery untuk agent
ORCHIS_AGENT_RELEASE_DIR=$PWD/dist/agent ./bin/server serve
```

Lalu **Organisations → organisasi → Install agent** memberi perintah pemasangan sekali pakai.
Untuk rilis baru, gunakan versi lebih tinggi dan kunci yang **sama** agar agent terpasang dapat
memperbarui diri. Simpan `dist/release.key` dengan aman: siapa pun yang memegangnya bisa merilis
agent. Beberapa agent di satu mesin memerlukan nama instance berbeda (`--name`).

Build Windows, macOS, dan arm64 tersedia, tetapi belum dijalankan pada perangkat nyata.

### Renderer PDF

```bash
npm --prefix deploy/report ci
./deploy/report/node_modules/.bin/playwright install chromium
node deploy/report/render.test.mjs   # membuktikan renderer tidak mengakses HTTP eksternal
```

MinIO harus dikonfigurasi untuk PDF. JSON tetap tersedia bila rendering PDF gagal.

### Pengaturan lain

- `ORCHIS_MITRE_URL`: sumber STIX ATT&CK (URL atau berkas di dev; `off` mematikan impor).
  Produksi memakai bundel Enterprise beku dengan `ORCHIS_MITRE_SHA256` wajib. Lihat
  [operasi MITRE](docs/OPERASI.md#attck-mitre-beku).

## Data demo

Data demo bertanda dan dapat dihapus kembali. Butuh database yang sudah dimigrasikan dan memiliki admin.

```bash
DEMO_SEED=7 make demo-seed
make demo-status
make demo-purge
```

Fixture mengisi organisasi, asset, agent, temuan, laporan, data endpoint, dan contoh fitur
lanjutan. Hasil sintetis tidak dihitung sebagai hasil scan nyata, akun demo tidak dapat login,
dan alamat berizin hanya loopback. Bila `cleanup_pending>0`, pulihkan backend lalu ulangi purge.
Lihat [operasi data demo](docs/OPERASI.md#data-demo).

## Deployment

Image server, compose terisolasi, HTTPS lokal, renderer internal, backup terenkripsi dengan
restore, dan metrik Prometheus/Grafana tersedia melalui target `make prod-*` dan
`deploy/prod/ops.py`. Ikuti [docs/OPERASI.md](docs/OPERASI.md) untuk secret di luar repo,
batas resource, dan prosedur upgrade (backup → migrate → start → smoke).

Worker AI (fitur lanjutan, mati bawaan) memakai `deploy/ai/compose.yml` serta
`ORCHIS_AI_ENCRYPTION_KEY` (32 byte acak, base64) dan `ORCHIS_AI_WORKER_TOKEN` (minimal 32 karakter).

## Tes

```bash
make dev-up-full injector-image
make mitre-fetch osquery-fetch     # artefak yang dipatok; flow14/15 gagal tanpa ini
(cd e2e && npx playwright install chromium)
make check                         # Go vet/test, frontend, seluruh flow E2E, egress AI
```

`make check` menjalankan tes backend dengan PostgreSQL, RabbitMQ, Elasticsearch, MinIO, dan
scanner asli di container; tes frontend; serta flow E2E di Chromium termasuk pemeriksaan
responsif (1280/834/390 px) dan aksesibilitas axe-core. Butuh juga dependensi renderer PDF.

- Alur sederhana saja: `E2E_SCRIPT=flow16.mjs ./e2e/run.sh` (dua agent Linux, osquery/Nmap/Nuclei
  nyata pada fixture lokal, PDF/JSON).
- Bila broker RabbitMQ dipakai instalasi lain, buat vhost khusus tes dan set **keduanya**
  `ORCHIS_TEST_AMQP_URL` dan `E2E_AMQP_URL` ke vhost itu. Hapus hanya vhost tes sesudahnya.

Uji mutasi untuk cek keamanan berjalan di salinan terisolasi `work/` dan memulihkan kode setelahnya:

```bash
python3 e2e/mutation-organization-scope.py
python3 e2e/mutation-scan-scope.py
python3 e2e/mutation-endpoint-collection.py
python3 e2e/mutation-report-snapshot.py
python3 e2e/mutation-agent-address.py
python3 e2e/mutation-core.py
```

## API lama

`POST /api/v1/organizations/{id}/assessments` (profil `quick|full|web`, header `Idempotency-Key`
wajib) masih tersedia untuk kompatibilitas. Alur baru memakai `POST /api/v1/scans`.

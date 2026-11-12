# Pengembangan Orchis

Jalankan perintah dari root repo. Pasang Go sesuai `go.mod`, Node.js sesuai
[`.nvmrc`](../.nvmrc), npm,
Docker Engine + Compose v2 + Buildx (BuildKit), serta Python 3.12+. Dependensi Node dipisahkan per komponen:

```bash
nvm use                          # bila memakai nvm
npm --prefix web ci
npm --prefix e2e ci
npm --prefix deploy/report ci
make help
```

CI memakai versi Node dari `.nvmrc` yang sama. Jalankan `make web-test` untuk
pemeriksaan tipe, kontras, dan tes frontend; `make test` untuk Go. Persyaratan
layanan dan pemeriksaan lengkap dijelaskan di bagian Tes di bawah.

## Menjalankan

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
  -key dist/release.key -out dist/agent   # sajikan osquery untuk agent Linux
go run ./cmd/agenttool -file deploy/osquery/osqueryd-macos-bare-5.24.0.tar.gz -os darwin -arch amd64 \
  -key dist/release.key -out dist/agent   # macOS Intel
go run ./cmd/agenttool -file deploy/osquery/osqueryd-macos-bare-5.24.0.tar.gz -os darwin -arch arm64 \
  -key dist/release.key -out dist/agent   # macOS Apple Silicon
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
  [operasi MITRE](OPERASI.md#attck-mitre-beku).

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

## Build dan engine privat

`make build` membangun frontend, server, dan utilitas backup untuk pengembangan.
`make injector-image` membuat scanner pengembangan; image produksi dibangun oleh
`make prod-build`. Dockerfile produksi membangun binary dari source checkout ini.

Untuk menjalankan orchis-redteam-agent, siapkan kedua checkout privat sesuai
[panduan engine](../deploy/hackbot/README.md), lalu jalankan `make hackbot-check`.
`make hackbot-image` membangun engine dan worker, sedangkan `make build-test`
memeriksa validasi source serta urutan build tanpa menjalankan Docker.

Tes CI memakai fixture lokal dan tidak memerlukan kredensial model AI.
Tes engine privat dijalankan di checkout pendamping; source privat tidak disalin
ke repo Orchis atau ke image server.

Endpoint monitoring uses catalog-only, consent-checked outbound agent claims and PostgreSQL notifications for browser SSE. See [OPERASI.md](OPERASI.md) for configuration, Windows limits and the required server-first rollout.

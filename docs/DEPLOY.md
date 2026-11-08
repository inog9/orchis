# Menjalankan Orchis dengan Docker Compose

Satu berkas `docker-compose.yaml` di root repo menjalankan seluruh stack: server, PostgreSQL, RabbitMQ,
Elasticsearch, MinIO, renderer PDF, worker Nmap/Nuclei, Hackbot/OpenCode, DefenseBot, dan Caddy (HTTPS). Layanan `migrate` berjalan
sekali sebelum server (migrasi lalu provisioning); server baru start setelah keduanya sukses.

## Prasyarat build

Gunakan Linux amd64, Docker Engine + Compose v2 + Buildx (BuildKit), Git, dan Python 3.12+.
Siapkan **kedua checkout privat** (`orchis-hackbot-lite` dan
`orchis-opencode-hackbot`) sesuai revisi dan hash di
[`engine-lock.json`](../deploy/hackbot/engine-lock.json). Lihat
[panduan setup engine](../deploy/hackbot/README.md#build-and-configure).

```bash
make hackbot-check                       # berhenti dengan petunjuk bila source/pin tidak cocok
```

Lokasi bawaan adalah direktori saudara repo Orchis. Gunakan
`ORCHIS_HACKBOT_SOURCE` dan `ORCHIS_OPENCODE_SOURCE` untuk lokasi lain.
`prod-build` memeriksa keduanya sebelum memulai build image server atau engine.
Akun operator build harus dapat membaca source tersebut dan mengakses Docker.

## Langkah

```bash
sudo make prod-init                      # rahasia acak di /var/lib/orchis (di luar checkout, izin 0600)
sudo python3 deploy/prod/ops.py env > .env    # nilai non-rahasia untuk compose (lihat .env.example)
sudo make prod-build                     # server, renderer, scanner, engine dan worker Hackbot
sudo make prod-up                        # docker compose up + tunggu health dan heartbeat injector
sudo make prod-probe                     # HTTPS, login, renderer
```

Login pertama memakai `/var/lib/orchis/initial-admin.json` (email dan password acak); ganti password setelah masuk.
Untuk instalasi lain di host yang sama: `ORCHIS_STATE`, `--project`, dan `--port` berbeda
(`python3 deploy/prod/ops.py init --state /var/lib/orchis-x --project orchis-x --port 18443`).

`docker compose up` langsung dari root juga bekerja bila `.env` sudah ada, tetapi `make prod-up` lebih aman
karena menunggu seluruh health check dan memastikan migrasi sukses.

## HTTPS (self-signed dahulu)

Bawaan Caddy memakai CA internalnya sendiri (`tls internal`). Sertifikat tidak dikenal browser sampai CA
dipercaya. Root CA diambil dari `https://<host>:<port>/ca.pem`; sidik jarinya dicetak oleh

```bash
python3 deploy/prod/ops.py ca-fingerprint
```

Bandingkan sidik jari itu lewat jalur lain sebelum mempercayai CA. Bila nanti ada domain publik,
konfigurasikan `ORCHIS_TLS` dan `ORCHIS_AUTO_HTTPS` pada environment Compose Caddy
serta port/DNS untuk ACME; rahasia dan data tidak berubah. Lihat Caddyfile dan
[operasi HTTPS](OPERASI.md#https-dan-akses-awal).

## Backup

Backup terenkripsi lokal: `make prod-backup` (kunci terpisah `backup.key`). Salin arsip dan kunci ke
penyimpanan lain secara manual bila ingin off-host. Restore ke project baru: `make prod-restore`.
Prosedur lengkap ada di [OPERASI.md](OPERASI.md).

## Menghentikan

```bash
make prod-down        # mempertahankan volume
```

Menghapus data adalah langkah terpisah dan eksplisit: `docker compose -p <project> down -v` hanya pada
project yang dipilih.

## Rilis dan kepercayaan agent

`prod-init` membuat direktori `agent-releases` kosong. Sebelum memakai **Install agent**,
bangun rilis bertanda tangan dan paket osquery. Pertahankan kunci privat yang sama untuk
upgrade agent; jangan memakai kunci baru pada instalasi yang sudah memiliki agent.
Agent memakai versi 1.3.x, terpisah dari versi produk.

```bash
make agent-release VERSION=1.3.1 RELEASE_KEY=$ORCHIS_STATE/release.key RELEASE_DIR=$ORCHIS_STATE/agent-releases
make osquery-fetch
go run ./cmd/agenttool -file deploy/osquery/osquery-5.24.0_1.linux_x86_64.tar.gz \
  -key "$ORCHIS_STATE/release.key" -out "$ORCHIS_STATE/agent-releases"
go run ./cmd/agenttool -file deploy/osquery/osqueryd-macos-bare-5.24.0.tar.gz -os darwin -arch amd64 \
  -key "$ORCHIS_STATE/release.key" -out "$ORCHIS_STATE/agent-releases"   # macOS Intel
go run ./cmd/agenttool -file deploy/osquery/osqueryd-macos-bare-5.24.0.tar.gz -os darwin -arch arm64 \
  -key "$ORCHIS_STATE/release.key" -out "$ORCHIS_STATE/agent-releases"   # macOS Apple Silicon (berkas yang sama, tanda tangan berbeda)
python3 deploy/prod/ops.py agent-trust
docker compose -f docker-compose.yaml -f deploy/prod/compose.ops.yml up -d --no-deps --force-recreate server
```

Set `ORCHIS_STATE` ke direktori state absolut sebelum perintah di atas. `agent-trust`
mengambil CA dari container Caddy milik project tersebut, bukan dari koneksi jaringan
belum dipercaya. Gunakan setelah `prod-up`; jalankan ulang bila CA diganti.

Konfigurasi berikut berada di `server.json` privat, bukan `.env` Compose:

- `ORCHIS_AGENT_RELEASE_PUBKEY`: kunci publik Ed25519, base64 32 byte. Bila kosong,
  server membaca `release.pub` dari direktori rilis. `agentrelease` menulis berkas publik ini;
  berkas ini aman disajikan ke konsol. Kunci privat `release.key` tidak boleh dipublikasikan.
- `ORCHIS_AGENT_CA_FINGERPRINT`: SHA-256 DER CA internal; diisi oleh `agent-trust`.
  Installer mencocokkan CA dan menyimpannya untuk agent melalui `--ca-file`.
- `ORCHIS_AGENT_TLS_PIN`: alternatif untuk sertifikat self-signed langsung, berbentuk
  `sha256//BASE64` dari SPKI. Bila fingerprint CA tersedia, fingerprint CA didahulukan.

Mode **user** hanya mendaftarkan agent. Jalankan perintah startup yang dicetak installer
pada terminal pengguna, dan biarkan terminal itu berjalan. Contoh instance `lab`:
`"$HOME/orchis-agent-lab/orchis-agent" run`. Mode **service** memasang dan menjalankan layanan.

## Agent dari mesin lain

Bawaan `prod-init` hanya mendengarkan di loopback. Untuk host yang harus dapat
menerima koneksi dari agent lain, pilih alamat antarmuka server dan identitas TLS
secara eksplisit saat membuat state baru:

```bash
python3 deploy/prod/ops.py init --state /var/lib/orchis-lan --project orchis-lan \
  --port 8443 --bind 192.168.10.20 --site 192.168.10.20
ORCHIS_STATE=/var/lib/orchis-lan python3 deploy/prod/ops.py env > .env
```

Ganti alamat contoh dengan alamat server yang dapat dijangkau agent. `--bind 0.0.0.0`
juga tersedia bila memang ingin semua antarmuka IPv4; batasi akses port HTTPS dengan
firewall. `--site` menerima hostname atau IPv4 tanpa skema/path/port. Bila memakai
hostname, DNS agent harus mengarahkannya ke alamat server. URL konsol dan pemasangan
harus memakai identitas `--site` yang sama agar verifikasi hostname TLS tetap berlaku.
Bawaan ini tetap memakai CA internal; HTTPS publik/ACME belum dibuktikan.

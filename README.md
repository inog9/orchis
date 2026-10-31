# orchis-go

ORCHIS versi ringan: platform validasi keamanan (breach & attack simulation) dengan backend Go.

Status: Fase 0–7, Fase 8a (responsif/aksesibilitas), Fase 9, audit UX Fase 9.5, dan implementasi lokal Fase 10 tersedia. Fase 9 menambahkan organisasi, beberapa instance agent per mesin, assessment organisasi dengan aset yang dilewati secara eksplisit, katalog aksi berversi, kondisi/mapper per aset, laporan organisasi beku (PDF/HTML/JSON/CSV), dan Home berbasis organisasi. Penjadwalan ditunda; AI nyata belum diaktifkan; sisa pengerasan produksi Fase 8 tetap tercatat. Arsitektur, keputusan, dan batasan: [docs/ARSITEKTUR.md](docs/ARSITEKTUR.md).

## Mulai dari tugas, bukan istilah internal (Fase 9.5)

Setelah login, Home memandu lima langkah berdasarkan organisasi yang dipilih: buat organisasi, pasang agent atau tambah aset, set alamat dan otorisasi, Assess organisation, lalu baca laporan. Langkah izin membutuhkan setidaknya satu target siap; aset lain tetap dapat dilaporkan dilewati dengan alasannya. Instalasi agent tidak memberi izin scan dan tidak mempercayakan alamat dari agent secara otomatis.

Navigasi kini berlabel dan punya bantuan saat hover atau fokus keyboard (Escape menutup). Pencarian, aksi cepat, dan notifikasi berada di header agar tidak menutupi isi halaman. Start berisi Home/Organisations/Assets; Test berisi Actions/Scenarios/Simulations; Results berisi Findings/Reports; Manage berisi Settings. Actions menghubungkan katalog ke Single tests (dulu Atomic testing). Scenarios menghubungkan rencana/target ke chain editor. MITRE, People dan tautan lama tetap bisa dibuka lewat palet Ctrl/⌘K, menu workspace, atau URL lama. Definisi baru tidak menjalankan scan sampai Run/Launch ditekan.

Detail aset menampilkan temuan nyata per aset; tautan ke Findings mempertahankan filter aset. Detail organisasi menampilkan pratinjau izin dan scope per aset serta tautan perbaikan. Nmap/Nuclei dijalankan scan worker (injector) container yang harus menjangkau target; agent menjalankan pemeriksaan lokal endpoint. PDF gagal dapat dicoba ulang tanpa scan baru; snapshot dan ekspor HTML/JSON/CSV tetap tersedia.

Reproduksi audit pada database disposable: `E2E_SCRIPT=flow9.mjs E2E_UX_ROUND=review ./e2e/run.sh`; flow9 juga masuk default `make check`. Uji mutasi tambahan: `python3 e2e/mutation-ux.py` (salinan terisolasi di work/). Pengguna sudah menyetujui Fase 10 dengan DeepSeek/OpenAI dan tes API palsu lokal. Pengiriman data AI nyata tetap memerlukan key, limit dan consent organisasi.

## Bantuan AI per organisasi (Fase 10)

**Organisations → organisasi → AI assistance** menyediakan consent, model/key provider, budget, pratinjau data dan riwayat. DeepSeek mengusulkan check allowlist yang memerlukan approval manusia. OpenAI menyusun saran remediasi sebagai suplemen PDF/HTML/JSON/CSV, dengan prompt/version tercatat. Model tidak mengubah snapshot laporan asli atau menjalankan shell.

AI mati default; seluruh bukti integrasi memakai API palsu lokal. Model ID, harga akun dan provider nyata belum diuji. Kredensial write-only dienkripsi, payload laporan memakai alias dan metadata teknis terbatas. Target scan tetap dijaga pada Run/Claim/injector. Panduan, API dan keterbatasan: [docs/ARSITEKTUR.md](docs/ARSITEKTUR.md), ADR 082–091.

```sh
make ai-image
make check                                  # seluruh flow, kini termasuk flow10 dan egress AI
E2E_SCRIPT=flow10.mjs ./e2e/run.sh            # provider palsu, worker container, Nmap/Nuclei nyata
python3 e2e/mutation-ai.py
```

Verifikasi Fase 10: `make check` lengkap lulus pada `f08acc4` (124 tes frontend, sepuluh flow, flow10 68/0); 12 mutasi terdeteksi/dipulihkan, axe nol pada 1280/834, egress 6/0. Polesan cetak berikutnya diuji ulang terarah. Jika broker dipakai instalasi review, tetapkan `E2E_AMQP_URL` ke vhost disposable khusus tes; lihat panduan hasil.

`deploy/ai/compose.yml` adalah recipe worker/proxy dengan network internal; server aplikasi harus dapat dijangkau pada network itu. Server memerlukan `ORCHIS_AI_ENCRYPTION_KEY` (32 byte acak, base64) dan `ORCHIS_AI_WORKER_TOKEN` (minimal 32 karakter). Worker menggunakan `ORCHIS_SERVER_URL`, token yang sama dan `ORCHIS_AI_PROXY`. Simpan melalui secret manager/env terlindungi; jangan memakai fixture e2e untuk deployment. Instalasi review lama belum dimigrasikan/restart.

## Fondasi deployment T1

Image server multi-stage, compose terisolasi, HTTPS lokal dan renderer internal
sedang diverifikasi. Ikuti [panduan operasi](docs/OPERASI.md) untuk setup secret di
luar repo, batas resource dan batas pembuktiannya. T1 belum dinyatakan selesai;
backup/restore dan gerbang produksi lengkap masih berjalan. Tidak ada push.

## Menjalankan

```bash
make dev-up                     # PostgreSQL di 127.0.0.1:15432
make build                      # frontend + server
./bin/server migrate
ORCHIS_WEB_DIR=$PWD/web/dist ORCHIS_ADMIN_EMAIL=admin@orchis.local ./bin/server serve
```
Bila `ORCHIS_ADMIN_PASSWORD` kosong, password admin pertama dibuat acak dan dicetak sekali di log.

### Menjalankan scan (Fase 3)
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
Token injector juga bisa dibuat dari **Settings → Integrations**.

### Agent endpoint
```bash
go run ./cmd/agentrelease -version 1.0.0 -key dist/release.key -init -out dist/agent   # bangun + tandatangani (semua platform)
ORCHIS_AGENT_RELEASE_DIR=$PWD/dist/agent ./bin/server serve
```
Lalu **Organisations → organisasi → Install agent** (atau **Assets → Agents → Install agent**) di konsol memberi perintah pemasangan sekali pakai. Untuk merilis versi baru, jalankan `agentrelease` lagi dengan `-version` lebih tinggi dan kunci yang SAMA; agent yang terpasang memperbarui dirinya. Simpan `release.key` dengan aman: siapa pun yang memegangnya bisa merilis agent.

### Pengaturan lain
- `ORCHIS_MITRE_URL`: sumber STIX ATT&CK. Bawaan = bundel resmi MITRE (diunduh otomatis setelah server naik, lalu dicek berkala); isi `off` untuk lingkungan tanpa internet.
- Akun baru dibuat admin di **Settings → Users** dan mendapat link sekali pakai (30 menit) untuk memilih password; tidak ada email.

Aset harus ditandai *boleh diuji* oleh Admin. Alamat loopback ditolak kecuali `ORCHIS_ALLOW_LOOPBACK_TARGETS=true` (hanya dev).

## Assessment dan laporan organisasi

1. Buat organisasi, lalu pasang agent atau assign aset yang sudah ada. Aset lama tetap tersedia sebagai **Unassigned**.
2. Admin menetapkan alamat yang bisa dijangkau injector dan mengotorisasi pengujian per aset; pendaftaran agent tidak memberi izin scan.
3. Pilih profil Quick exposure, Full TCP + services, atau Web CVEs, kemudian **Assess organisation** untuk seluruh organisasi, atau centang aset dan pilih **Assess selected assets**. Kegagalan satu aset tidak menghentikan aset lainnya.
4. Buka **Reports** untuk snapshot organisasi, print preview, PDF, HTML, JSON, dan CSV. Aset yang dilewati atau scan gagal tetap terlihat dalam cakupan.

Nmap/Nuclei berjalan di container injector yang harus dapat menjangkau aset. Untuk beberapa agent pada mesin yang sama, berikan nama instance berbeda; perintah pemasangan membuat direktori terpisah dan service memakai nama instance. Untuk opsi `--name`, rilis agent harus dibangun dari kode Fase 9 ini. Saat deployment, gunakan nomor versi lebih tinggi dan kunci penandatanganan yang sama agar instalasi lama dapat memperbarui diri. Agent Windows/macOS/arm64 terbukti ter-build, belum dijalankan pada perangkat nyata dalam sesi ini. Dua instance baru diuji dalam mode user; instalasi service bernama belum diuji langsung.

Dependensi renderer PDF:
```bash
npm --prefix deploy/report ci
./deploy/report/node_modules/.bin/playwright install chromium
node deploy/report/render.test.mjs   # membuktikan renderer tidak mengakses HTTP eksternal
```
MinIO harus dikonfigurasi untuk PDF. HTML/JSON/CSV tetap tersedia bila rendering PDF gagal; **Retry PDF generation** tidak menjalankan scan lagi. Tidak ada data yang dikirim ke API AI.

## Tes
`make check` (termasuk pemeriksaan responsif 1280/834 px dan aksesibilitas axe-core pada semua halaman) menjalankan tes backend (PostgreSQL, RabbitMQ, Elasticsearch, MinIO, dan scanner asli di container), tes frontend, dan tes end-to-end di Chromium (termasuk container injector). Butuh `make dev-up-full`, `make injector-image`, dan dependensi renderer PDF di atas. `E2E_SCRIPT=flow8.mjs ./e2e/run.sh` menguji alur organisasi hingga PDF, kondisi/mapper, dan axe di 1280/834 px.
Verifikasi audit UX: 119 tes frontend, sembilan flow E2E; `flow9.mjs` mencatat 283 lulus/0 gagal; flow7 mencakup 116 pemeriksaan di dua viewport. Uji mutasi Fase 9 (3) dan UX (4) terdeteksi/dipulihkan, renderer nol HTTP. Bukti lengkap dan keterbatasan ada di laporan audit.
Tes end-to-end memerlukan `npx playwright install chromium` di folder `e2e/`.

Uji mutasi yang dapat diulang (memakai salinan terisolasi di `work/`, tidak merusak checkout aktif):
```bash
python3 e2e/mutation-phase9.py
```
Script memeriksa baseline hijau, memastikan mutasi otorisasi/kondisi/snapshot membuat tes yang tepat gagal, lalu memulihkan kode dan memastikan tes kembali hijau. Log bukti disimpan di direktori salinan tersebut.

API assessment: `POST /api/v1/organizations/{id}/assessments` memerlukan header `Idempotency-Key` dan menerima profil `quick|full|web`, `asset_ids` opsional, serta override `nmap_params`/`nuclei_params` yang tetap divalidasi. Header yang sama dengan opsi sama mengembalikan assessment yang sama; opsi berbeda menghasilkan 409. Aset tanpa organisasi memakai filter `GET /api/v1/assets?organization_id=unassigned`.

## Desain

## Gerbang alur inti

`E2E_SCRIPT=flow11.mjs E2E_CORE_ROUND=review ./e2e/run.sh` menjalankan alur organisasi hingga laporan dari database baru, dua agent Linux, scanner nyata pada fixture lokal, perbandingan temuan teratasi, kegagalan parsial, serta retry PDF tanpa mengulang scan. Hasil dan nama organisasi ditandai LOCAL FIXTURE. Bukan laporan keamanan pelanggan. Script masuk default `make check`.

Bila broker dipakai instalasi lain, buat vhost disposable dan berikan izin ke user tes; setel **keduanya** `ORCHIS_TEST_AMQP_URL` dan `E2E_AMQP_URL` ke vhost tersebut sebelum `make check`. Hapus hanya vhost tes sesudahnya. Uji mutasi tambahan: `python3 e2e/mutation-core.py`.

Endpoint web yang tidak merespons sebelum/sesudah Nuclei dan scan TCP tanpa respons dicatat incomplete/error. Kegagalan atau aset yang dilewati tidak menyimpulkan temuan teratasi. Closed TCP ports yang merespons tetap merupakan scan sah dengan nol finding port terbuka.

Ringkasan laporan kini membedakan nol kerentanan yang teramati pada cakupan selesai dari tidak adanya cakupan lengkap. Total aset diuji/dilewati/incomplete dan alasan kegagalan terlihat langsung; keadaan port terbuka tetap terpisah dari hitungan kerentanan berdasarkan severity. Snapshot assessment lama tetap beku.

Verifikasi T0 pada `f7cee76` (31 Oktober 2026): `make check` default seluruh 11 flow lulus, 126 tes frontend/17 berkas; flow11 98/0, flow9 283/0, egress AI 6/0. Enam belas mutasi terdeteksi/dipulihkan; renderer nol HTTP. Dua puluh enam screenshot dan seluruh 12 halaman dari tiga PDF fixture ditinjau pada kode yang sama. Ini pembuktian alur inti lokal, bukan kesiapan infrastruktur produksi atau studi pengguna manusia.


Infrastruktur T1 tersedia melalui `deploy/prod/ops.py` dengan state privat di luar
repo, HTTPS lokal, backup age dan restore ke project baru. Petunjuk operasi,
batas backup saat idle, pemisahan kunci, retensi dan smoke produksi tersendiri
`node e2e/flow12.mjs` ada di [docs/OPERASI.md](docs/OPERASI.md). T1 belum dinyatakan
selesai sampai pemeriksaan metrik, rilis, volume dan gerbang verifikasi terpenuhi.


Metrik internal server/injector/AI dan profile ops Prometheus/Grafana tersedia;
petunjuk setup/upgrade `metrics-init`, SSH tunnel, alert dan batas metrik ada di
[operasi](docs/OPERASI.md#metrik-dan-dashboard-ops-opsional). Listener metrik tidak
dipublikasikan. Dummy penuh, osquery dan gerbang akhir tetap tahap berikutnya.

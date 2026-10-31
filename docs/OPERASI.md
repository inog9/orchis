# Operasi ORCHIS

Status: komponen T1 telah diimplementasikan dan diuji terarah; pemeriksaan CI
akhir masih menunggu. Deployment yang dibuktikan memakai HTTPS lokal, bukan
DNS/ACME publik atau SLA produksi. Gerbang T1 belum dinyatakan selesai.

## Menyiapkan satu instalasi terisolasi

Prasyarat: Linux amd64, Docker Engine + Compose v2, Python 3.12+, akses registry
image dan modul saat build, serta ruang/RAM sesuai batas di compose. Go dan Node
host tidak diperlukan untuk image server: frontend dan biner dibangun dalam
multi-stage Dockerfile. Runtime server statis memakai UID 10001, rootfs hanya
baca, tanpa capability. PostgreSQL/RabbitMQ/Elasticsearch/MinIO tidak memublikasikan
port host. Server hanya dapat dijangkau melalui Caddy pada port HTTPS loopback.

Jalankan sebagai operator root agar secret 0600 dapat dimiliki UID container:

```sh
export ORCHIS_STATE=/var/lib/orchis
make prod-init
make prod-build
make prod-up
make prod-status
```

`prod-init` menolak direktori yang sudah ada: tidak menimpa secret. Direktori
state 0700 harus di luar checkout. JSON konfigurasi per layanan hanya berisi
variabel `ORCHIS_*`, dibaca langsung saat proses mulai, tidak menjadi argumen
container atau keluaran `docker inspect`. Secret bukan variabel interpolasi
Compose. Pemilik secret menyesuaikan UID pembaca; bind mount Compose tidak
menjamin pemetaan uid/mode seperti secret manager Swarm.

Kredensial awal di `initial-admin.json` (0600/root) hanya untuk operator. Jangan
menyalin ke chat/log/shell history. Password acak setup berbeda per instalasi;
simpan pada pengelola password lalu ganti melalui Profile. Kunci master AI ada
terpisah di `ai-master.key`; jangan kehilangan, mencetak atau memasukkannya ke git.
Tidak ada consent organisasi atau API key provider yang dibuat saat setup;
worker kosong tidak memanggil DeepSeek/OpenAI. Produksi menolak URL provider tes.

PostgreSQL menggunakan akun `postgres` hanya untuk init volume, owner `orchis`
untuk migrasi satu kali, `orchis_app` untuk server (bukan pemilik tabel dan tidak
BYPASSRLS), serta `orchis_maintenance` untuk pemeliharaan/backup lintas tenant.
Server tidak menerima password owner/maintenance. UID secret PostgreSQL Alpine dipatok ke 70; RabbitMQ ke 999. Init berhenti bila pembacaan secret gagal, sebelum SQL dijalankan. Migrasi harus sukses sebelum
provisioning token injector; provisioning harus sukses sebelum server. Restart
server tidak menjalankan migrasi atau merotasi token. `prod-up` memeriksa health
termasuk database, MQ, ES, MinIO, renderer dan API, dengan batas waktu.

`operator.json` menentukan project, bind/port dan subnet proxy khusus. Default
`orchis-prod`, `127.0.0.1:8443`, `localhost`. Setup memilih subnet /24 yang tidak
bertabrakan dengan network Docker yang ada; operator tetap harus meninjau konflik
VPN/rute jaringan. Alamat Caddy tetap pada jaringan proxy khusus, dan server
memercayai hanya /32 tersebut. Header XFF dari peer lain diabaikan; rantai berjalan
dari kanan, berhenti pada hop pertama yang tidak dipercaya. Caddy tidak masuk
jaringan database, tetapi server masuk kedua jaringan yang diperlukan.

## HTTPS dan akses awal

Default Caddy memakai CA lokal (`tls internal`). Ambil hanya **sertifikat publik**
CA dari volume Caddy ke trust store klien; jangan memindahkan private key CA.
Akses `https://localhost:8443`, bukan HTTP. Cookie sesi Secure/HttpOnly/SameSite=Lax.
Jangan memakai `-k` sebagai prosedur produksi. HSTS, batas body 17 MB, kompresi,
CSP, no-sniff dan anti-frame diaktifkan. Ubah `ORCHIS_SITE` pada konfigurasi
operator hanya setelah DNS tersedia. Untuk domain publik, tinjau Caddyfile,
ganti `tls internal` dengan ACME dan publikasikan 80/443 pada proxy. Recipe lokal
belum membuktikan DNS/ACME publik.

Renderer berada pada network internal tersendiri tanpa egress/published port.
API renderer memerlukan token acak, satu job bersamaan, input maksimum 16 MiB,
PDF maksimum 32 MiB dan batas waktu. Browser menonaktifkan JavaScript, service
worker dan jaringan; HTML/PDF memakai halaman cetak yang sama. Server tidak
menjalankan Node di image produksinya. Mode dev tetap memakai script lokal.

## Menghentikan dan memeriksa

```sh
make prod-status
make prod-down
```

`prod-down` mempertahankan seluruh volume. Tidak ada perintah prune global atau
`down --volumes`. Untuk project lain gunakan state berbeda; jangan memakai state
review/OpenAEV. Semua container diberi batas memori, CPU dan PID. Jumlah batas
bukan kebutuhan RAM minimum; ukur pemakaian aktual dengan `docker stats` dan
pastikan host masih mempunyai ruang bagi aplikasi lain.

Sumber: [Compose secrets](https://docs.docker.com/compose/how-tos/use-secrets/),
[Caddy reverse proxy](https://caddyserver.com/docs/caddyfile/directives/reverse_proxy),
[body limit](https://caddyserver.com/docs/caddyfile/directives/request_body),
[Playwright Docker](https://playwright.dev/docs/docker).

## Bukti komponen deployment T1

31 Oktober 2026, project `orchis-t1`, state `/var/lib/orchis-t1`, port loopback
8443: build server/web, renderer dan scanner berhasil. Server image 16.893.318
byte, UID 10001. Runtime server/renderer/Nmap/Nuclei/AI-worker non-root dan
rootfs read-only diverifikasi melalui Docker metadata. Migrasi/provisioning
selesai sebelum API; core services health hijau. Readiness kini menunggu heartbeat terautentikasi Nmap/Nuclei; integrasi yang sengaja disabled dibedakan. Metrik/health worker lebih rinci masih T1.

`make prod-probe` membuktikan HTTPS dengan verifikasi CA (tanpa bypass TLS), login,
cookie Secure/HttpOnly/SameSite, PDF nyata 25.208 byte, permintaan renderer tanpa
token 401, serta koneksi keluar renderer ditolak. Spoof XFF dari klien diabaikan
ketika last_login_ip disimpan. App role melihat nol users tanpa konteks tenant;
owner/app NOSUPERUSER/NOBYPASSRLS, maintenance BYPASSRLS. Seluruh 6.105 berkas
template image lulus SHA256SUMS. Template v10.5.1 dibatasi tiga direktori HTTP
dan dipatok hash arsip; lisensi upstream dipertahankan (lihat NOTICE).

Caddy memerlukan NET_BIND_SERVICE untuk capability berkas binernya dan port 443;
capability lain dihapus. Access log proxy dimatikan agar query reset/enrollment
tidak tercatat; log akses JSON API hanya memuat path tanpa query. CA privat tetap
pada volume. Readiness operator mengecek health layanan inti serta proses worker,
bukan menganggap semua worker memiliki endpoint health. RabbitMQ CLI dibatasi
scheduler terpisah agar healthcheck tidak menghabiskan batas PID.

Masalah startup yang sudah diperbaiki: quoting tmpfs YAML, UID PostgreSQL Alpine,
propagasi error pembacaan secret, scheduler CLI RabbitMQ, capability Caddy dan
perbedaan Compose --wait untuk container tanpa healthcheck. Seluruh verifikasi
akhir memakai recipe yang sudah diperbaiki. Probe ini belum membuktikan scan
assessment, backup/restore, studi volume, CI atau gerbang T1 lengkap.

## Pembatasan publik dan audit

Pada mode produksi, login dan reset/check reset berbagi batas grup masing-masing
10 request/IP/60 detik; enrollment20. Registry4096 IP/grup terbatas dan ditolak bila
penuh sampai jendela kedaluwarsa. Ini limiter per proses untuk satu API. Lock
per akun disimpan di database dan hitungan serentak diserialisasi. IP datang dari
peer tepercaya setelah pemrosesan XFF kanan-ke-kiri; header kiriman klien tidak
dapat mengganti peer. Batasi perubahan daftar proxy pada operator.

Settings → Audit log memerlukan audit.read. Filter event, Previous/Next dan metadata
aktor/waktu/IP/resource/request tersedia. Login/logout/kegagalan dicatat dalam
transaksi yang sama; kegagalan audit membatalkan login. Email tebakan akun tak
dikenal tidak disimpan: kegagalan memakai bucket IP dan aktor Unauthenticated.
API audit tidak mengeluarkan detail mentah atau kredensial. Parameter SQL tidak
dicatat pada log PostgreSQL. Riwayat akun dikenal tetap bersifat data audit yang
dibatasi permission.

Uji ulang UI audit pada DB disposable: E2E_SCRIPT=access-audit.mjs ./e2e/run.sh.
Script juga masuk default make check. Bukti staging:38 pemeriksaan, axe0 1280/834,
empat screenshot diperiksa. Ini membuktikan UI/API nyata; limiter produksi juga
diuji melalui HTTP handler dan hitungan audit yang tidak bertambah setelah limit.

## CI lokal dan toolchain

```sh
make dev-up-full
make ci
```

Go minimum1.26.10 ditetapkan go.mod dan digest builder. Host dengan Go lama dapat
menggunakan auto-toolchain Go; tidak ada perubahan ke Go global mesin oleh recipe
ini. govulncheck dipatokv1.9.0. CI menolak kegagalan audit npm, test/build/E2E, atau
chunkJS>500000 byte. Halaman memakai dynamic import; chunk terbesar303kB pada
bukti awal. Vhost CI unik per run dan dipakai kedua variabel antrean; cleanup
tidak menyentuh vhost aplikasi lain. Backend dev harus sudah aktif. CI tidak
memanggil API AI asli dan tidak membuat workflow cloud.

Pemindaian1.26.8 awal menemukan13 advisory stdlib terjangkau; patch1.26.10
menutupnya. Hasil patched0reachable/0imported, tiga advisory modul yang tidak
dipanggil tetap dilaporkan govulncheck. Audit npm web/E2E/renderer nol. Full CI
pada commit baru masih harus selesai sebelum gerbang T1 dinyatakan lulus.

Perbaikan provisioning: token injector harus mengikuti kontrak oij_ +32 byte
base64url. Generator dan job provision memvalidasi format sebelum startup.
Readiness tidak cukup melihat proses running: dua injector enabled harus punya
heartbeat terbaru. Integrasi disabled oleh admin tetap dihormati. Perubahan secret
pada bind mount diterapkan dengan prod-down/prod-up agar proses membaca ulang.
Uji upgrade schema13→14 memakai owner NOSUPERUSER/NOBYPASSRLS, data tebakan lama
dihapus tanpa menonaktifkan RLS. Stack Go1.26.10/Caddy2.11.7 lulus probe TLS/login/
PDF/egress setelah build ulang; rate limit diuji pada HTTPS produksi nyata.

Rotasi token injector: gunakan aksi rotasi Integrations, simpan nilai baru pada
file privat worker dan provisioning, kemudian prod-down/prod-up. Provisioning
menolak secret yang berbeda dari token tersimpan; tidak meresurrect token yang
sudah dicabut. Jangan mengedit JSON dengan echo token ke terminal/history.

Endpoint lupa password (/auth/forgot), reset dan pemeriksaan link berbagi budget
reset publik yang sama; berpindah endpoint tidak menambah kuota. Tes tabel menjaga
bahwa tiga jalur tersebut dibatasi. CI lengkap32c4b52 sudah lulus sebelum komponen
backup/restore berikutnya; gerbang T1 keseluruhan masih berjalan.


## Backup terenkripsi dan restore terisolasi

Jalankan sebagai operator root. State privat berada di luar checkout; contoh ini
memilih satu project secara eksplisit. Simpan salinan `backup.key` secara terpisah
di penyimpanan operator yang terlindungi sebelum mengandalkan backup. Jangan
menaruh kunci di git, log, argumen berisi nilainya, atau direktori arsip. Kunci
256-bit acak dibuat saat init; `backup-key` hanya membuat bila belum ada.

```sh
python3 deploy/prod/ops.py backup-key --state /var/lib/orchis-prod
python3 deploy/prod/ops.py backup --state /var/lib/orchis-prod --retention-days 7
python3 deploy/prod/ops.py restore --state /var/lib/orchis-restore \
  --project orchis-restore --port 8444 \
  --archive /var/lib/orchis-prod/backups/ARSIP.zip.age \
  --key-file /path/privat/salinan-backup.key
```

Nama arsip aktual dicetak tanpa isi secret. Default batas input arsip 8 GiB dan
headroom disk tambahan 1 GiB; ubah `--max-bytes` sesuai volume. Retensi lokal default
7 hari (`--retention-days` atau ORCHIS_BACKUP_RETENTION_DAYS), bukan kebijakan retensi
organisasi. Hanya arsip lengkap dengan pola milik backup ini yang dibersihkan.
Arsip memakai format age standar dengan pustaka age1.3.2, ZIP streaming dan scrypt
berbatas; tidak ada arsip plaintext pada disk sumber. Secret, database, objek PDF,
state TLS dan artefak rilis ada di dalam enkripsi. Kunci backup tidak disertakan.

Backup menghentikan sementara writer project terpilih, menolak eksekusi aktif,
simulasi paused/running, assessment aktif, laporan queued/rendering dan pekerjaan
AI queued/running. Selesaikan atau batalkan pekerjaan tersebut lebih dulu. Broker
RabbitMQ dibangun ulang saat restore; pesan antrean tidak dicadangkan. Setelah
berhasil maupun gagal, hanya layanan yang sebelumnya berjalan yang dinyalakan
kembali. Jangan memakai backup ini untuk memindahkan pekerjaan yang masih aktif.

Restore hanya menerima state/project/volume baru; tidak menimpa instalasi lama.
Dekripsi dan validasi archive/path/link/ukuran selesai sebelum init target. File
sementara restore plaintext dibatasi izin0700 dan dihapus sesudahnya. Restore
memverifikasi jumlah entitas, hash snapshot serta byte PDF, lalu membangun ulang
indeks Elasticsearch dan menjalankan probe HTTPS/login/renderer. Bila password
admin telah berubah, berikan `--credentials-file` berisi JSON email/password privat
0600 untuk probe; initial-admin.json bukan sumber password mutakhir. Target
mendapat kunci backup baru; simpan juga salinan terpisah untuk backup berikutnya.
Pemulihan ini lokal dan terisolasi; salinan off-host dan retensi organisasi masih
harus ditetapkan operator.

### Smoke produksi yang dapat diulang

```sh
node e2e/flow12.mjs
python3 -m unittest discover -s deploy/prod -p backup_test.py
```

Flow12 membuat project baru, port HTTPS loopback acak, fixture HTTP backend-only,
scan Nmap/Nuclei asli, satu aset diuji dan dua dilewati, satu PDF, backup terenkripsi
serta restore pada project lain. Akhirnya hanya container/volume/network berlabel
tepat milik run dihapus. Arsip/kunci tetap dalam state privat di luar git. Tidak
ada API AI nyata. Hasil ringkas ada di e2e/shots/production-smoke/result.json.

Bukti manual 31 Oktober 2026: satu organisasi, tiga aset, dua temuan (port terbuka
dan laravel-env high dari fixture sintetis), satu assessment/laporan. Snapshot
JSON dan PDF byte-identik setelah restore tanpa scan ulang; Elasticsearch dua
dokumen dibangun ulang. PDF tiga halaman A4 ditinjau seluruhnya. Lima tes safety
Python dan tes age (kunci salah, manipulasi, truncation, trailing data) lulus.
Ini bukti komponen backup; T1 keseluruhan masih menunggu metrik, rilis dan volume.


## Metrik dan dashboard ops opsional

Server, kedua injector dan worker AI mempunyai listener internal9101 `/metrics`.
Setup menghasilkan token privat terpisah; request tanpa bearer token ditolak401.
Tidak ada port metrik host atau route Caddy menuju listener ini. Label HTTP memakai
grup route tetap dan kelas status; ID aset, alamat target, query dan kredensial
tidak masuk label. Counter HTTP/scan/AI reset saat proses restart. Latensi HTTP
tersedia sebagai sum/count untuk rata-rata; bukan histogram persentil.

Untuk memperbarui state yang dibuat sebelum dukungan metrik, hentikan project,
jalankan `metrics-init`, lalu mulai kembali. Perintah ini mempertahankan credential
lain, token metrik yang ada, dan password Grafana yang sudah dibuat.

```sh
python3 deploy/prod/ops.py down --state /var/lib/orchis-prod
python3 deploy/prod/ops.py metrics-init --state /var/lib/orchis-prod
COMPOSE_PROFILES=ops ORCHIS_GRAFANA_PORT=3000 \
  python3 deploy/prod/ops.py up --state /var/lib/orchis-prod
```

Grafana hanya dipublikasikan pada127.0.0.1:3000; gunakan SSH tunnel untuk akses
jarak jauh. User awal `admin`; password berada pada berkas privat grafana_password
(UID472), tidak dicetak. Prometheus tidak mempunyai port host. Dashboard Orchis
operations dan datasource diprovisi dari file; tidak perlu plugin eksternal.
Jaringan UI Grafana terpisah dari backend aplikasi; pengambilan metrik tetap pada
jaringan internal. Analytics dan pemeriksaan/pemasangan plugin otomatis dimatikan.
Image OSS Grafana13.3.0 dan Prometheus3.15.1 dipatok digest, bukan latest. Dukungan
konfigurasi mengikuti [Prometheus](https://prometheus.io/docs/prometheus/latest/configuration/configuration/)
dan [Grafana](https://grafana.com/docs/grafana/latest/administration/provisioning/).

Delapan alert dasar dimuat: endpoint hilang, heartbeat worker stale, DLQ, lease/
antrean terlambat, backup gagal, belum ada bukti backup, disk rendah, kolektor
DB/antrean gagal. Alert terlihat di Prometheus; pengiriman notifikasi tetap backlog
Nanti. Gunakan maintenance window ketika backup/upgrade menghentikan worker.
Batas60 detik heartbeat dan5 menit queued adalah default operasional, bukan SLA.
Antrean mengukur pesan ready, tidak termasuk unacked; konsumen ditampilkan terpisah.
Gauges DB/antrean diperbarui15 detik dan harus dibaca bersama status kolektornya.
Umur heartbeat agent memakai agen non-revoked tertua serta hitungan tanpa heartbeat.
Ukuran DB adalah seluruh database instalasi satu tenant ini.

Status backup non-secret ditulis atomik ke ops-status/backup.json dan dipasang
sebagai direktori read-only pada server. Tidak ada kunci atau nama arsip di metrik.
Disk diukur langsung pada filesystem direktori state, sehingga berubah tanpa
menunggu backup berikutnya. Histori time-series/Grafana tidak termasuk backup
bisnis; restore memprovisi ulang dashboard dan dapat memulai histori baru.
Token metrik boleh diganti dalam berkas privat, lalu `metrics-init` dan down/up;
jangan mencetak nilai token ke terminal. Password Grafana yang sudah dipakai harus
dirotasi melalui Grafana sendiri; file awal tidak mengubah akun yang telah ada.

Pemeriksaan manual ops: `deploy/prod/monitoring/probe.mjs` memverifikasi empat
endpoint, menolak anonim dan memeriksa kolektor/heartbeat; setel
ORCHIS_PROBE_PROMETHEUS=1 di container helper pada backend untuk memeriksa empat
target Prometheus dan delapan rule. `grafana-probe.mjs` pada jaringan ops memeriksa
autentikasi, datasource dan sepuluh panel tanpa mencetak password. Flow12 memakai
probe metrik dasar otomatis. Tes rule: promtool check rules alerts.yml dan promtool
test rules alerts.test.yml dari direktori monitoring (tmpfs /tmp diperlukan).

Bukti aktual: empat target sehat, delapan rule dimuat, tes DLQ/backup gagal lulus,
Grafana autentikasi/datasource/sepuluh panel lulus. Batas awal Grafana64 PID terlalu
kecil untuk plugin bawaan; setelah GOMAXPROCS2/PID256/memori512MiB, penggunaan aktual
sekitar267MiB/111 PID. Metrik memakai log JSON stdout pada server/injector/AI;
renderer dan dependensi mempertahankan logger masing-masing. Ini belum menyatakan
T1 lengkap: rilis agent, volume serta pemeriksaan gerbang masih harus ditutup.


## Rilis agent dan akar kepercayaan

```sh
make agent-release VERSION=1.2.0 \
  RELEASE_KEY=/var/lib/orchis-prod/release.key \
  RELEASE_DIR=/var/lib/orchis-prod/agent-releases
ORCHIS_TEST_RELEASE_DIR=/var/lib/orchis-prod/agent-releases \
ORCHIS_TEST_RELEASE_KEY=/var/lib/orchis-prod/release.key \
  go test -v ./cmd/agentrelease
```

Gunakan versi lebih tinggi untuk perubahan berikutnya. Default Makefile masih
dist untuk pengembangan; produksi wajib memilih state privat di luar repo.
Kunci Ed25519 dibuat hanya bila belum ada, secara eksklusif0600, lalu di-fsync.
Berkas publik, symlink, rusak atau galat akses ditolak; `-init` tidak mengganti
akar kepercayaan yang telah ada. Manifest ditulis atomik setelah seluruh artefak.
Simpan salinan terenkripsi kunci rilis; kunci juga termasuk backup age bisnis.

Enam target: linux/darwin/windows, masing-masing amd64/arm64. Bukti T1 versi1.2.0
memverifikasi semua hash/tanda tangan serta penolakan digest yang diubah; perintah
version Linux amd64 dijalankan. Windows/macOS/Linux arm64 hanya build, tidak
mengklaim install/service atau osquery pada perangkat tersebut. Rilis review lama
tidak diubah. Kunci deployment T1 terpisah dari fixture review; pertahankan kunci
T1 yang sama untuk rilis osquery berikutnya.

Kunci publik tertanam pada agent. Rotasi kunci rilis tidak dapat dilakukan hanya
dengan mengganti release.key: agent lama akan menolak tanda tangan baru. Lakukan
migrasi kepercayaan yang disetujui melalui pemasangan ulang agent bertanda tangan
akar baru; dokumentasikan dampak dan simpan akar lama untuk armada yang belum
bermigrasi. Jangan menerbitkan artefak baru memakai nomor versi lama.


## Studi volume dan rencana kueri

`make volume-study` membutuhkan backend dev lokal. Script memakai database tes
baru dan indeks Elasticsearch dengan UUID unik, bukan data review/produksi.
Dataset sintetis tetap:50 organisasi,5000 aset,200000 temuan,50 snapshot laporan
(100 aset/4000 temuan per snapshot). Tidak menjalankan scanner atau provider AI.
State laporan sintetis tidak menjadi bukti eksekusi MITRE; semua resource dihapus.
Prasyarat disk4GiB, context12menit, statement seed120detik, timeout harness15menit.
Marker nama resource hanya dipakai run ini; finally menghapus database/indeks yang
cocok pola tepat, termasuk jika tes gagal. Tidak memakai prune atau pola wildcard
untuk menghapus resource. Hasil/EXPLAIN ANALYZE BUFFERS disimpan di work/volume/.

Angka nyata31 Oktober2026, host bersama6CPU/15.6GiB, PostgreSQL17 dan ES8 dev lokal.
Sepuluh sampel berurutan sesudah satu warm-up; satu proses, bukan uji load serentak,
SLA atau kapasitas pelanggan. Waktu milidetik median (min–max) hasil akhir:

| Operasi layanan aktual | Sebelum | Akhir (rentang) |
| --- | ---: | ---: |
| Home lengkap |471.32|465.84 (421.29–561.04)|
| Daftar temuan25 |105.85|1.58 (1.36–2.22)|
| Filter severity25 |28.31|1.73 (1.39–2.71)|
| Search fallback substring |164.62|150.27 (139.35–249.02)|
| Search Elasticsearch+fetch |36.01|40.00 (29.64–47.59)|
| Baca/decode snapshot4000 temuan |52.14|49.15 (43.49–60.44)|

Migrasi00015 menyelaraskan indeks dengan urutan last_seen_at/id termasuk severity.
Plan halaman berubah dari parallel scan+top-N-sort200000 baris (79.43ms) menjadi
index scan26 baris (plan akhir sekitar1ms). Menambah indeks saja sempat membuat
fallback substring memilih heap scan acak berurutan indeks: median595.74ms.
Perbaikan query mematerialisasi ID yang cocok sebelum memilih halaman dan baru
mengambil details untuk ID halaman; median kembali150.27ms. Tenant, soft-delete,
severity, literal wildcard dan cursor tetap berlaku; tes batas/cursor ditambahkan.
Tidak mematerialisasi evidence seluruh hasil pencarian.

Database akhir157947571 byte (~150.6MiB), seed32.39detik, bulk index200000 dokumen
14.49detik. JSON rencana adalah kueri akses representatif; waktu tabel berasal
pemanggilan Service.Summary/ListFindings/Get sebenarnya, termasuk decoding Go.
Ukuran JSON laporan turut diukur, bukan render PDF ratusan halaman atau pembuatan
snapshot dari eksekusi. Query Home masih melakukan agregasi penuh; fallback tanpa
ES tetap substring scan. Perbedaan kecil antar-run pada host bersama tidak dianggap
percepatan. Ambang kinerja dan uji beban serentak tetap backlog Nanti.


## Upgrade dan pemecahan masalah

Sebelum upgrade, pastikan pekerjaan selesai/dibatalkan dan buat backup terenkripsi
beserta salinan kunci yang terpisah. Simpan versi image lama. Bangun versi baru
terlebih dahulu dengan ORCHIS_VERSION yang berbeda, lalu down/up project yang
dipilih. Migrasi terkontrol dijalankan owner sebelum provisioning dan API. Untuk
state sebelum metrik, jalankan metrics-init saat layanan berhenti. Jalankan probe
HTTPS/login/renderer, periksa heartbeat dan buka laporan yang sudah ada. Perubahan
secret bind mount memerlukan pembuatan ulang container agar file terbaru terbaca.

```sh
ORCHIS_VERSION=VERSI_BARU python3 deploy/prod/ops.py build --state /var/lib/orchis-prod
python3 deploy/prod/ops.py backup --state /var/lib/orchis-prod
python3 deploy/prod/ops.py down --state /var/lib/orchis-prod
python3 deploy/prod/ops.py metrics-init --state /var/lib/orchis-prod
ORCHIS_VERSION=VERSI_BARU python3 deploy/prod/ops.py up --state /var/lib/orchis-prod
python3 deploy/prod/ops.py probe --state /var/lib/orchis-prod
```

Jangan menjalankan migrasi down terhadap data aktif sebagai rollback otomatis.
Untuk rollback skema/data gunakan restore ke project/volume baru dari backup versi
sebelumnya, lalu verifikasi sebelum memindahkan endpoint. Uji upgrade awal13→14
memakai owner NOSUPERUSER. Migrasi00015 menambah indeks sehingga mengambil lock;
jadwalkan maintenance window untuk database besar. Studi volume tidak menggantikan
uji upgrade organisasi sendiri.

Jika startup gagal: gunakan prod-status, lalu log service milik project tepat.
Jangan mencetak file JSON secret atau seluruh Docker environment. Migration gagal:
periksa role owner, volume dan nomor skema. Worker401: periksa status integrasi dan
keselarasan token oij_, jangan menerapkan token lama setelah rotasi. PDF gagal:
periksa renderer/MinIO dan gunakan retry laporan, tanpa mengulang scan. Audit auth
429: tunggu Retry-After; jangan memperlonggar guard. Restore gagal validasi: simpan
arsip/kunci sumber, pilih state kosong baru. Pending rekey: lanjutkan dengan kunci
baru yang sama sebagaimana di bawah.

Password admin yang telah diganti tidak direset saat restart; initial-admin.json
hanya credential awal. Simpan JSON credential operator terkini0600 jika diperlukan
untuk probe. Password DB/MQ/S3 yang tersimpan pada volume tidak berubah hanya dengan
mengedit file init. Rotasi akun layanan memerlukan maintenance window, perubahan
credential pada layanan tersebut, penyelarasan file privat seluruh konsumennya,
restart dan probe; jangan menciptakan ulang volume produksi untuk merotasi password.
Rotasi token worker/API tersedia melalui UI yang berizin; nilai lama tidak berlaku.

## Rotasi master key AI dengan re-enkripsi

Rotasi memakai kunci acak32byte base64 baru, tanpa menghubungi provider. Buat file
baru0600 di state privat (contoh Python, tidak mencetak nilainya):

```sh
python3 - <<'PY'
import base64,os,secrets
path='/var/lib/orchis-prod/new-ai-master.key'
fd=os.open(path,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
with os.fdopen(fd,'wb') as out:
    out.write(base64.b64encode(secrets.token_bytes(32))+b'\n')
    out.flush();os.fsync(out.fileno())
PY
python3 deploy/prod/ops.py backup --state /var/lib/orchis-prod
python3 deploy/prod/ops.py ai-rekey --state /var/lib/orchis-prod \
  --key-file /var/lib/orchis-prod/new-ai-master.key
python3 deploy/prod/ops.py probe --state /var/lib/orchis-prod
python3 deploy/prod/ops.py backup --state /var/lib/orchis-prod
```

Jalankan saat instalasi idle. Precheck menolak pekerjaan aktif sebelum menghentikan
layanan. API/worker/proxy terpilih dihentikan; ciphertext ai_policies tenant diubah
atomik dan diaudit, kemudian file master dipromosikan atomik, layanan sebelumnya
berjalan dibuat ulang dan health ditunggu. Consent tidak otomatis diaktifkan dan
tidak ada job provider baru. Version policy naik sehingga pekerjaan review lama
harus ditinjau kembali terhadap policy baru. Kunci provider plaintext tidak dicetak.

Jika berhenti sesudah commit DB tetapi sebelum file master dipasang, ai-rekey.json
menahan startup/backup. Jalankan ulang perintah dengan file kunci baru yang sama;
baris yang sudah terenkripsi dengan kunci baru diterima secara idempoten, lalu
promosi diselesaikan. Jangan menghapus jurnal untuk melewati penjaga ini. Gagal
sebelum perubahan DB akibat pekerjaan baru akan mengembalikan layanan. Pada galat
lain layanan ditahan agar tidak memakai master yang salah. Pemulihan alternatif
adalah restore database dan key bersama dari backup lama ke project baru.

ai-master.previous.key dipertahankan0600 pada state privat untuk operator; backup
bisnis menyimpan master saat ini, dan backup sebelum rotasi menyimpan master lama.
Simpan salinan kunci/arsip sesuai retensi organisasi. Jangan mengganti file master
secara langsung ketika ciphertext belum direenkripsi. Kunci master AI berbeda dari
kunci enkripsi backup maupun kunci tanda tangan agent.

## Data demo

Data dummy bertanda `is_demo` untuk review dan demonstrasi. Seed hanya lewat CLI (tidak ada endpoint seed):

```bash
DEMO_SEED=1 ./bin/server demo-seed     # isi (atau make demo-seed)
./bin/server demo-status               # hitungan per objek dan seed
./bin/server demo-purge                # hapus hanya data demo (atau tombol di banner UI, perlu settings.manage)
```

Purge gagal tertutup bila data nyata bergantung pada data demo (misalnya temuan nyata pada aset demo);
tidak ada penghapusan sebagian. Seed berbeda tanpa purge ditolak. Data demo berlabel di UI, PDF/HTML,
CSV (kolom `synthetic`) dan JSON; tidak pernah dihitung sebagai cakupan "trained" di MITRE.

## ATT&CK (MITRE) beku

Bundel Enterprise ATT&CK dibekukan di `deploy/mitre` (manifest ikut repo, bundel tidak):

```bash
make mitre-update VERSION=20.0   # unduh, validasi, tampilkan diff, tulis bundel + manifest
make mitre-verify                # periksa hash, versi, dan hitungan terhadap manifest
```

Untuk memakainya di server: arahkan `ORCHIS_MITRE_URL` ke berkas bundel dan `ORCHIS_MITRE_SHA256` ke hash di
`manifest.json`. Di produksi `ORCHIS_MITRE_URL=off` (bawaan `ops.py`) atau berkas beku dengan hash wajib;
unduhan HTTP tanpa hash ditolak saat start. Impor satu transaksi, idempoten; teknik yang hilang dari sumber
ditandai deprecated, bukan dihapus.

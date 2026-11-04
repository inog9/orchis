# Operasi ORCHIS

Deployment telah diuji dengan fixture lokal: HTTPS, scan nyata, PDF, backup terenkripsi
serta restore ke project terisolasi. HTTPS publik/ACME, backup off-host dan SLA produksi
belum dibuktikan. Ikuti batasan runtime dan platform pada bagian terkait.

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

Manifest Enterprise ATT&CK di `deploy/mitre/manifest.json` ikut repo; bundel besar
dapat dipulihkan tanpa mengubah pin:

```sh
make mitre-fetch                 # mengambil tepat versi, hash, ukuran dan hitungan manifest
make mitre-verify                # verifikasi offline; menolak artefak atau metadata yang berubah
```

`fetch` tidak mengubah manifest dan tidak mengunduh lagi bila cache sudah valid.
Penggantian cache atomik: hash, ukuran, versi, dan hitungan harus cocok terlebih dahulu.
Pemelihara yang memang memperbarui sumber menjalankan `make mitre-update VERSION=20.0`
dengan versi yang ditinjau, membaca diff baru/berubah/deprecated, lalu meninjau perubahan
manifest di Git. Laporan lama tetap memakai snapshot yang sudah dibekukan.

Build image server mengambil bundel yang dipatok bila belum ada pada konteks lokal,
memverifikasinya, dan menyalinnya ke `/opt/orchis/mitre` sebagai berkas read-only yang
dapat dibaca UID 10001. Untuk build tanpa internet, siapkan artefak valid terlebih dahulu.
Lisensi dan disclaimer MITRE lengkap disertakan dalam `/licenses/mitre.LICENSE`.
`prod-init` memasang path lokal dan SHA-256 dari manifest pada konfigurasi privat;
produksi menolak URL runtime, termasuk URL yang diberi hash. `off` masih tersedia
sebagai keputusan eksplisit untuk menonaktifkan fitur.

Instalasi lama tidak diubah otomatis. Sesudah meninjau pin dan membangun image yang cocok:

```sh
make prod-backup
make prod-mitre-config            # ORCHIS_STATE menunjuk hanya project yang dipilih
make prod-down
make prod-up
make prod-probe
```

Perintah konfigurasi mempertahankan kredensial dan izin 0600, tidak merestart container.
Jangan restart jika pembaruan konfigurasi gagal; perbaiki dan jalankan kembali.
Probe HTTPS memeriksa versi/hash impor dan jumlah induk/sub-teknik/taktik terhadap manifest.
Gunakan tag image versi tetap untuk upgrade/rollback; rollback database tetap melalui restore
backup, bukan migrasi turun. `make prod-smoke` membuktikan alur scan, PDF dan restore di
project fixture baru, tanpa mengubah instalasi yang ada.

Mapped berarti ada pemetaan aksi, trained berarti aksi nyata selesai sukses, dan
simulated berarti ada run atau referensi temuan sintetis. Ketiganya dapat beririsan;
badge Demo tetap terlihat pada teknik mapped/trained. Hitungan induk memakai ID
run unik, dan angka sintetis tidak pernah ditambahkan ke trained. Panel detail dapat
digulir; Escape kembali ke teknik yang dipilih.

## Data endpoint (osquery)

Agent dapat mengumpulkan data dari dalam mesin (OS, paket, akun, port listening, hardening, firewall) dengan osquery. Semua query
bernama dan hanya-baca; teks SQL tidak pernah dikirim server. Persetujuan per organisasi: sensitivitas rendah aktif bawaan; `users` dan
`network` perlu persetujuan admin (Organisations → Endpoint data, dialog menampilkan tabel yang dibaca; diaudit).

Pemasangan: artefak osquery yang dipatok (5.24.0, linux/amd64) ditandatangani dengan kunci rilis agent dan disajikan server dari
`<direktori rilis>/tools/`. Agent memasangnya sendiri di direktorinya saat query osquery pertama dibutuhkan.

```bash
make osquery-fetch                                # ambil artefak yang dipatok ke deploy/osquery (diverifikasi SHA-256)
go run ./cmd/agenttool -file deploy/osquery/osquery-5.24.0_1.linux_x86_64.tar.gz -key <release.key> -out <direktori rilis>
```

`agenttool` menolak menandatangani berkas yang hash/ukurannya berbeda dari yang dipatok. Rilis agent harus berversi setelah T4 agar
mengenal aksi osquery (`make agent-release`); agent lama melaporkan galat "tidak dikenal agent ini". Hanya Linux amd64 yang terbukti.
Hasil terakhir per aset+query tersimpan di `endpoint_datasets`; tidak ada riwayat. Mencabut persetujuan menghentikan pengumpulan dan
mengeluarkan datanya dari laporan baru.

Pack resmi osquery dibekukan di `internal/osquery/packs.json` (dihasilkan `make osquery-packs OSQUERYD=<biner dari artefak yang sama>`,
tidak diunduh saat runtime). Admin menyetujui satu pack per organisasi (Organisations → Endpoint data → Official packs); hanya query
berstatus "Runs" yang aktif, sisanya dilewati dengan alasan tertulis (data sensitif, tabel belum ditinjau, macOS/Windows, tanpa
sampel). `ossec-rootkit` membaca berkas dan tidak menjalankan apa pun. Persetujuan dan pencabutan tercatat di audit (`osquery.pack`)
beserta hash pack dan tabel yang dibaca. Chain dapat memakai paket server terpasang: kondisi `installed_software` dan mapper
`installed_software -> tags` pada langkah Nuclei (tabel tag beku; tanpa argumen mentah).


### Menjalankan demo pada project produksi yang dipilih

Tidak ada seed otomatis saat startup. Operator dapat mengaktifkan demo secara eksplisit:

```bash
python3 deploy/prod/ops.py demo-seed --state /var/lib/orchis --demo-seed 7
python3 deploy/prod/ops.py demo-status --state /var/lib/orchis
python3 deploy/prod/ops.py demo-purge --state /var/lib/orchis
```

Perintah memakai konfigurasi server project itu, termasuk bucket dan index yang dikonfigurasi.
Purge menghapus baris bisnis secara transaksional lalu membersihkan PDF dan dokumen pencarian
melalui antrean tahan restart. `has_demo=false` dengan `cleanup_pending>0` berarti baris bisnis
sudah dihapus, tetapi pembersihan artefak belum selesai. Banner menyediakan retry dan server
mencoba otomatis tiap lima detik. Pulihkan koneksi/konfigurasi MinIO dan Elasticsearch lalu ulangi
purge bila diperlukan. Jangan menghapus antrean secara manual. Bila CLI tanpa backend lengkap,
tugas tetap pending; jalankan kembali dengan konfigurasi penyimpanan dan pencarian yang benar.
Seed berikutnya ditolak sampai cleanup selesai, untuk mencegah retry menghapus PDF seed baru.

Purge menolak ketika PDF demo masih dirender atau data nyata bergantung pada induk demo.
Selesaikan render atau lepaskan relasi nyata tersebut terlebih dahulu. Audit operator tetap
tersimpan; hanya contoh audit sintetis yang dihapus. Report demo `queued` membutuhkan worker
laporan dan renderer normal, sama seperti report nyata; tidak ada status sukses tanpa PDF.


Fixture identitas demo tidak dapat dipakai untuk login: akun disabled, tanpa password, serta
penanda demo ditolak di autentikasi. Token dan link reset contoh sudah dibatalkan/kedaluwarsa;
token contoh di Profile berlabel synthetic dan tidak menyediakan rahasia. Tiga injector contoh
hanya menunjukkan status ilustratif, bukan worker yang terhubung. Home dan metrik worker tetap
memakai sumber nyata. Peserta, tim, grup, saved tests dan preset bernama Demo dapat ditinjau dari
halaman yang sama dengan data biasa. Alamat demo yang berizin hanya loopback; hostname fiktif
tidak diberi izin scan. Purge menolak penghapusan jika pengguna menambahkan relasi nyata ke
induk demo; lepaskan relasi tersebut terlebih dahulu.


Fixture demo juga menyediakan riwayat sintetis untuk tujuh status simulasi, cabang kondisi,
serta tiga versi laporan per organisasi dengan cakupan lengkap/gagal/dilewati. Label synthetic
berlaku untuk hasil, waktu, metodologi dan perbandingan; riwayat ini tidak pernah diterbitkan
ke worker atau dihitung sebagai MITRE trained. Gunakan scenario baru dengan target lokal yang
berizin untuk menguji eksekusi nyata. Format fixture saat ini versi 3; jika demo-status melaporkan
format lama, jalankan purge dan tunggu cleanup_pending=0 sebelum seed ulang. Snapshot lama
sengaja tidak diperbarui otomatis. Penghapusan dapat ditolak jika riwayat nyata menggunakan
induk demo; jangan menghapus referensi atau antrean melalui SQL untuk memaksa purge.


Demo juga mengisi AI assistance melalui API palsu yang dibuka sebentar pada loopback oleh CLI
seed itu sendiri. Tidak perlu key AI dan tidak perlu menjalankan ai-worker untuk menghasilkan
contoh. Policy tetap disabled, tanpa key tersimpan, dan read-only. Job review/completed/rejected/
canceled serta template berversi dan suplemen empat format semuanya sintetis; tidak dapat
di-approve untuk memindai. Tombol Run pada saved test tetap memakai guard otorisasi normal,
sedangkan hasil contoh yang sudah ada tidak pernah berasal dari proses scanner. Pemutakhiran
format fixture memerlukan purge dan cleanup selesai sebelum seed ulang.

Suplemen AI PDF memakai ORCHIS_RENDERER_URL/TOKEN yang sama dengan worker laporan normal pada
server produksi. Model/kredensial provider nyata tetap membutuhkan persetujuan terpisah dan
belum diuji; fixture ini tidak memanggil DeepSeek/OpenAI di internet.


Build injector pengembangan (`make injector-image`) memakai konteks khusus berisi binernya,
seperti worker AI. Direktori bin pada konteks server produksi tetap dikecualikan; jangan
membuka pengecualian untuk semua keluaran build atau rahasia agar build injector berhasil.

### Batas keluaran osquery

Server menerima data endpoint hanya melalui dataset query bernama dan aturan
pemetaan katalog. Output mentah, log bebas, serta temuan kiriman agent pada aksi
osquery dibuang. Diagnostik disunting; versi alat wajib berbentuk numerik.
Setiap dataset tersanitasi dibatasi 1 MiB termasuk escaping JSON, dengan sel
maksimal 512 karakter dan batas baris per query. `Truncated` berarti hasil parsial;
jangan menafsirkan baris yang tidak terkumpul sebagai bukti aman. Runner membatasi
stdout 8 MiB dan stderr 64 KiB. Query yang perlu root dilewati sebelum instalasi
bila agent berjalan tanpa hak tersebut. Seluruh pack selalu perlu opt-in admin,
termasuk bila ada query pack yang kelak diklasifikasikan bersensitivitas rendah.

### Versi yang didukung untuk instalasi dan upgrade

Batas dukungan awal mengikuti versi yang benar-benar diuji, bukan perkiraan versi
lama yang mungkin kompatibel. Instalasi Linux amd64 memakai Docker Engine 29.1.3,
Compose 2.40.3, dan Python 3.14.4. Build langsung memakai Go 1.26.10 atau lebih baru
sesuai `go.mod`; frontend diuji pada Node 22.22.1, sedangkan builder image memakai
Node 24.22.0 yang dipatok. Memakai build image tidak memerlukan Go/Node di host.
PostgreSQL 17 wajib; PostgreSQL major lebih lama dan upgrade major in-place belum
didukung. Broker 4.4, Elasticsearch 8.19.23, renderer Playwright 1.64.1 dan proxy
Caddy 2.11.7 mengikuti digest Compose/Dockerfile. Jangan mengganti major di luar
matriks ini dalam upgrade aplikasi yang sama. Windows/macOS/arm64 tetap build-only
untuk agent sampai pengujian perangkat nyata selesai.

Tentukan versi image aplikasi secara eksplisit. Skema awal yang terbukti untuk
upgrade ini adalah 17, kemudian migrasi berurutan sampai 26. Instalasi yang lebih
lama memerlukan rehearsal tersendiri; jangan mengasumsikan uji 17→26 mencakupnya.
Simpan image dan backup dari versi asal. Menjalankan image lama terhadap database
baru bukan rollback yang didukung: restore backup ke volume/database baru, jalankan
image asal, verifikasi, lalu pindahkan endpoint secara terencana. Jangan menjalankan
migrasi Down untuk rollback produksi.

Rehearsal database lokal yang hanya membaca sumber dan membersihkan DB salinan:

```sh
go build -o bin/server-t5-upgrade ./cmd/server
go build -o bin/backup-seal ./cmd/backup-seal
python3 e2e/upgrade-restore.py --source-db orchis_review \
  --out /var/lib/orchis-upgrade-proof-UNIK \
  --password-file /jalur/privat/password-admin-saat-ini
```

Password dibaca di dalam proses, tidak diberikan lewat argv atau dicetak. Direktori
bukti baru berizin 0700; dump dienkripsi dan dump plaintext sementara dihapus saat
selesai, termasuk saat gagal. Simpan `backup.key` terpisah dari arsip sesuai prosedur
backup produksi. Skrip membatasi sumber 512 MiB, mensyaratkan ruang bebas 2 GiB,
dan memberi timeout per perintah. Ini rehearsal DB dev, bukan pengganti backup
produksi lengkap (MinIO, kunci, rilis agent, dan sertifikat).

Bukti 1 November 2026: salinan `orchis_review` skema17 berukuran13.7 MB memuat
53 aset, 95 temuan, dan17 laporan. Enkripsi/dekripsi menghasilkan dump identik;
restore kedua tetap skema17 dan seluruh data asli. Upgrade ke26 memverifikasi
fingerprint SHA-256 proyeksi kolom lama pada63 tabel. Satu transformasi yang memang
diwajibkan migrasi22 diperiksa secara eksplisit:18 aset demo nonlokal kehilangan
otorisasi scan. Snapshot laporan dan data lain tetap identik. Login memakai password
lama serta API Home/Organisations/Assets/Reports dan HTML laporan lama lulus pada
salinan baru. Sumber review dan layanan yang sedang berjalan tidak diubah.

### Penolakan kredensial contoh

`prod-init` membuat setiap rahasia secara acak untuk satu state baru. Nilai tidak
dicetak; kredensial admin awal dibaca dari `initial-admin.json` berizin0600 sekali
oleh operator lalu disimpan di pengelola password. State yang sudah ada ditolak
agar init ulang tidak mengganti rahasia. Dua setup terpisah diuji menghasilkan
rahasia berbeda; seluruh file rahasia tetap0600. Admin yang sudah ada tidak direset
saat restart. Password awal acak menggantikan kebutuhan password contoh yang harus
diganti; operator tetap dapat menggantinya melalui Profile.

Mode produksi menolak fallback DB dan password DB pendek, password admin awal
kurang24 karakter, kredensial broker contoh atau akun guest, secret object storage
kurang24 karakter, dan token renderer kurang32 karakter. Layanan opsional yang
benar-benar tidak dikonfigurasi tidak memerlukan rahasia palsu. Error menyebut
jenis pengaturan tanpa nilai rahasianya. Gunakan `prod-init` untuk menghasilkan
nilai; jangan memperpanjang password contoh agar melewati batas panjang.

### QA PDF kecil dan besar

HTML dan PDF memakai halaman cetak yang sama. Parameter lampiran dicetak sebagai
JSON ringkas dan tetap di-escape, sehingga array berindentasi tidak menghabiskan
halaman sendiri. Snapshot tersimpan tidak ditulis ulang. Laporan sintetis mempunyai
penanda demo pada halaman pertama dan footer setiap halaman, termasuk lampiran.

Buat fixture QA tanpa pemindaian atau akses layanan eksternal:

```sh
go run ./e2e/report-preview -large -snapshot-out /tmp/demo-layout.json > /tmp/demo-layout.html
node deploy/report/render.mjs < /tmp/demo-layout.html > /tmp/demo-layout.pdf
python3 e2e/check-report-pdf.py /tmp/demo-layout.pdf /tmp/demo-layout.json
pdftoppm -png -scale-to 1200 /tmp/demo-layout.pdf /tmp/demo-layout-page
```

Perintah Python dan PNG memerlukan Poppler (`pdftotext`, `pdftoppm`) di mesin QA;
tidak dipasang pada host produksi atau diperlukan oleh renderer runtime. Untuk
snapshot nyata yang sudah diekspor, berikan JSON melalui stdin ke `report-preview`
tanpa `-large`. Jangan memasukkan snapshot/bukti privat ke Git.

Uji teks memeriksa A4, batas teks, setiap nama aset/judul temuan, cakupan dari snapshot,
footer provenance dan nomor halaman. Pemeriksaan itu melengkapi, bukan menggantikan,
tinjauan gambar setiap halaman. Bukti T5: laporan scan lokal nyata3 aset/2 temuan
berkurang4→3 halaman tanpa kehilangan isi; fixture sintetis12 aset/36 temuan adalah17
halaman, dengan8 diuji,3 dilewati,1 tidak lengkap. Seluruh20 halaman final ditinjau
sebagai PNG; tidak ditemukan clipping, tumpang tindih atau halaman kosong. Uji
renderer juga membuktikan tidak ada permintaan HTTP gambar/CSS/JavaScript.

### Persetujuan dan penghapusan data endpoint

Dokumen produk [ENDPOINT-CONSENT.md](ENDPOINT-CONSENT.md), versi2026-11-01.1,
ditampilkan sebelum admin menyetujui query atau pack pada Organisations → Endpoint
data. Daftar tabel tetap terlihat; dokumen dalam panel dapat digulir dengan pointer
atau keyboard. Tombol Approve menyetujui dokumen yang ditampilkan. API aktivasi
memerlukan `consent: {version, sha256}` dari katalog saat ini. Versi/hash yang tidak
cocok ditolak422; tidak ada izin baru yang tersimpan. Persetujuan lama yang tidak
merekam dokumen perlu diperbarui untuk query yang memerlukan persetujuan eksplisit.
Pemeriksaan dasar bersensitivitas rendah tetap mengikuti pengaturan bawaan; semua
pack selalu opt-in. Persetujuan ini tidak mengaktifkan AI atau penjadwalan.

Setting dan audit merekam pengguna, waktu server, versi/hash dokumen, query/pack,
serta tabel yang dibaca. Pack juga merekam versi/hash sumbernya. Recorded collection
approvals menampilkan identitas dan waktu; pencabutan terlihat di Audit log. Perubahan
persetujuan membatalkan hasil yang sedang berjalan, termasuk cabut lalu setujui ulang.

Retensi yang berlaku saat ini dinyatakan secara eksplisit: dataset sukses terakhir
bertahan sampai digantikan hasil sukses lain atau dihapus; belum ada kedaluwarsa
berdasarkan umur dan belum ada riwayat koleksi. Temuan, laporan beku, audit dan backup
memiliki siklus terpisah. Jangan menyetujui model ini bila tidak sesuai kebijakan
organisasi. Otomatisasi retensi/riwayat masih sisa T4, bukan klaim selesai T5.

Pada Assets → detail aset → Endpoint data, admin dengan `asset.write` dan
`settings.manage` dapat memilih Delete collected data. Dialog menjelaskan lingkupnya.
Permintaan memerlukan versi aset saat ini; perubahan bersamaan ditolak409. Penghapusan
diaudit, menghapus dataset terakhir saja, dan menaikkan versi aset agar hasil yang
sudah berjalan tidak mengisi ulang data yang dihapus. Query yang masih antre atau
permintaan manual berikutnya tetap dapat berjalan. Matikan query/pack atau cabut
izin pengujian terlebih dahulu untuk mencegah pengumpulan ulang. Pencabutan agent
menghentikan seluruh tindakan agent. Data demo bersifat read-only dan memakai purge
demo terpisah; tenant lain tidak dapat menghapus data ini.

Penghapusan dataset tidak menghapus temuan, snapshot laporan, audit, ataupun salinan
backup. Verifikasi dan kelola retensi masing-masing sesuai kebijakan operator;
jangan menganggap penghapusan tampilan endpoint sebagai penghapusan seluruh bukti.

Verifikasi akhir T5 mengulang upgrade17→26 dari backup skema17 yang sudah
dibekukan, kemudian restore ke17:63 tabel cocok (selain transformasi izin demo
yang memang diwajibkan), login/API/laporan lama lulus. Salinan sumber lain pada
skema24 juga lulus24→26 dan restore ke24 dengan66 tabel cocok. Rehearsal memakai
database disposable pada PostgreSQL dev; tidak memindahkan endpoint produksi.


## Retensi dan penghapusan organisasi

Admin mengatur masa simpan di Settings → Data retention. Bawaan findings 180 hari,
hasil scan 90 hari, laporan PDF/JSON 180 hari, dataset endpoint 30 hari, dan audit
365 hari (minimum 90). Definisi umur, status pending cleanup, dan langkah typed
confirmation dijelaskan di [RETENTION.md](RETENTION.md).

**Backup lama tetap berisi data organisasi sampai backup itu kedaluwarsa.**
Penghapusan dari aplikasi tidak menghapus arsip backup. Operator harus menerapkan
masa simpan backup tersendiri dan mengulang keputusan penghapusan/pencabutan pada
hasil restore sebelum membuka akses pengguna. Jangan melaporkan penghapusan
storage eksternal selesai selama status operasi masih pending.

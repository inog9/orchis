# Panduan pilot Orchis

Dokumen ini mengumpulkan yang perlu disiapkan sebelum Orchis dipakai pada pilot kecil: jaringan, HTTPS,
backup dan restore, upgrade, rilis agent, login pertama, batas yang diketahui, dan daftar go/no-go.
Semua perintah diambil dari berkas di repo ini. Yang belum dicoba ditandai **belum diverifikasi**.

Rujukan: [DEPLOY.md](DEPLOY.md) (instalasi), [OPERASI.md](OPERASI.md) (backup, upgrade, rilis agent),
[PANDUAN.md](PANDUAN.md) dan [USER-GUIDE.md](USER-GUIDE.md) (pemakaian).

## 1. Persyaratan jaringan

| Arah | Syarat |
|---|---|
| Browser/pengguna → server | HTTPS ke port yang dipublikasikan Caddy (`ORCHIS_HTTPS_PORT`, bawaan 8443). |
| Agent → server | Agent harus bisa membuka HTTPS ke alamat server (enroll, heartbeat, update). |
| Injector Nmap/Nuclei → target | Scan **dijalankan dari server Orchis**, bukan dari agent. Injector berada di network `scan` (tidak internal), jadi target harus terjangkau dari host server. Rentang yang tidak terjangkau tercatat gagal; itu tidak berarti host aman. |
| Server → internet | Renderer PDF, backend, dan layanan AI berada di network internal tanpa egress. Hanya `scan` dan `edge` yang tidak internal. |

Hal yang perlu diputuskan sebelum pilot:

- **Alamat bind dan nama situs.** Hasil `prod-init` memakai `bind=127.0.0.1` dan `site=localhost`
  (nilai ini ditulis ke `operator.json` di direktori state dan dibaca `ops.py env`). Dengan nilai itu
  server hanya bisa dibuka dari mesin yang sama; agent di mesin lain tidak akan terhubung.
  Untuk pilot lintas mesin, `ORCHIS_BIND` dan `ORCHIS_SITE` harus diganti ke alamat/nama yang dipakai
  agent. **Belum diverifikasi:** mengubah `operator.json` dengan tangan lalu `up`, serta penerbitan
  sertifikat internal Caddy untuk alamat IP. Uji dulu pada project terpisah.
- **Firewall.** Buka hanya port HTTPS server untuk agent dan pengguna. Jangan publikasikan RabbitMQ,
  PostgreSQL, Elasticsearch, atau MinIO (di compose semuanya berada di network internal).
- **Batas scan.** Bawaan 256 alamat per scan dan 4 job aktif per tenant (`ORCHIS_SCAN_MAX_HOSTS`,
  `ORCHIS_SCAN_CONCURRENCY`); alamat loopback ditolak kecuali pengembangan.
- **Waktu.** Jendela waktu scan memakai zona IANA; pastikan jam server benar (lihat
  [SCAN-CONTROLS.md](SCAN-CONTROLS.md)).

## 2. HTTPS dengan sertifikat self-signed dahulu

Caddy memakai CA internalnya sendiri (`tls internal` di `deploy/prod/Caddyfile`). Browser tidak
mempercayai sertifikat ini sampai root CA dipercaya. Root CA disajikan di `https://<server>:<port>/ca.pem`.

1. Cetak sidik jari root CA dari server:

   ```bash
   python3 deploy/prod/ops.py ca-fingerprint
   ```

   Perintah ini membaca `root.crt` dari kontainer Caddy dan mencetak SHA-256 DER-nya.
2. Sampaikan sidik jari itu ke operator agent lewat **jalur lain** (bukan dari server yang sama).
3. Pasang agent dengan `install.sh --ca-fingerprint <sidik-jari>`. Skrip mengambil `/ca.pem`,
   menerimanya hanya bila SHA-256-nya cocok, menyimpannya, dan memakainya untuk enroll. TLS tidak pernah
   dimatikan (tidak ada `curl -k`).
4. Mode `--pin` memakai pin SPKI sertifikat server. Pada mode ini agent hanya mempercayai CA sistem,
   jadi **update otomatis agent ke server self-signed tidak berjalan**. Untuk pilot gunakan
   `--ca-fingerprint`.
5. Untuk browser, pasang hanya sertifikat publik root CA ke trust store pengguna; jangan memindahkan
   private key CA.

**Domain publik dengan ACME** (opsional): ganti `tls internal` di Caddyfile dengan ACME, publikasikan
80/443, dan set `ORCHIS_SITE` ke domain. Rahasia dan data tidak berubah. **Belum diverifikasi:** DNS dan
ACME publik belum pernah dicoba di repo ini (OPERASI.md mengatakan hal yang sama).

Agent yang sudah terpasang tidak mengenal CA baru tanpa rilis agent yang memuat `--ca-file`
(lihat bagian 6).

## 3. Backup dan restore

Backup terenkripsi lokal (format age) dan restore ke project baru. Kunci backup (`backup.key`) dibuat
saat `prod-init` dan **tidak ikut** di arsip.

```bash
python3 deploy/prod/ops.py backup-key --state /var/lib/orchis-prod        # buat kunci bila belum ada
python3 deploy/prod/ops.py backup --state /var/lib/orchis-prod --retention-days 7
```

Aturan yang perlu dipatuhi:

- Simpan **salinan terpisah** `backup.key` di penyimpanan operator yang terlindungi sebelum mengandalkan
  backup. Tanpa kunci, arsip tidak bisa dibuka.
- Backup menghentikan sementara writer dan menolak berjalan bila ada eksekusi, scan, laporan
  queued/rendering, atau pekerjaan AI yang aktif. Selesaikan atau batalkan dulu.
- Pesan RabbitMQ tidak dicadangkan; broker dibangun ulang saat restore.
- Arsip memuat secret, database, objek PDF, state TLS, dan artefak rilis; kunci backup tidak.
- Retensi lokal bawaan 7 hari adalah retensi berkas backup, bukan retensi data organisasi.

**Off-host (opsional, manual).** Salin arsip dan kunci ke penyimpanan lain secara manual dan terpisah.
Orchis tidak mengirim backup keluar host. Jadwal otomatis juga belum ada: jalankan perintah di atas dari
penjadwal milik operator (cron/systemd timer). **Belum diverifikasi:** salinan off-host dan penjadwalan.

**Restore untuk membuktikan backup bisa dipakai**, ke state dan project **baru** (tidak menimpa instalasi lama):

```bash
python3 deploy/prod/ops.py restore --state /var/lib/orchis-restore \
  --project orchis-restore --port 8444 \
  --archive /var/lib/orchis-prod/backups/ARSIP.zip.age \
  --key-file /path/privat/salinan-backup.key
```

Restore memverifikasi jumlah entitas, hash snapshot, dan byte PDF, membangun ulang indeks Elasticsearch,
lalu menjalankan probe HTTPS/login/renderer. Bila password admin sudah diganti, berikan
`--credentials-file` (JSON email/password, izin 0600). Latih restore sebelum pilot dimulai dan catat hasilnya.

## 4. Upgrade

**Server tidak menjalankan migrasi sendiri saat start pada mode pengembangan**, dan pada stack compose
migrasi dijalankan oleh layanan `migrate` sebelum server. Jangan menjalankan migrasi `down` sebagai
rollback.

Urutan: **backup → migrate → start → smoke.**

```bash
ORCHIS_VERSION=VERSI_BARU python3 deploy/prod/ops.py build --state /var/lib/orchis-prod
python3 deploy/prod/ops.py backup --state /var/lib/orchis-prod
python3 deploy/prod/ops.py down --state /var/lib/orchis-prod
ORCHIS_VERSION=VERSI_BARU python3 deploy/prod/ops.py up --state /var/lib/orchis-prod   # migrate dijalankan lebih dulu
python3 deploy/prod/ops.py probe --state /var/lib/orchis-prod
```

(`metrics-init` hanya untuk state lama dari sebelum fitur metrik; lihat OPERASI.md.)

- Simpan image versi lama. Rollback = restore backup ke project baru, jalankan image lama, verifikasi,
  baru pindahkan endpoint. Menjalankan image lama pada database baru bukan rollback yang didukung.
- Jadwalkan jendela maintenance; sebagian migrasi mengambil lock.
- Setelah upgrade buka laporan lama dan periksa heartbeat injector.
- **Belum diverifikasi:** upgrade dari versi yang sudah ada di GitHub ke versi akhir v1.0.0. Jalur yang
  terbukti di OPERASI.md hanya skema 17 → 26; migrasi sesudahnya dan upgrade lintas build belum dilatih.

## 5. Login pertama dan alur awal

Kredensial awal ada di `<state>/initial-admin.json` (email dan password acak). Urutan:

1. Login lalu **ganti password**. Berkas awal bukan sumber password mutakhir.
2. Buat organisasi.
3. Pasang agent dari **Organisations → Install agent** (token sekali pakai, `install.sh`).
4. Tambah asset dan **otorisasi pengujian**; pendaftaran agent saja tidak memberi izin scan.
5. Beri **consent** pengumpulan data endpoint (Organisations → Agents/Endpoints → Configure endpoint
   collection); tanpa consent osquery tidak mengumpulkan data.
6. Jalankan scan pertama (Nmap/Nuclei, preset Quick) dan lihat hasilnya di Findings.
7. Buat report PDF/JSON dari tab Reports.

Peran: Admin, Analyst, Viewer ([ROLES.md](ROLES.md)). Tombol stop darurat dan jendela waktu scan ada di
[SCAN-CONTROLS.md](SCAN-CONTROLS.md).

## 6. Rilis agent

```bash
make agent-release VERSION=1.3.1 \
  RELEASE_KEY=/var/lib/orchis-prod/release.key \
  RELEASE_DIR=/var/lib/orchis-prod/agent-releases
```

- Gunakan **kunci rilis yang sama** dengan rilis sebelumnya dan **versi yang lebih tinggi**. Agent menolak
  versi lebih rendah (anti-downgrade), dan agent lama menolak tanda tangan dari kunci baru.
- **Cadangkan `release.key`** (terenkripsi, terpisah). Siapa pun yang memegangnya bisa merilis agent, dan
  kehilangannya berarti agent terpasang tidak bisa diperbarui lagi tanpa pasang ulang.
- Direktori rilis yang disajikan server adalah `<state>/agent-releases` (di-mount read-only ke server).
- osquery Linux amd64 disajikan dari `<direktori rilis>/tools/`: jalankan `make osquery-fetch`, lalu
  `go run ./cmd/agenttool -file deploy/osquery/osquery-5.24.0_1.linux_x86_64.tar.gz -key <release.key> -out <direktori rilis>`.
- Dukungan `--ca-file` (agent memercayai CA internal Caddy) hanya sampai ke agent terpasang lewat rilis
  agent baru di jalur versi agent. **Belum diverifikasi:** update otomatis nyata dari agent lama ke rilis itu
  pada server ber-CA internal.
- Versi produk (v1.0.0) dan versi agent (1.3.x) berjalan terpisah; lihat [CHANGELOG](../CHANGELOG.md).

## 7. Didukung dan tidak didukung

| Didukung dan terbukti | Belum / tersembunyi |
|---|---|
| Alur Organisasi → Agent/Asset → Scan Nmap+Nuclei → Findings → Reports PDF/JSON | osquery macOS dan Windows: tampil "belum didukung", dikerjakan nanti |
| Agent Linux amd64 dengan osquery 5.24.0 | Agent macOS/Windows/arm64: build tersedia, belum dijalankan di perangkat nyata |
| Scan dari server Orchis ke target yang terjangkau | Scan dari dalam agent: tidak ada |
| Peran Admin/Analyst/Viewer, jendela waktu dan stop darurat, perbandingan scan, notifikasi di aplikasi, retensi dan hapus organisasi, kontrol admin atas agent, UI EN/ID | Chains, Single tests/Atomic, AI, dan MITRE: tersembunyi, aktif hanya dengan `ORCHIS_ADVANCED_FEATURES=true` |
| Backup terenkripsi lokal dan restore ke project baru | Backup off-host otomatis, jadwal backup otomatis |
| HTTPS dengan CA internal Caddy | Domain publik dengan ACME (hanya terdokumentasi) |

## 8. Batas yang diketahui

- Hasil scan gagal karena target tak terjangkau tidak berarti target aman.
- Integrasi AI hanya diuji dengan API palsu lokal; provider nyata belum diuji.
- Tinjauan keamanan terarah dan uji keamanan independen belum dilakukan.
- Mode agent `--pin` tidak mendukung update otomatis (bagian 2).
- Consent endpoint berlaku per versi dokumen; persetujuan lama perlu diperbarui bila dokumen berubah.
- Data endpoint hanya menyimpan hasil terakhir per aset dan query, tanpa riwayat.
- Performa dan beban besar belum diukur.

## 9. Daftar go/no-go pilot

Centang semua sebelum pilot dimulai.

**Jaringan dan akses**
- [ ] `ORCHIS_BIND`/`ORCHIS_SITE` sesuai alamat yang dipakai agent dan pengguna; HTTPS terbuka dari mesin agent.
- [ ] Sidik jari CA dicocokkan lewat jalur terpisah; browser/agent mempercayai CA itu.
- [ ] Injector terbukti menjangkau semua target pilot (scan uji pada satu target).
- [ ] Firewall hanya membuka port HTTPS server.

**Data dan pemulihan**
- [ ] `backup` berhasil, `backup.key` dan `release.key` tersimpan terpisah di luar host.
- [ ] `restore` ke project baru berhasil dan dicatat tanggalnya.
- [ ] Penjadwal backup milik operator terpasang (bila diperlukan).

**Akun dan data**
- [ ] Password admin awal sudah diganti; `initial-admin.json` tidak dipakai lagi.
- [ ] Pengguna pilot dibuat dengan peran Admin/Analyst/Viewer yang tepat.
- [ ] Organisasi, asset, dan otorisasi pengujian pilot sudah ditetapkan tertulis; consent endpoint diberikan.
- [ ] Data demo tidak ada (`demo-status` menunjukkan `has_demo:false`).

**Fungsi**
- [ ] `prod-probe` lulus; agent pilot online dan versi sesuai.
- [ ] Satu scan Nmap+Nuclei selesai dan satu report PDF/JSON dibuat dan dibuka.
- [ ] Stop darurat dicoba sekali dan scan berhenti.

**Keputusan**
- [ ] Semua pihak menerima batas di bagian 7 dan 8 (khususnya: tanpa osquery macOS/Windows, tanpa tinjauan keamanan independen).
- [ ] Ada pemilik yang menjalankan upgrade dengan urutan backup → migrate → start → smoke.

Bila ada kotak yang tidak bisa dicentang, hasilnya **no-go** sampai alasannya jelas.

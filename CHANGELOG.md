# Changelog

Format mengikuti [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Produk dan agent memakai nomor versi **terpisah**: agent menolak turun versi
(anti-downgrade), jadi jalur versinya tidak boleh dipaksa sama dengan produk.
Prosedur rilis agent ada di [README](README.md#agent-endpoint).

## Produk

### [1.0.0] — belum dirilis

Rilis stabil pertama. Alur utama:
**Organisasi → Agent/Endpoint atau Asset → Scan (Nmap/Nuclei) → Findings → Reports (PDF/JSON)**.

#### Ditambahkan

- **Data endpoint di macOS (eksperimental):** osquery 5.24.0 untuk macOS (biner universal Intel + Apple Silicon)
  dipatok dengan SHA-256 dan ukuran, ditandatangani dengan kunci rilis agent seperti Linux. Query lintas-platform
  yang sama dengan Linux (inventaris, akun, jaringan, hardening/FileVault, rentang jaringan) dibuka untuk macOS;
  query khusus Linux (paket, firewall, pack resmi) tetap Linux saja. Belum terbukti di Mac nyata, jadi katalog
  menandai macOS "not proven yet".
- **Info host otomatis dari agent:** versi OS dilaporkan agent lewat heartbeat dan tampil di halaman aset
  (`agent_os_version`, tidak pernah menimpa isian manual). Platform dan arsitektur aset agent mengikuti agent
  dan tidak bisa diubah dari konsol; peran dan lokasi tetap isian manual dengan keterangan yang jelas.
- **Peran pengguna:** tiga peran tetap, Admin, Analyst, dan Viewer. Peran lama
  dipetakan otomatis ke peran terluas yang tidak menambah izin
  ([docs/ROLES.md](docs/ROLES.md)).
- **Jendela waktu scan dan stop darurat:** jendela mingguan per organisasi dengan
  zona waktu IANA, serta tombol *Stop all scans* yang bersifat tahan lama dan
  membatalkan scan yang sedang berjalan ([docs/SCAN-CONTROLS.md](docs/SCAN-CONTROLS.md)).
- **Perbandingan scan:** temuan ditandai Baru, Masih ada, atau Teratasi per
  pemeriksaan. Pemeriksaan yang gagal, dilewati, atau dihentikan tidak pernah
  dihitung sebagai teratasi ([docs/SCAN-COMPARISON.md](docs/SCAN-COMPARISON.md)).
- **Retensi data dan hapus organisasi:** masa simpan yang dapat diatur per jenis
  data, serta peninjauan sebelum organisasi dihapus ([docs/RETENTION.md](docs/RETENTION.md)).
- **Kontrol agent oleh Admin:** lihat versi dan status, *Update now*, rollout
  otomatis atau manual per agent, jeda, *Collect data now*, uninstall jarak jauh,
  dan diagnostik ([docs/AGENT-CONTROLS.md](docs/AGENT-CONTROLS.md)).
- **Notifikasi di dalam aplikasi:** lonceng dengan jumlah belum dibaca; ambang
  agent offline dapat diatur (bawaan 15 menit). Tanpa email atau webhook
  ([docs/NOTIFICATIONS.md](docs/NOTIFICATIONS.md)).
- **Bahasa English/Indonesia:** pilihan bahasa tersimpan per akun; report
  mengikuti bahasa yang dipilih saat scan dijalankan.
- **Report PDF dan JSON:** tata letak diadaptasi dari OpenAEV Community Edition
  dengan logo Orchis ([docs/REPORT-LAYOUT.md](docs/REPORT-LAYOUT.md),
  [docs/REPORT-SCHEMA.md](docs/REPORT-SCHEMA.md)).
- **Scan paralel** per organisasi dengan banyak target (asset atau rentang jaringan
  milik agent) dan alat Nmap/Nuclei.
- **Data endpoint (osquery)** berjalan di dalam agent, hanya setelah consent
  admin ([docs/ENDPOINT-CONSENT.md](docs/ENDPOINT-CONSENT.md)).
- **LICENSE:** Apache-2.0.
- **Instalasi server:** satu `docker-compose.yaml` di root, `.env.example` minimal, dan
  layanan `migrate` sekali jalan; panduan di `docs/DEPLOY.md` dan panduan pilot di
  [docs/PILOT.md](docs/PILOT.md).
- **`install.sh`:** pemasang agent yang disajikan server di `/install.sh`; memverifikasi
  SHA-256 dan tanda tangan Ed25519 (OpenSSL 3), tidak pernah mematikan verifikasi TLS
  (`--ca-fingerprint` atau `--pin`), dengan `--uninstall` dan `--purge`. Dialog Install agent
  di konsol kini menampilkan perintah `install.sh`.

#### Diubah

- Navigasi disederhanakan: Organisations, Simulations, Findings, Assets, Reports,
  Settings. Scenarios dan Actions dihapus dari UI; Chains, Single tests/Atomic,
  AI, dan MITRE tersembunyi di balik `ORCHIS_ADVANCED_FEATURES`.
- Findings dan Assets dapat difilter per organisasi (`?org=<uuid>`, difilter di server).
- Persetujuan koleksi osquery yang usang tidak lagi dipakai; perubahan pilihan
  koleksi memakai kontrol versi (HTTP 422/409 bila usang).
- Ekspor HTML/CSV dihapus; hanya PDF dan JSON.
- Aplikasi memulihkan diri bila stylesheet gagal dimuat sebelum antarmuka tampil.

#### Batasan yang diketahui

- Perintah pemasangan di konsol masih memakai perintah lama; konsol belum dialihkan ke
  `install.sh` (perubahan frontend menunggu).
- Mode `--pin` pada `install.sh` tidak mendukung update otomatis agent terhadap server
  self-signed; gunakan `--ca-fingerprint` untuk CA internal. `--ca-file` pada agent butuh
  rilis agent baru di jalur 1.3.x.
- osquery hanya terbukti di **Linux amd64**. macOS dan Windows tampil sebagai
  *belum didukung*.
- HTTPS awal memakai sertifikat **self-signed** (lihat [docs/OPERASI.md](docs/OPERASI.md));
  ACME untuk domain publik belum dibuktikan.
- Backup terenkripsi bersifat lokal; backup off-host belum dibuktikan.
- Integrasi AI hanya diuji dengan API palsu lokal.

## Agent

Versi agent mengikuti jalurnya sendiri dan diperbarui otomatis bila server
menyajikan rilis yang lebih baru, bertanda tangan, dan sesuai platform.

### [1.3.0]

- Pembaruan sendiri dibuktikan dari agent 1.2.0 asli, dan dari 1.1.0 yang
  direkonstruksi (bukan artefak rilis asli), ke 1.3.0 dengan kunci rilis yang sama.
  Identitas dan heartbeat tetap.
- Dibangun dan ditandatangani untuk enam target (Linux, macOS, Windows; amd64 dan arm64).
  Windows dan macOS belum dijalankan di perangkat nyata.
- Rilis lama 1.0.0–1.2.0 yang dibuat sebelum kunci rilis resmi memakai kunci
  berbeda; agent di instalasi lama perlu dipindahkan sekali secara manual.

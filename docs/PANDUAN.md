# Panduan pengguna Orchis

English: [USER-GUIDE.md](USER-GUIDE.md).

Orchis memvalidasi keamanan jaringan dan endpoint milik organisasi Anda. Alurnya:

**Organisasi → Agent atau Asset → Scan (Nmap/Nuclei) → Findings → Reports (PDF/JSON)**

Gunakan Orchis hanya pada sistem yang Anda berhak uji. Orchis menolak memindai
target yang belum diotorisasi.

## 1. Masuk dan bahasa

- Buka alamat server Orchis, lalu masuk dengan email dan password. Pada instalasi
  produksi lokal alamatnya `https://localhost:8443`. Sertifikat awal adalah
  **self-signed**; percayai sertifikatnya di peramban, jangan mematikan verifikasi TLS.
- Admin pertama dibuat saat server dimulai. Bila `ORCHIS_ADMIN_PASSWORD` kosong,
  password acak dicetak sekali di log server.
- Akun baru dibuat admin di **Settings → Users**. Pengguna menerima tautan sekali
  pakai (berlaku 30 menit) untuk memilih password.
- Pilih bahasa **English** atau **Indonesia** lewat pemilih bahasa di bagian atas
  aplikasi. Pilihan tersimpan pada akun Anda. Report memakai bahasa yang dipilih
  saat scan dijalankan, dan tidak berubah setelah dibekukan.

## 2. Peran

| Kemampuan | Admin | Analyst | Viewer |
|---|---|---|---|
| Membaca findings dan report, mengunduh report | Ya | Ya | Ya |
| Melihat organisasi, asset, agent; menjalankan scan pada target yang sudah diotorisasi | Ya | Ya | Tidak |
| Membuat report dan mencoba ulang PDF | Ya | Ya | Tidak |
| Mengotorisasi target, consent endpoint, rentang jaringan | Ya | Tidak | Tidak |
| Mengelola agent, pengguna, pengaturan, retensi, stop darurat | Ya | Tidak | Tidak |

Viewer hanya melihat menu Findings dan Reports. Rincian: [ROLES.md](ROLES.md).

## 3. Membuat organisasi dan memasang agent

1. Buka **Organisations** dan buat organisasi. Halaman detailnya memiliki tab
   Overview, Agents/Endpoints, Assets, Scans, Findings, dan Reports.
2. Di organisasi, pilih **Install agent**. Pilih OS (Linux, macOS, atau Windows)
   dan mode (layanan atau pengguna). Orchis menampilkan satu perintah sekali pakai
   yang mengunduh agent, memeriksa SHA-256, mendaftarkannya, lalu memasangnya.
   Jalankan perintah itu di mesin target (di Windows sebagai Administrator).
3. Agent muncul di **Agents/Endpoints** dan menampilkan status online.

> **Status:** perintah yang tampil di konsol saat ini masih perintah pemasangan
> lama. Konsol belum dialihkan ke `install.sh` (menunggu perubahan frontend).
> Server sudah menyajikan `install.sh` (lihat di bawah); sampai konsol dialihkan,
> argumennya diisi manual dari nilai yang ditampilkan konsol.

### Memasang agent dengan `install.sh` (Linux dan macOS)

Server menyajikan skrip pemasang di `https://<server>/install.sh`. Skrip tidak
berisi rahasia; token, hash, dan kunci rilis diberikan sebagai argumen dari konsol.

```bash
curl -fsSL https://<server>/install.sh -o install.sh   # server harus dipercaya CA sistem
sh install.sh --server https://<server> --token <TOKEN> \
  --sha256 <SHA256_BINER> --release-key <KUNCI_RILIS_BASE64> \
  --ca-fingerprint <SHA256_CA>          # atau: --pin sha256//<BASE64>
```

- Bila server memakai CA internal (belum dipercaya sistem), `curl` biasa untuk mengunduh skrip akan gagal. Unduh `ca.pem` dari server, cocokkan sidik jarinya lewat jalur lain, lalu pakai `curl --cacert ca.pem ...`; jangan memakai `-k`.
- Biner hanya dipasang bila **SHA-256 cocok** dan **tanda tangan Ed25519** valid
  untuk versi, platform, dan hash itu (butuh `curl` dan OpenSSL 3 atau lebih baru).
- Verifikasi TLS **tidak pernah dimatikan** (tidak ada `curl -k`). Sertifikat server
  dipercaya lewat salah satu dari:
  - `--ca-fingerprint`: skrip mengambil `/ca.pem` tanpa mempercayainya, menerimanya
    hanya bila SHA-256 DER-nya sama dengan sidik jari dari konsol, lalu menyimpannya
    sebagai `ca.pem` di folder agent. Cocok untuk CA internal pada instalasi Docker
    Compose. Sidik jarinya dapat dibandingkan dengan `python3 deploy/prod/ops.py ca-fingerprint`.
  - `--pin sha256//<BASE64>`: sertifikat server harus memiliki SPKI yang sama
    dengan pin (untuk server self-signed).
- `http://` hanya diterima untuk server loopback.
- Opsi lain: `--name` (beberapa agent di satu mesin), `--mode service|user`, `--dir`.

**Batasan jujur:** pada mode `--pin`, agent tetap hanya mempercayai CA sistem, jadi
**update otomatis agent ke server self-signed dengan pin tidak berjalan**. Untuk update
otomatis pada server dengan CA internal, gunakan `--ca-fingerprint`. Dukungan `--ca-file`
pada agent baru sampai ke mesin setelah rilis agent berikutnya di jalur 1.3.x.

**macOS:** agent berjalan, tetapi data endpoint osquery belum didukung di macOS
(baru Linux amd64). **Windows:** `install.sh` tidak berjalan di Windows; gunakan
perintah PowerShell dari konsol (osquery di Windows juga belum didukung).

### Menghapus agent

```bash
sh install.sh --uninstall            # hentikan layanan, hapus biner, simpan konfigurasi dan data
sh install.sh --uninstall --purge    # hapus juga konfigurasi dan data agent
```

Direktori berbahaya (`/`, `/opt`, `/usr`, `/home`, `/root`) ditolak. Cabut juga agent di
konsol bila tidak akan kembali.

Mendaftarkan agent **tidak** memberi izin scan. Beberapa agent pada satu mesin
memerlukan nama instance berbeda (`--name`).

## 4. Asset dan otorisasi

- Tambahkan asset satu per satu di tab **Assets**, atau tambahkan host yang
  ditemukan scan jaringan dengan satu klik.
- Admin mengisi alamat target lalu **mengotorisasi pengujian** per asset, atau
  per rentang jaringan milik agent. Tanpa otorisasi, asset tidak bisa di-scan.
- Rentang jaringan agent hanya bisa dipindai bila admin menyetujuinya, endpoint
  memiliki consent yang berlaku, dan agent melaporkan interface/rute dalam 24 jam terakhir.

## 5. Menjalankan scan

1. Tekan **Scan** (dari organisasi, atau di **Simulations**).
2. Pilih organisasi, lalu target: beberapa asset yang sudah diotorisasi dan/atau
   rentang dari satu agent.
3. Centang alat **Nmap** dan/atau **Nuclei**, lalu preset **Quick** atau **Standard**.
4. Tekan **Jalankan**. Target dan alat berjalan paralel. Kegagalan satu target
   tidak menghentikan yang lain.

Catatan penting:

- Scan dijalankan dari **server Orchis**, bukan dari agent. Rentang jaringan harus
  dapat dijangkau dari server. Rentang yang tidak terjangkau tercatat **gagal**;
  itu bukan bukti host aman.
- Batas bawaan: 256 alamat per scan dan 4 job aktif.
- Hasil tampil per target: host hidup, port terbuka, dan layanan (Nmap); temuan
  dengan severity, template, dan bukti (Nuclei).

### Jendela waktu dan stop darurat (Admin)

- Di organisasi, **Scan window and emergency stop** membatasi scan pada hari dan
  jam tertentu (zona waktu IANA). Tanpa jendela, scan boleh kapan saja.
- **Stop all scans** (juga ada di Settings untuk seluruh sistem) membatalkan scan
  yang menunggu dan yang berjalan, lalu menahan scan baru. Scan yang dihentikan
  ditandai dibatalkan, bukan aman. **Resume** hanya mengizinkan scan baru dan tidak
  melanjutkan scan yang terputus. Rincian: [SCAN-CONTROLS.md](SCAN-CONTROLS.md).

## 6. Findings dan perbandingan scan

- **Findings** dan **Assets** dapat difilter per organisasi.
- Setiap temuan dibandingkan dengan scan sebelumnya pada pemeriksaan yang sama:
  **Baru**, **Masih ada**, atau **Teratasi**. Bila pemeriksaan sebelumnya atau
  sekarang gagal, dilewati, atau dihentikan, hasilnya **Tidak dibandingkan**;
  temuan tidak pernah dianggap teratasi hanya karena scan gagal.
  Rincian: [SCAN-COMPARISON.md](SCAN-COMPARISON.md).

## 7. Reports

- Report dibuat per organisasi sebagai snapshot beku, lalu diunduh sebagai **PDF**
  atau **JSON** (skema: [REPORT-SCHEMA.md](REPORT-SCHEMA.md)).
- Bila PDF gagal dibuat, tekan **Retry PDF generation**; scan tidak diulang.
  JSON tetap tersedia.

## 8. Data endpoint (osquery)

osquery berjalan di dalam agent dan hanya mengumpulkan data setelah persetujuan.
Buka organisasi → **Agents/Endpoints** → pilih endpoint → **Configure endpoint
collection**. Query yang didukung OS endpoint diusulkan, tetapi kotak persetujuan
tetap kosong sampai admin menyetujuinya. Pengumpulan berjalan berkala (bawaan 15 menit).
Admin dapat menghapus dataset terakhir dari detail asset. Isi persetujuan:
[ENDPOINT-CONSENT.md](ENDPOINT-CONSENT.md).

**Batasan:** data endpoint baru terbukti di **Linux amd64**. macOS dan Windows
ditampilkan sebagai *belum didukung* dan belum diuji di perangkat nyata.

## 9. Notifikasi

Ikon lonceng menampilkan jumlah notifikasi belum dibaca; halaman **Notifications**
memuat riwayatnya. Peristiwa mencakup scan selesai atau dihentikan, PDF siap atau
gagal, agent offline atau dicabut, kegagalan update agent, dan permintaan
otorisasi atau consent yang menunggu admin. Tidak ada email atau webhook.
Admin dapat mengubah ambang agent offline di halaman Notifications (bawaan 15 menit).
Rincian: [NOTIFICATIONS.md](NOTIFICATIONS.md).

## 10. Mengelola agent (Admin)

Buka **Assets → Agents → Manage**. Admin dapat:

- melihat versi agent, versi osquery yang dilaporkan, OS, kontak terakhir, dan status update;
- memilih rollout **Automatic for all agents** atau **Manual per agent**, lalu **Update now**;
- **Pause/Resume** agent (hanya menghentikan tugas baru) dan **Collect data now**;
- mencabut agent atau menghapusnya dari jarak jauh, serta melihat diagnostik aman.

Agent hanya memasang rilis bertanda tangan yang valid dan tidak pernah turun versi.
Rincian: [AGENT-CONTROLS.md](AGENT-CONTROLS.md).

## 11. Retensi data dan menghapus organisasi (Admin)

- **Settings → Data retention** mengatur masa simpan findings (180 hari), hasil scan
  (90), report (180), dataset endpoint (30), dan audit (365). Data yang lebih tua
  dari batas dihapus otomatis. Perhatikan backup sebelum memperpendek masa simpan.
- **Review deletion** pada Overview organisasi menampilkan dampak penghapusan
  organisasi sebelum Anda mengonfirmasi. Rincian: [RETENTION.md](RETENTION.md).

## 12. Masalah umum

| Gejala | Penyebab dan tindakan |
|---|---|
| "This asset is not authorized for testing" | Admin belum mengotorisasi asset atau alamatnya. |
| Rentang jaringan gagal | Rentang tidak terjangkau dari server Orchis. Periksa routing dan firewall server. |
| Scan ditolak: jendela tertutup atau ditahan | Di luar jendela waktu, atau stop darurat aktif. Admin dapat mengubah jendela atau menekan Resume. |
| Agent tampil offline | Agent tidak mengirim heartbeat melewati ambang. Periksa layanan agent dan koneksi ke server. |
| PDF gagal | Tekan **Retry PDF generation**; periksa renderer dan MinIO. JSON tetap tersedia. |
| Tampilan tidak berubah setelah upgrade | Muat ulang paksa peramban (Ctrl+Shift+R). |

Prosedur operasi (upgrade, backup, restore) ada di [OPERASI.md](OPERASI.md).

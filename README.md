# Orchis

Orchis adalah platform validasi keamanan self-hosted dengan backend Go, frontend
React (UI v3), dan laporan PDF/JSON. Alur utama:

**Organisasi → Aset dan agent → Pindai → Temuan → Laporan → Perbaikan dan retest**

| Pilihan pindai | Yang dijalankan |
|---|---|
| Web pentest (orchis-redteam-agent) | Nmap dan Nuclei sebagai persiapan, lalu pengujian web melalui worker OpenCode |
| Installed software check (DefenseBot) | Koleksi osquery yang disetujui dari endpoint, lalu pencocokan kerentanan dan hardening |
| Full assessment | Memilih engine per aset: URL yang disetujui menjalankan orchis-redteam-agent, agent menjalankan DefenseBot, aset dengan keduanya menjalankan keduanya |

Aset dalam satu Full assessment boleh campuran. Pengujian memerlukan otorisasi,
prasyarat engine, dan jendela pindai yang berlaku. Kegagalan atau cakupan parsial
tercatat sebagai keterbatasan; hasil kosong tidak membuktikan aset aman.
Lihat [alur produk](docs/FLOW.md), [orchis-redteam-agent](deploy/hackbot/README.md), dan
[DefenseBot](docs/DEFENSEBOT.md).

## Mulai

Untuk deployment, siapkan Linux amd64, Docker Engine + Compose v2 + Buildx (BuildKit), Git, dan Python 3.12+.
Source engine sudah berada dalam repo ini; lihat [panduan build orchis-redteam-agent](deploy/hackbot/README.md#build-and-configure).

```bash
make hackbot-check                 # validasi source engine dalam repo, tanpa Docker
sudo make prod-init                # sekali untuk state baru; secret di /var/lib/orchis
sudo python3 deploy/prod/ops.py env > .env
sudo make prod-build               # server/web, renderer, scanner, engine, worker orchis-redteam-agent
sudo make prod-up
sudo make prod-probe
```

Jalankan build dengan akun yang memiliki akses Docker dan dapat membaca repo ini.
Untuk state yang sudah ada, gunakan `ORCHIS_STATE` yang sama dan ikuti
[prosedur upgrade](docs/OPERASI.md); jangan menjalankan init ulang.
Login pertama ada di `initial-admin.json` dalam direktori state privat.

Panduan [deployment](docs/DEPLOY.md) menjelaskan HTTPS, agent, dan konfigurasi
instalasi. Untuk bekerja pada source, lihat [pengembangan dan tes](docs/DEVELOPMENT.md).
Jalankan `make help` untuk melihat perintah yang tersedia.

## Struktur repo

| Lokasi | Isi |
|---|---|
| `cmd/` | Entry point server, agent, injector, dan utilitas rilis |
| `internal/` | Domain, layanan, API, worker, dan tes Go |
| `web/` | Aplikasi React dan tes frontend |
| `migrations/` | Migrasi PostgreSQL; dijalankan sebelum server versi baru |
| `deploy/` | Dockerfile, konfigurasi pengembangan/produksi, engine lock, dan CI |
| `e2e/` | Fixture, tes browser, dan pemeriksaan alur lengkap |
| `docs/` | Panduan produk, API, operasi, arsitektur, dan bukti audit |
| `docker-compose.yaml` | Stack produksi; konfigurasi non-rahasia dari `.env` |

`bin/`, `dist/`, `work/`, dan `node_modules/` adalah hasil lokal dan diabaikan Git.
Setiap komponen Node (`web/`, `e2e/`, `deploy/report/`) memiliki lockfile sendiri;
jalankan npm dengan `--prefix`, bukan dari root repo.
Checkout engine privat berada di luar repo Orchis. Secret, token model, dan state
produksi juga disimpan di luar checkout.

## Dokumentasi

- [Indeks dokumentasi](docs/README.md) — rute panduan menurut kebutuhan.
- [Alur produk dan kesiapan aset](docs/FLOW.md), [peran pengguna](docs/ROLES.md).
- [Audit terjadwal](docs/AUDIT-SCHEDULES.md).
- [API dan Swagger/OpenAPI](docs/API.md), [skema laporan](docs/REPORT-SCHEMA.md).
- [Operasi, upgrade, backup dan restore](docs/OPERASI.md).
- [Arsitektur](docs/ARSITEKTUR.md), [kompatibilitas fitur historis](docs/FEATURE-COMPATIBILITY.md).
- [Changelog](CHANGELOG.md), [lisensi](LICENSE), [komponen pihak ketiga](NOTICE).

## Batasan

DefenseBot mendukung koleksi endpoint Linux x86_64, macOS x86_64/arm64, dan
Windows x86_64/arm64. Artefak yang didukung dipatok di
[`catalog.json`](internal/osquery/catalog.json). Cakupan pemetaan berbeda per OS;
distribusi RPM belum didukung. Lihat [cakupan endpoint](docs/DEFENSEBOT.md).
orchis-redteam-agent memerlukan konfigurasi autentikasi model pada home worker persisten.
Nmap/Nuclei berjalan dari jaringan server, sehingga hasil keterjangkauan mengikuti
posisi scanner. Aset atau origin baru memerlukan persetujuan tersendiri.

Chains, Single tests/Atomic, AI, dan MITRE merupakan fitur lanjutan yang disembunyikan
secara bawaan (`ORCHIS_ADVANCED_FEATURES=true`). Data dan API historis tetap mengikuti
[batas kompatibilitas](docs/FEATURE-COMPATIBILITY.md).

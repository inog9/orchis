# Dokumentasi Orchis

Gunakan panduan berikut sebagai pintu masuk. Alur produk saat ini memakai UI v3,
Hackbot untuk sisi web, dan DefenseBot untuk endpoint.

| Kebutuhan | Panduan |
|---|---|
| Memahami perjalanan produk dan prasyarat engine | [FLOW](FLOW.md) |
| Menyiapkan server dan agent | [DEPLOY](DEPLOY.md) |
| Mengembangkan, membangun, dan menjalankan tes | [DEVELOPMENT](DEVELOPMENT.md) |
| Menyiapkan checkout dan worker Hackbot/OpenCode | [Build Hackbot](../deploy/hackbot/README.md) |
| Memahami sumber data dan cakupan endpoint | [DEFENSEBOT](DEFENSEBOT.md) |
| Mengatur audit berkala, zona waktu, dan riwayat jadwal | [AUDIT-SCHEDULES](AUDIT-SCHEDULES.md) |
| Mengoperasikan, upgrade, backup, dan restore | [OPERASI](OPERASI.md) |
| Menyiapkan pilot | [PILOT](PILOT.md) |
| Menelusuri API dan referensi offline | [API](API.md), [referensi API](api/README.md) |
| Memahami skema, isolasi tenant, dan keputusan desain | [ARSITEKTUR](ARSITEKTUR.md) |

## Izin dan siklus data

- [Peran pengguna](ROLES.md), [consent endpoint](ENDPOINT-CONSENT.md),
  [kontrol dan pembaruan agent](AGENT-CONTROLS.md).
- [Jendela pindai dan stop darurat](SCAN-CONTROLS.md),
  [perbandingan hasil](SCAN-COMPARISON.md), [retensi](RETENTION.md),
  [notifikasi](NOTIFICATIONS.md).
- [Format snapshot laporan](REPORT-SCHEMA.md) dan [layout laporan](REPORT-LAYOUT.md).

## Referensi dan riwayat

[PANDUAN](PANDUAN.md) dan [USER-GUIDE](USER-GUIDE.md) memuat rincian penggunaan
termasuk layar dan istilah dari iterasi sebelumnya. Untuk alur engine saat ini,
mulai dari FLOW dan panduan engine di atas.
[FEATURE-COMPATIBILITY](FEATURE-COMPATIBILITY.md) menjelaskan API dan fitur historis
beserta pemakai aktifnya. [ROADMAP-FLOW](ROADMAP-FLOW.md) menyimpan rencana perjalanan
produk; [audit UI](ui-audit/AUDIT.md) menyimpan bukti saat audit dilakukan.
Berkas historis dan bukti tetap disimpan pada lokasinya agar tautan lama berlaku.

Informasi lisensi komponen pihak ketiga ada di [licenses](licenses/README.md).

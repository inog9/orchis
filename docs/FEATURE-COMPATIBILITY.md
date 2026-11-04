# Kompatibilitas fitur lanjutan

Chains, Atomic, AI dan MITRE tetap tersedia setelah opt-in server. Flag advanced
mengatur tampilan; permission, scope, otorisasi target dan consent API tetap
berlaku. Menghilangkan halaman dari menu tidak membuat penyimpanan atau API
bersamanya dapat dihapus.

## Batas penghapusan bersyarat

Halaman Scenarios dan Actions, route frontend dan entri navigasinya sudah dihapus
pada commit567d6ea. ScenariosPage.tsx dan ActionsPage.tsx tidak lagi ada.
NewChainDialog.tsx mempertahankan pembuatan chain; ScenarioDetailPage.tsx kini
melayani detail/setup Chains, bukan halaman Scenarios lama. Nama tipe/API scenario
tetap menjadi kontrak kompatibilitas. Route lama tidak dibuka lagi oleh flag.

| Komponen yang dipertahankan | Pemakai yang masih ada |
|---|---|
| internal/scenario dan /api/v1/scenarios | ChainsPage, NewChainDialog, ChainWorkspacePage, ScenarioDetailPage; graph, targets, launch dan kontrol versi |
| internal/contract, katalog aksi/preset | Form deklaratif Chains/Atomic dan validasi server/injector; empat adapter lama tetap diperlukan |
| internal/simulation.Engine dan simulations/executions | Chain, Atomic, job Scan dan runtime injector/agent; hasil dan pembatalan run |
| assessments dan assessment_targets | Scan baru menyimpan is_scan/scan_options pada assessment; laporan, snapshot, demo, histori dan target beku juga memakainya |
| GET assessment/list dan cancel | AssessmentPage, tautan laporan historis, overview/setup serta pembatalan run lama |
| POST organisasi/assessment lama | Kontrak API profil quick/full/web dan parameter tervalidasi masih didokumentasikan; full/web/custom tidak seluruhnya terwakili preset Scan quick/standard. Masih menjadi jalur kompatibilitas serta fixture integrasi laporan/AI |
| AI dan MITRE | Jalur advanced, data/snapshot dan izin terkait; tidak ada panggilan AI nyata yang diperlukan untuk mempertahankannya |

API peluncuran assessment lama bukan penghapusan aman yang terbukti saat ini:
menghapusnya akan menghapus kemampuan profil API yang masih didokumentasikan,
serta mengharuskan pemindahan fixture/kontrak cakupan ke Scan baru. Tidak ada
klaim bahwa API ini sudah dihapus atau dipisahkan dari engine bersama.
Tidak ada endpoint baru atau pelonggaran izin sebagai pengganti.

Keputusan pengguna: pertahankan inti bersama dan hapus hanya bagian yang aman.
Karena itu, penghapusan UI yang sudah dilakukan dipertahankan; tidak ada
penghapusan backend/tabel tambahan berdasarkan asumsi halaman sudah tersembunyi.
Tidak ada tabel/kolom sisa yang terbukti tak dibutuhkan, sehingga tidak dibuat
migrasi penghapusan data. Skema29 tetap berlaku. Uji upgrade destruktif tidak
berlaku karena tidak ada DDL baru pada keputusan ini; gerbang keseluruhan tetap
wajib menguji skema yang dipakai aplikasi.

## Bukti pemeriksaan

Pemetaan import frontend tidak menemukan berkas TS/TSX aplikasi yatim. Pemeriksaan
route dan navigasi tidak menemukan pemulihan halaman Scenarios/Actions. App.test
menguji flag advanced mati/aktif serta route scenario yang tetap hilang; tes
Chains/Atomic dan193 tes UI pada checkpoint9c1d83d lulus. Tidak ada perubahan
runtime pada pencatatan keputusan ini, sehingga tes yang sama tidak diulang hanya
untuk perubahan dokumentasi. Gerbang tahap penuh dicatat terpisah.

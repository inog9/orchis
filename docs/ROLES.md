# Peran pengguna

Orchis menyediakan tiga peran tetap per tenant. Endpoint API tetap memeriksa izin;
menu yang disembunyikan bukan pengganti otorisasi. Fitur lanjutan tetap mengikuti
flag server yang sudah ada.

| Kemampuan | Admin | Analyst | Viewer |
|---|---|---|---|
| Membaca findings dan snapshot report, mengekspor report | Ya | Ya | Ya |
| Inventaris aset, organisasi, agent, hasil scan langsung | Ya | Ya | Tidak |
| Membuat/mengubah inventaris aset dan menjalankan target yang sudah diotorisasi | Ya | Ya | Tidak |
| Membuat report dan mencoba ulang PDF | Ya | Ya | Tidak |
| Memberi/mencabut otorisasi target, mengubah alamat target yang sudah diotorisasi | Ya | Tidak | Tidak |
| Consent, koleksi endpoint, rentang jaringan yang boleh dipindai | Ya | Tidak | Tidak |
| Menghapus data endpoint atau aset, mengelola agent/integrasi/pengguna/pengaturan | Ya | Tidak | Tidak |
| Profil, password, token API pribadi, logout | Ya | Ya | Ya |

Viewer mempunyai tepat `finding.read` dan `report.read`. Halaman awalnya Findings;
menu utamanya Findings dan Reports. Snapshot report dapat dibaca tanpa akses ke
inventaris organisasi/aset hidup: report masih memerlukan `finding.read` selain
`report.read`, memakai transaksi tenant/RLS, serta redaksi snapshot yang sudah ada.
Tautan inventaris/eksekusi dan pilihan organisasi hidup disembunyikan bagi Viewer.
Nama platform dan versi tetap tersedia lewat metadata Settings untuk header semua
pengguna; detail kesehatan infrastruktur tetap hanya `settings.manage`.

Analyst tidak memiliki `asset.authorize`, `agent.manage`, `settings.manage`,
`user.manage`, atau `ai.manage`. Pendaftaran agent tidak memberi otorisasi scan.
Token API pribadi selalu mewarisi izin peran terkini, bukan salinan izin saat token
dibuat. Viewer tidak mendapat akses ke statistik Home maupun hitungan demo global.

## Upgrade dari skema 29

Migrasi 30 membekukan izin efektif lama sebelum mengubah definisi peran bawaan.
Pemetaan memilih peran terluas yang seluruh izinnya merupakan subset izin efektif
lama. Urutan pilihan adalah Admin, Analyst, Viewer. Izin yang tidak dikenali dibuang;
izin baru tidak diberikan otomatis.

| Izin efektif lama | Hasil |
|---|---|
| Mencakup semua izin Admin baru | Admin |
| Tidak mencakup Admin, mencakup semua izin Analyst baru | Analyst |
| Mencakup findings dan akses report efektif lama | Viewer |
| Lebih sempit atau tidak dapat direpresentasikan | Viewer **disabled**, perlu tinjauan Admin |

Sebelum migrasi, akses report juga memerlukan `organization.read`,
`assessment.read`, `asset.read`, dan `finding.read`. Memiliki dua string
`report.read`/`finding.read` saja tidak berarti report dahulu dapat dibaca. Migrasi
menghapus `report.read` dari izin efektif pembanding bila prasyarat lama tidak
lengkap. Ini mencegah perluasan akses melalui perubahan dependensi report.
Viewer bawaan lama dipersempit ke dua izin baru. Semua pengguna, termasuk akun
service, demo, disabled, dan soft-deleted, dipetakan sebelum peran kustom dihapus.

Akun yang perlu tinjauan tetap disabled, sesi dan token API lamanya dicabut.
Settings → Users menampilkan *Review required* dan nama peran lama. Admin memilih
satu dari tiga peran, membaca izin, lalu mencentang persetujuan eksplisit sebelum
mengaktifkannya. Konflik versi tetap menolak perubahan bersamaan. Aktivasi yang
menambah izin adalah keputusan Admin yang diaudit, bukan hasil otomatis migrasi.

Database menolak kunci peran selain tiga tersebut, izin Viewer di luar dua izin
baca, dan akun `role_review_required` yang active. Audit `user.role_migrated`
menyimpan peran lama/baru dan status tinjauan per pengguna, tanpa kredensial.

## Pemulihan dan rollback

Cadangkan database sebelum upgrade. Pemetaan ini tidak memiliki invers yang aman.
Rollback harus memulihkan database pra-upgrade bersama biner lama; `goose down`
migrasi 30 sengaja menolak. Biner lama yang mencoba menyinkronkan ulang Viewer ke
izin luas akan ditolak constraint database baru.

Bootstrap hanya mengakui Admin manusia nyata yang active. Bila pengguna manusia
nyata sudah ada tetapi tidak ada Admin active, server memberi pesan pemulihan dan
tidak membuat Admin pengganti secara diam-diam. Operator harus memulihkan akun
Admin yang diketahui dari cadangan, atau melakukan pemulihan database terkontrol:
hentikan server, simpan cadangan, periksa tenant/ID akun/peran/password yang sudah
diketahui, tetapkan peran Admin dan status active pada akun yang disetujui, hapus
flag tinjauan dengan audit pemulihan, lalu mulai server dan verifikasi login. Jangan
mengaktifkan akun demo atau memakai alamat akun baru sebagai jalan pintas.

## Verifikasi

`internal/server/testdata/role-api-policy.csv` adalah kebijakan endpoint tersendiri,
bukan hasil menyalin guard runtime. Matriks menguji ketiga peran pada 147 endpoint,
termasuk lima keluarga registrasi dinamis. Penambahan endpoint literal yang belum
masuk matriks menggagalkan tes. Kanal agent/injector/AI worker tetap memerlukan
kredensial mesin; cookie Admin tidak menggantikannya. Pendaftaran menggunakan token
sekali pakai, bukan role pengguna.

Uji upgrade membuat database skema 29 sungguhan, memuat definisi peran lama yang
dibekukan dalam fixture, lalu menjalankan migrasi 30 secara atomik. Cakupannya dua
tenant, peran kustom sempit/luas, dependensi report lama, pencabutan kredensial,
audit, constraint, persetujuan aktivasi, dan bootstrap pemulihan.

`e2e/flow19.mjs` menguji akun nyata, navigasi Viewer, penolakan otorisasi target,
tiga pilihan peran, serta axe/overflow pada 1280/834/390. `flow9`/`flow10` tetap
menguji fitur organisasi dan AI lokal untuk Analyst, serta penolakan Viewer.
Jalankan `python3 e2e/mutation-three-roles.py` untuk mutasi terisolasi; nama mutan
opsional dapat diberikan untuk satu subset. Setiap mutan harus lolos kompilasi dan
kemudian gagal pada assertion terkait; sumber aplikasi tidak dimutasi.

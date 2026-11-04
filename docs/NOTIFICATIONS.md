# Notifikasi di dalam aplikasi

Lonceng menampilkan jumlah belum dibaca dan delapan peristiwa terbaru. Halaman
Notifications menampilkan sampai 100 peristiwa terbaru. Keduanya diperbarui setiap
10 detik ketika aplikasi aktif; tidak ada email, webhook, atau layanan AI.

Peristiwa baru meliputi scan selesai, selesai dengan pemeriksaan gagal/dilewati,
scan dihentikan, PDF siap, kegagalan PDF, agent offline, kredensial dicabut,
kegagalan pembaruan, serta otorisasi aset atau pembaruan consent yang menunggu admin.
Scan parsial tetap menampilkan kegagalannya. Scan dihentikan tidak dianggap aman.
PDF gagal dapat dicoba ulang tanpa scan baru. Generasi render membedakan retry
manual dari pengulangan worker pada percobaan yang sama.

Admin dapat mengubah ambang offline di Notifications: bawaan **15 menit**, batas
2 menit sampai 7 hari. Penyimpanan memakai versi optimistis; konflik memerlukan
reload. Indikator koneksi tetap berubah setelah 90 detik. Sweep berjalan setiap
15 detik. Notifikasi muncul setelah ambang benar-benar terlewati, sekali per
periode tanpa kontak. Heartbeat setelah offline membuka episode baru. Jeda tugas
tidak menghentikan heartbeat. Dua kode kegagalan update yang berurutan termasuk
satu kejadian; setelah pemulihan, kegagalan baru memberi notifikasi baru.

Otorisasi yang menunggu admin dipantau untuk aset organisasi dengan agent yang
belum dicabut. Perubahan versi aset yang masih belum diotorisasi dapat meminta
tinjauan baru. Consent hanya diminta diperbarui untuk pilihan koleksi yang sudah
aktif dan memakai dokumen lama/tidak lengkap. Tidak ada koleksi yang otomatis
boleh dijalankan karena notifikasi. Otorisasi, consent dan batas scan tetap
memakai penjaga masing-masing. Daftar notifikasi adalah riwayat kejadian;
pembacaan notifikasi tidak memberi izin apa pun.

## Izin dan isolasi

Peran memberikan akses organisasi pada tingkat tenant; aplikasi belum memiliki
ACL anggota pengguna per organisasi. Filter organisasi membatasi daftar dan
jumlah belum dibaca pada organisasi yang dipilih. Setiap daftar, hitungan,
baca satu, baca semua, dan dismiss memeriksa izin audiens dan keberadaan/izin
objek terkini. Asosiasi organisasi yang berubah membuat tautan lama disembunyikan.
Objek yang terhapus tidak ditampilkan. RLS dan FK organisasi selalu memakai tenant.

Viewer dapat membuka inbox berdasarkan izin membaca temuan/laporan yang sudah
dimilikinya, tanpa tambahan permission peran. Viewer tidak mendapat rincian
scan, aset, agent, pengaturan, atau permintaan persetujuan admin. Laporan beku
memerlukan report.read dan finding.read; membaca notifikasinya mengikuti aturan
itu. Peran kustom tetap memerlukan seluruh izin objek terkait. Status baca dan
dismiss tersimpan per pengguna; baca semua berlaku untuk semua peristiwa yang
terlihat bagi pengguna tersebut, termasuk di luar filter organisasi aktif.

Tautan scan dan laporan membuka ID yang tepat. Tautan agent memilih satu instance,
bukan seluruh daftar; aksi admin masih diperiksa ulang oleh API. Nama objek atau
isi keluaran scanner tidak menentukan rute. Pesan baru memakai kode peristiwa dan
kamus EN/ID; penyambungan pilihan bahasa aplikasi dilakukan pada fitur bahasa.
Diagnostik baru hanya memakai kategori tetap, tanpa log mentah, alamat berkas,
token, konfigurasi privat, atau keluaran koleksi endpoint.

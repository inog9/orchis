# Retensi dan penghapusan organisasi

Hanya Admin dapat mengubah **Settings → Data retention** atau membuka **Review
deletion** pada Overview organisasi. Perubahan retensi berlaku pada data yang
sudah ada. Tinjau kebijakan dan backup sebelum memperpendek masa simpannya.

| Data | Bawaan | Awal perhitungan |
|---|---:|---|
| Findings | 180 hari | pengamatan terakhir pada server |
| Hasil Scan, Atomic, dan Chain terminal | 90 hari | akhir eksekusi; waktu pembuatan sebagai fallback data lama |
| Laporan PDF/JSON | 180 hari | pembuatan snapshot |
| Dataset endpoint terbaru | 30 hari | pengumpulan sukses terakhir; percobaan pertama bila belum pernah sukses |
| Audit | 365 hari | waktu kejadian server |

Satu hari berarti 24 jam UTC. Batasnya 1–3.650 hari, kecuali audit minimum 90
hari. Data pada tepat batas umur tetap disimpan; yang lebih tua diproses.
Kegagalan koleksi endpoint tidak memperpanjang umur dataset sukses sebelumnya.
Tidak ada riwayat koleksi endpoint; pengumpulan baru tetap memerlukan otorisasi
dan consent versi yang berlaku. Perubahan dokumen consent retensi memerlukan
persetujuan ulang, tanpa mengaktifkan query secara otomatis.

Worker memulai siklus setiap jam dan setelah perubahan kebijakan. Setiap
transaksi memproses batch terbatas; backlog penuh diteruskan pada tick berikutnya.
Intent objek eksternal dicoba setiap lima detik. Restart melanjutkan pekerjaan
tersimpan. Error ditampilkan sebagai kode operasional aman dan jumlah pending;
kegagalan database/storage tidak boleh dilaporkan sebagai penghapusan selesai.
Pencatatan status worker tidak mengubah versi konfigurasi: kebijakan bawaan yang
belum disimpan tetap versi 0, dan penyimpanan Admin pertama memakai versi 1.
Ini mencegah konflik palsu ketika worker mulai bersamaan dengan halaman Settings.
Demo tidak mengikuti masa simpan otomatis untuk findings, hasil scan, laporan,
dan dataset endpoint: gunakan Purge demo data.

Snapshot laporan memiliki umur terpisah. Saat hasil scan kedaluwarsa, job,
execution, log dan keluaran mentah dihapus, sedangkan metadata assessment minimal
bertahan selama snapshot membutuhkannya. UI menandai hasil rinci kedaluwarsa.
Setelah laporan kedaluwarsa, metadata yang tidak diperlukan ikut dibuang. Kunci
idempotensi peluncuran ikut berakhir bersama metadata assessment; jangan memakai
ulang kunci lama sebagai cara mengulang scan. Scan aktif dan simulasi yang masih
dipakai assessment lain ditahan dari penghapusan. Konfigurasi Atomic/Chain yang
dapat dipakai ulang tidak ikut kedaluwarsa hanya karena hasilnya berakhir.

Finding yang dihapus lalu ditemukan kembali mendapat identitas baru. Organisasi
menaikkan generasi identitas perbandingan saat expiry. Perbandingan lintas generasi
menjadi **Tidak dapat dibandingkan**, bukan Teratasi atau Baru yang menyesatkan.
Snapshot berikutnya menyediakan baseline baru. Snapshot historis tetap beku dan
boleh masih memuat ringkasan finding sampai masa simpan laporannya sendiri habis.
Ini tidak mengubah status triase manual.

## Hapus organisasi

1. Buka organisasi → Overview → Review deletion.
2. Periksa jumlah asset, agent, findings, report, dataset endpoint, scan, dan
   execution. Ketik nama organisasi persis, termasuk huruf besar/kecil.
3. Jika jumlah atau versi berubah, refresh summary dan periksa kembali.
4. Setelah konfirmasi, agent dicabut dan data database organisasi dihapus dalam
   satu transaksi. Injector yang masih bekerja menerima execution yang sudah
   tidak tersedia dan menghentikan grup proses scanner pada polling berikutnya.
5. Settings menampilkan operasi penghapusan dan pending eksternal sampai worker
   menyelesaikannya. URL status dapat dimuat ulang. Organisasi lain tidak diubah.

PDF yang sedang dirender menahan penghapusan sampai selesai atau dipulihkan oleh
worker laporan. Data yang menjadi referensi riwayat organisasi lain juga menahan
penghapusan, termasuk snapshot beku yang masih menyebut asset tersebut setelah
hasil mentahnya kedaluwarsa; selesaikan dependensi tersebut dahulu. Target lain dalam Chain
bersama tetap dipertahankan. Demo dihapus melalui alur purge tersendiri.

Cleanup hanya memakai UUID dan jenis objek yang ditetapkan server. Key MinIO
harus persis key laporan/execution milik tenant; key tak dikenal menolak
transaksi. Elasticsearch menghapus hanya ID finding dalam tenant yang sesuai,
bukan index atau prefiks. Intent tetap ada sampai storage mengakui keberhasilan;
pengulangan sesudah commit gagal aman karena penghapusan idempoten.

Audit penghapusan memuat aktor, waktu, ID organisasi dan jumlah data, tanpa isi
finding/dataset/kredensial. Metadata status menyimpan nama organisasi yang sudah
diredaksi, jumlah, dan waktu sampai batas audit setelah cleanup selesai. Operasi
yang masih pending dipertahankan agar tidak kehilangan intent.

**Backup lama tetap mengandung data sampai backup tersebut kedaluwarsa.**
Menghapus organisasi atau mempersingkat retensi aplikasi tidak menghapus arsip
backup. Operator menerapkan siklus kedaluwarsa backup secara terpisah. Restore
backup dapat mengembalikan data dan kredensial dari saat backup; terapkan kembali
keputusan penghapusan/pencabutan sebelum membuka hasil restore untuk pengguna.

## API admin

- `GET/PUT /api/v1/retention`: kebijakan dan CAS `version`.
- `GET /api/v1/organizations/{id}/deletion-preview`: jumlah dan blocker.
- `DELETE /api/v1/organizations/{id}`: nama, versi, dan jumlah dari preview.
- `GET /api/v1/data-deletions/{id}`: status database dan cleanup eksternal.

Semua route memerlukan sesi/izin Admin dan penulisan memakai CSRF yang sama
dengan API lain. Penghapusan juga memeriksa izin organisasi, asset, agent,
finding, dan report. RLS membatasi seluruh kebijakan, status, dan intent per tenant.

# Perbandingan scan

Pemeriksaan awal A-2.9: alur Scan baru sudah memanggil `Compare` di
`FreezeScanSnapshot`. Hasilnya tersimpan sebagai teks dalam snapshot, dengan
baseline organisasi/profil sebelumnya. Pembanding lama mensyaratkan semua metode
satu aset sukses, sensitif terhadap urutan metode, dan belum menyediakan baris
Baru/Teratasi/Masih ada, hitungan severity, atau filter pada hasil/Findings.

Implementasi berikut mempertahankan snapshot lama dan menambah data terstruktur.
Baseline dipilih per aset, target, tool/versi definisi, parameter, dan profil yang
sama. Hasil scan gagal/dilewati/dihentikan tidak membuktikan temuan teratasi.
Perubahan temuan dipisahkan dari status triase manual pada Findings.

## Aturan pembandingan

- Identitas observasi memakai ID finding per aset yang sudah dideduplikasi ingest.
  Parameter dinormalisasi memakai versi katalog yang dibekukan. Perubahan target,
  versi, tool, parameter atau profil tidak menjadi bukti remediasi.
- Baseline adalah laporan beku run lebih awal dalam tenant/organisasi dan jenis
  alur yang sama (Scan dengan Scan, assessment kompatibilitas dengan assessment).
  Pembaca berjalan dari run terbaru dan memilih baseline per pemeriksaan, bukan
  melewati kegagalan terbaru untuk mencari sukses yang lebih lama.
- Observasi positif pada pemeriksaan pertama adalah **New / Baru**. Observasi
  yang ditemukan lagi pada pemeriksaan sebanding adalah **Still present / Masih
  ada**. Observasi lama yang tidak ditemukan lagi hanya **Resolved / Teratasi**
  setelah semua pemeriksaan asalnya yang relevan selesai dengan sukses.
- Status **Not compared / Tidak dibandingkan** menjelaskan kegagalan, penghentian,
  scope/profil berubah, atau asal observasi yang tidak dapat dipastikan. Satu tool
  yang gagal tidak menghapus bukti dari tool lain yang benar-benar selesai.
  Alamat yang disunting atau dipotong tidak dianggap identik dengan alamat asli.
- Snapshot baru menyimpan seluruh pemeriksaan penghasil sebuah finding melalui
  `coverage`. Snapshot historis tanpa informasi itu hanya dapat dibandingkan bila
  tepat satu pemeriksaan kompatibel menghasilkan jenis finding tersebut.
- Penggabungan observasi berulang menyimpan seluruh sumber dan severity tertinggi.
  Urutan job/metode/observasi tidak mengubah keputusan. Input snapshot tidak diubah.
  Data perbandingan tidak mengubah status triase manual finding.

## Antarmuka dan format

`comparison_details` bersifat aditif pada JSON snapshot dan hasil Scan terminal.
Bentuk versi 1 memuat `totals`, `by_severity`, `rows`, dan `checks`. Baris hanya
berisi ringkasan temuan yang disunting; evidence dan keluaran mentah tidak disalin.
`checks` merekam alasan serta ID/waktu baseline agar keputusan dapat ditelusuri.
Kunci JSON dan kode keputusan tetap Inggris; terjemahan UI ditangani pada A-2.6.

Hasil Scan dan detail Reports menampilkan tabel jumlah per severity serta filter
New/Still present/Resolved/Not compared. Findings organisasi menyediakan pilihan
**Latest completed scan changes** dari snapshot terakhir yang selesai, termasuk
observasi Teratasi yang tidak lagi muncul pada scan itu. Pencarian, aset, jenis,
dan severity tetap membatasi baris yang terlihat. Pilihan tampilan ini terpisah
dari daftar seluruh findings. `GET /api/v1/reports?scan_only=true` memfilter laporan
Scan; daftar tetap ringan dan detail diambil melalui endpoint report berizin yang
sudah ada. Viewer tetap membutuhkan izin finding dan report.

HTML/PDF memakai data snapshot yang sama dengan JSON. Laporan lama tetap tersedia
sebagai snapshot historis dengan pesan bahwa perbandingan terstruktur belum ada.
Jumlah perubahan mencakup observasi discovery; risiko organisasi tetap hanya
berdasarkan severity kerentanan yang teramati. Tabel severity dapat digeser
horizontal pada layar kecil.

## Verifikasi komponen

Tes tabel meliputi scan parsial, gagal/dihentikan/dilewati, alamat/profil/parameter
berubah, sumber ambigu/lintas aset, duplikasi, beberapa penghasil, baseline gagal
terbaru, determinisme dan snapshot tidak berubah. Tes database nyata memeriksa
batas tenant/organisasi, profil dan penyimpanan keputusan saat freeze.

`flow21.mjs` memakai dua endpoint loopback dan Nuclei sungguhan: Baru → Masih ada
→ satu Teratasi + satu tidak terjangkau, kemudian pembatalan. JSON dan PDF nyata
diperiksa; hasil/Findings organisasi/Reports diperiksa dengan axe serta screenshot
1280/834/390. Uji mutasi `mutation-scan-comparison.py` bekerja pada salinan terpisah,
mewajibkan setiap mutan berhasil build, lalu memulihkan dan menguji baseline.

Retensi berikutnya harus mempertahankan ketepatan identitas/baseline ini: menghapus
finding lalu membentuk ID baru tidak boleh dipakai untuk menyatakan identitas lama
teratasi tanpa pemeriksaan cakupan yang sah. Snapshot lama tetap tunduk pada
kebijakan retensinya sendiri.

Generator demo baru juga menyimpan sumber dan perbandingan terstruktur. Demo
historis yang sudah terpasang tetap merupakan snapshot lama; tidak ditulis ulang
secara diam-diam. Pengujian demo memeriksa jumlah aset dan jumlah pemeriksaan
sebanding secara terpisah, karena keduanya bukan satuan yang sama.

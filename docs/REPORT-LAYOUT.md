# Halaman cetak laporan organisasi

Laporan memakai sampul A4, daftar isi bertaut, ringkasan cakupan, distribusi
severity, daftar aset/temuan, endpoint posture bila tersedia, remediasi,
perbandingan cakupan dan lampiran metodologi. Logo adalah Orchis. Tata letak
mengadaptasi OpenAEV Community Edition; atribusi dan lisensi ada pada NOTICE.

Halaman HTML internal dan PDF memakai template yang sama. Font Geologica dan
IBM Plex Sans disisipkan lokal dari aset beku, sehingga renderer tidak perlu
mengakses server font. Daftar isi tetap menjadi tautan internal di PDF.
Footer memuat nomor halaman serta penanda snapshot atau DEMO/SYNTHETIC.

Kartu ringkasan menghitung aset diuji/dilewati/tidak lengkap dari snapshot beku;
selesainya sebuah scan tidak membuktikan aset aman. Risiko mengikuti severity
kerentanan tertinggi, bukan skor prevention/detection rekaan. Data endpoint
serta perbedaan port luar/dalam tidak otomatis membuktikan kerentanan.

Lampiran metodologi dimulai pada halaman tersendiri. Metode suatu aset
berkelompok bila muat pada halaman; daftar temuan panjang tetap dapat mengalir.
Bukti memakai daftar field yang diizinkan, redaksi dan escaping yang sama dengan
sebelumnya. Keluaran mentah tidak ditambahkan oleh perubahan tata letak.

Pemeriksaan QA dapat memakai e2e/report-preview, e2e/check-report-pdf.py dan
raster Poppler. Selalu tinjau setiap halaman, termasuk laporan kecil/besar/demo;
pemeriksaan teks saja tidak membuktikan tidak ada tumpang tindih. Data demo harus
berasal dari generator aplikasi atau diberi label fixture sintetis yang jelas.

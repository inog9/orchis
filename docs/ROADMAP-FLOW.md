# Roadmap alur produk — spesifikasi saja

Daftar ini adalah perbaikan alur yang diminta pemilik produk tetapi **belum dibangun**. Yang sudah
dibangun tercantum di CHANGELOG dan docs/FLOW.md.

1. **Re-match terjadwal data software** — menyimpan dataset koleksi dan menjalankan ulang pencocokan
   DefenseBot berkala; notifikasi per organisasi (opt-in) saat CVE known-exploited baru mengenai
   software tersimpan. Data yang dibutuhkan sudah tersimpan (`endpoint_datasets`).
2. **Owner/assignee dan tenggat SLA per finding** — kolom assignee + due date, tampil di daftar
   "perbaiki lebih dulu", terhubung dengan triage (risiko diterima menunda SLA sampai kedaluwarsa).
3. **Ekspor tiket** — kirim finding ke Jira/GitHub issue dengan bukti dan nilai; peta status tiket
   kembali ke triage (`fixed-pending-verification` saat tiket ditutup).
4. **Alur verifikasi remediasi** — setelah retest "still present", temuan kembali `open` otomatis dan
   jejak audit mencatat siapa yang mengklaim perbaikan.
5. **Pemetaan kepatuhan (CIS)** — celah hardening DefenseBot dipetakan ke kontrol CIS untuk laporan
   kepatuhan per organisasi.
6. **Tren skor risiko per organisasi** — skor deterministik dari snapshot laporan; grafik tren di
   ringkasan organisasi.
7. **Peran auditor baca-saja** — peran kustom dengan `finding.read` + `report.read` saja; kode sudah
   memeriksa permission, jadi cukup peran baru.
8. **Dukungan Windows dan distribusi RPM** — kumpulan query osquery + ekosistem OSV per distro;
   `CollectionSupported` dan provenance ekosistem adalah titik integrasinya.
9. **Ringkasan AI atas temuan terstruktur** — lewat `ai-proxy`, dengan aturan konsistensi pelaporan
   dari `PROMPT-DEFENSEBOT-P6.md` (teks advisory tetap Inggris dan di-escape; angka tidak boleh
   dikarang oleh model).

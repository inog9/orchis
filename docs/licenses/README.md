# Provenans lisensi komponen beku

Inventaris A-1.2, 2 November 2026. Teks lisensi disalin utuh; lisensi proyek
Orchis sendiri masih menunggu K2. Komponen mempertahankan ketentuannya sendiri.

| Komponen | Versi | Teks lokal | Sumber |
|---|---|---|---|
| osquery | 5.24.0 | osquery.LICENSE, osquery-Apache-2.0.LICENSE | https://github.com/osquery/osquery/tree/5.24.0 |
| Nmap Debian | 7.93+dfsg1-1 | nmap-debian.copyright, GPL-2 | /usr/share/doc/nmap/copyright dan /usr/share/common-licenses/GPL-2 dari orchis-injector:dev; dpkg-query mengonfirmasi versi sama dengan pin produksi |
| Nuclei | 3.11.1 | nuclei.LICENSE | https://github.com/projectdiscovery/nuclei/blob/v3.11.1/LICENSE.md |
| nuclei-templates | 10.5.1 | nuclei-templates.LICENSE | https://github.com/projectdiscovery/nuclei-templates/blob/v10.5.1/LICENSE.md |
| MITRE Enterprise | 20.0 | mitre.LICENSE | https://github.com/mitre-attack/attack-stix-data/blob/master/LICENSE.txt; pin bundel deploy/mitre/manifest.json |

osquery menawarkan pilihan Apache-2.0 atau GPL-2.0-only; integrasi Orchis memilih
Apache-2.0. Nmap Debian memuat syarat dan pengecualian khusus; jangan menyederhanakan
labelnya menjadi Apache atau GPL-2 biasa. File copyright Debian mencakup komponen
bundel serta teks/rujukan lisensinya. Ini inventaris, bukan kesimpulan legal
mengenai kompatibilitas atau pemenuhan seluruh kewajiban distribusi.

Server Dockerfile menyalin seluruh direktori ini dan NOTICE ke /licenses.
Image scanner mempertahankan copyright paket Debian dan lisensi template.
Packaging rilis agent/osquery dan ketersediaan source distribusi diperiksa lagi
pada tahap rilis/repo bersih; belum dinyatakan lengkap oleh inventaris ini.

## Tata letak laporan dan font

Tata letak laporan mengadaptasi modul OpenAEV CE yang tercantum pada NOTICE,
dari snapshot sumber ee696b2b24c5de09cd1c7df035c2fd1ad9df5148. Root LICENSE
menetapkan berkas tanpa header khusus sebagai CE Apache-2.0; setiap berkas
referensi diperiksa sebelum adaptasi. Teks dan copyright Apache disalin ke
openaev-Apache-2.0.LICENSE. Tidak ada modul berheader Enterprise yang digunakan.

Geologica/IBM Plex Sans memakai font Latin WOFF2 tanpa modifikasi dari Google
Fonts. Teks OFL dan copyright ada pada geologica.OFL.txt/ibmplexsans.OFL.txt;
manifest font menyimpan URL, SHA-256, ukuran dan commit sumber lisensinya. Build
dan renderer memakai aset repository; tidak mengambil font dari internet.

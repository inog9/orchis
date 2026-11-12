# Perjalanan produk ujung-ke-ujung

Dokumen ini menggambarkan perjalanan lengkap pengguna Orchis: dari aset mentah sampai laporan yang
diverifikasi. Dua engine yang dilihat pengguna: **Web pentest (orchis-redteam-agent)** untuk sisi luar dan
**Installed software check (DefenseBot)** untuk sisi dalam. Nmap dan Nuclei adalah langkah persiapan
internal orchis-redteam-agent dan tidak pernah ditampilkan di UI.

## Peta perjalanan

```
 Organisasi
    │  buat organisasi
    ▼
 Aset ──────────────────────────────┐
    │  tambah URL web dan/atau      │
    │  pasang agent                 │
    ▼                              │
 Kesiapan (checklist per aset) ────┘
    │  1. aset dalam organisasi
    │  2. pengujian diotorisasi        (admin/analyst: tab Assets organisasi)
    │  3. agent terpasang dan online   (admin: Assets → Agents)
    │  4. platform didukung            (Linux, macOS; Windows tampil "unsupported")
    │  5. koleksi disetujui            (admin: tab Agents → consent dokumen saat ini)
    │  6. URL web disetujui            (sisi luar saja)
    │  7. di dalam jendela pindai      (admin: scan control; stop darurat menutup semuanya)
    ▼
 Pindai baru (wizard 4 langkah)
    │  1 Organisasi → 2 Yang dijalankan → 3 Sasaran → 4 Tinjauan
    │  kartu: Web pentest / Installed software check / Full assessment
    ▼
 Detail pindai (satu baris engine per aset)
    │  orchis-redteam-agent:      Bersiap → Menguji → Menyusun laporan
    │  DefenseBot:   Menunggu pemindai → Mengumpulkan → Mencocokkan
    │  Full assessment: engine dipilih per aset (URL, agent, atau keduanya)
    ▼
 Temuan
    │  metode (luar/dalam), sumber severity, status perbaikan,
    │  known-exploited, "seen by scanner", "also found by", triage, retest
    ▼
 Laporan (snapshot beku per organisasi)
    │  executive summary → fleet overview → per host → hardening gaps → coverage → provenance
    ▼
 Perbaiki → Retest → "resolved / still present" (cakupan parsial tidak pernah mengklaim resolved)
```

Full assessment menerima aset campuran dalam satu pindai: aset dengan URL yang disetujui
menjalankan orchis-redteam-agent, aset dengan agent menjalankan DefenseBot, dan aset dengan keduanya
menjalankan kedua engine. DefenseBot menunggu Nmap aset yang sama bila tahap itu ada;
ia tidak menunggu seluruh pentest orchis-redteam-agent selesai.

## Siapa boleh apa

| Langkah | Viewer | Analyst | Admin |
|---|---|---|---|
| Melihat aset, temuan, laporan | ✓ | ✓ | ✓ |
| Menjalankan pindai (semua engine) | — | ✓ | ✓ |
| Retest finding | — | ✓ | ✓ |
| Triage finding | — | ✓ (finding.write) | ✓ |
| Otorisasi pengujian aset | — | — | ✓ (asset.authorize) |
| Menyetujui koleksi endpoint (consent) | — | — | ✓ (settings.manage) |
| Jendela pindai & stop darurat | — | — | ✓ (settings.manage) |

## Prasyarat per engine

| Prasyarat | Web pentest | Installed software check |
|---|---|---|
| Pengujian diotorisasi | wajib | wajib |
| URL web disetujui | wajib | tidak perlu |
| Agent terdaftar, online | tidak perlu | wajib |
| Platform didukung | tidak relevan | Linux/macOS |
| Koleksi disetujui (consent saat ini) | tidak perlu | wajib |
| Jendela pindai terbuka, tanpa stop darurat | wajib | wajib |

Alasan setiap prasyarat yang belum terpenuhi selalu tampil di wizard (sasaran nonaktif dengan alasan
dan tautan perbaikan) dan di checklist aset — tidak ada aset yang disembunyikan.

## Perlindungan yang tetap berlaku

- Launch idempoten (Idempotency-Key), dijaga otorisasi, scan control, stop darurat, dan audit.
- Hanya satu run orchis-redteam-agent aktif per aset; run kedua ditolak dengan pesan yang jelas.
- DefenseBot menunggu Nmap aset yang sama pada Full assessment agar tag "seen by scanner" terisi.
- Data agent tidak dipercaya; koleksi hanya query tetap read-only yang disetujui.

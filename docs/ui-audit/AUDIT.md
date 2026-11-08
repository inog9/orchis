# AUDIT UI Orchis — Fase A (sebelum redesign)

Tanggal: 2026-11-08. Basis: clone `github.com/inog9/orchis` @ `0ed05d1`, branch `feat/ui-v3-redesign`.
Desain rujukan: `fixed.pen` ("Argus x Orchis - Komponen", "Organisasi v3 Dark/Light", "Findings v3") + keputusan mengikat pemilik (PROMPT §1b). Rubrik: aturan antislop (core R-01–R-40, ui, copywriting, layoutmobile, human) — lihat `TOOLS.md`.
Bukti visual: `docs/ui-audit/shots/before/` — 208 screenshot (24 rute × tema × bahasa × lebar, state kosong/muat/galat, pass interaktif) + 13 gambar beranotasi `mark-UI-*.png`. Inventaris elemen: `harvest.json`, log klik: `interact-log.json`.

## Cara audit

- App dijalankan lokal (127.0.0.1:18190 data demo seed 7; 127.0.0.1:18191 database kosong; infra compose terisolasi `deploy/compose.audit.yml`). Tidak ada sentuhan ke `/opt/orchis` atau produksi.
- Setiap rute dibuka pada: tema light+dark, bahasa ID+EN, lebar 1440/820/390; state kosong, memuat (tunda 2,5 s), galat (API 500); klik interaktif terpandu (wizard, tab, drawer, dialog, palet perintah, login salah); urutan Tab direkam.
- Pemeriksaan otomatis: error konsol, overflow horizontal, radius terkomputasi, status disabled tombol, state terpilih (computed style sebelum/sesudah klik).

## Ringkasan

| Kategori | Blocker | Major | Minor |
|---|---|---|---|
| Keputusan pemilik dilanggar (tema, radius, engine) | 2 | 3 | 0 |
| Seleksi & kontrol | 1 | 3 | 2 |
| Teks, copy, i18n | 0 | 5 | 3 |
| Kotak & kerangka (box clutter) | 0 | 4 | 3 |
| Top bar & navigasi | 1 | 2 | 0 |
| Aksesibilitas & responsif | 1 | 2 | 1 |
| Alur (FLOW) | 0 | 3 | 3 |
| **Total** | **5** | **22** | **12** |

Konsol & jaringan: **nol error** pada semua rute utama saat pemakaian normal (login valid, ID residual: `harvest.json` 401 berasal dari konteks tamu skrip, 404 dari id scan yang belum terselesaikan di putaran pertama, 500 disuntikkan sendiri untuk state galat).

---

## A. Keputusan pemilik dilanggar

**UI-001 · Login terkunci gelap di tema terang, tanpa toggle.** `/login` (juga `/forgot`, `/reset`) dengan `orchis.theme=light` tetap gelap total: kunci `:root[data-theme='light'] .auth …` di `ui2.css` memaksa token gelap. Tidak ada toggle tema di halaman. Melanggar keputusan §1b-3. Bukti: `mark-UI-001-login-gelap.png` (tema light tersimpan, layar gelap). Perbaikan: hapus kunci paksa, ikuti tema (bawaan putih), tambah tombol ikon tema kecil kiri-atas dengan nama aksesibel EN/ID, operasi keyboard. [antislop R-21/R-34, R-39]

**UI-002 · Sudut membulat di mana-mana.** ~150 nilai radius hard-coded: kartu 8–12px, tombol/input/select 8–10px, pil `999px` (run pill, avatar), tag 6px. Inventaris terkomputasi (contoh `organizations`): `button.gsearch=10px, button.btn-new=10px, select=9px, button.runpill=999px, a.skip-link=8px`. Keputusan §1b-1: semua radius = 0; hanya radio, dot status, avatar yang bulat. Bukti: semua screenshot; `mark-UI-006-orgs.png` kotak 4. Perbaikan: set `radius-ctl/tag/panel` = 0, hapus radius literal (termasuk inline dan pembungkus avatar/Lucide), regression check lewat inventaris radius terkomputasi. [R-11]

**UI-003 · Tema bawaan mengikuti OS, bukan putih.** `theme.ts` `currentTheme()` = pilihan tersimpan ?? preferensi sistem; `index.html`/listener `prefers-color-scheme` ikut serta. Keputusan §1b-2: tanpa pilihan tersimpan → **light** selalu. Bukti: `web/src/lib/theme.ts:14-16`. Perbaikan: default `light`, hapus listener OS, uji tanpa kilap gelap paint pertama, perbarui tes yang mengasumsikan dark/sistem. [R-21, R-34]

**UI-004 · Nama engine & jargon internal bocor ke UI.** (a) Overline detail scan: **"INJECTOR SERVER · standard"**; (b) kolom PRESET daftar scan: **"standard"**; (c) KPI "Pindai terakhir": **"standard"**; (d) panel organisasi: **"Worker scan tidak tersedia: DefenseBot, Hackbot Lite, Nmap, Nuclei"** — menyebut Nmap/Nuclei (dilarang) dan **"Hackbot Lite"**, nama engine ketiga yang tidak pernah diperkenalkan; (e) copy "container worker scan". Melanggar §1b-4 dan kosakata §3 Phase B. Bukti: `mark-UI-016-scan.png` kotak 1, `mark-UI-017-orgtab.png` kotak 1, daftar Pindai. Perbaikan: peta nama internal → Hackbot/DefenseBot; preset tidak ditampilkan sebagai teks (jika perlu, "Pemindaian cepat/standar" tanpa kata `preset/standard`); pesan ketersediaan hanya menyebut Hackbot/DefenseBot. [R-16, §1b-4]

**UI-005 · Nama run berbahaya di run-pill.** aria run-pill: "1 eksekusi aktif. PT Audit…" — istilah "eksekusi" vs "scan" vs "run" tidak konsisten (lihat UI-013). minor. [R-16]

## B. Seleksi & kontrol

**UI-006 · Kartu engine wizard tanpa status terpilih.** `/simulations/new` langkah 2: computed style kartu `.wiz-card` **identik** sebelum dan sesudah diklik (bg transparan, border `rgb(24,24,27)`); tanpa `role="radio"`, tanpa `aria-checked`. Orang tidak bisa tahu apa yang dipilih. Ini keluhan eksplisit pemilik. Bukti: `mark-UI-011-wizard2.png`, probe di `interact-log.json`. Perbaikan: kartu opsi dengan 4 state + fokus (DESIGN.md §7), terpilih = `accent-soft` + `accent-line` + centang; semantik `radiogroup/radio`. [R-39, blocker]

**UI-007 · Stepper palsu.** Langkah wizard hanya deret angka+teks; tidak ada state aktif (kecuali ✓ setelah selesai), tidak bisa diklik untuk kembali, tidak ada progres. Bukti: `new-scan--light--id--1440.jpg`, `mark-UI-010-wizard1.png`. Perbaikan: stepper nyata (aktif/tercapai/mendatang, bisa diklik ke langkah tercapai). [major]

**UI-008 · Langkah 1 kosong dan tanpa konteks.** Satu `<select>` kecil di kartu raksasa; tanpa penjelasan apa yang dipilih nanti; slug internal "demo-vantage-0"/"pt-audit-contoh" tampil di opsi. Pada `/simulations/new` **tanpa** `?org=`, tidak ada tombol Lanjut sampai organisasi dipilih, tanpa petunjuk. Bukti: `mark-UI-010-wizard1.png`; probe tanpa param. Perbaikan: langkah organisasi sebagai daftar baris pilih (bukan select), tanpa slug, hint langkah berikutnya. [major]

**UI-009 · Sasaran wizard: baris tanpa kesiapan yang jelas.** Langkah 3 memakai checkbox polos; readiness per engine dan baris disabled-dengan-alasan (tuntutan §3 Phase B-4) tidak terlihat pada data demo; kolom tidak menjelaskan mengapa baris tak bisa dipilih. [major — diperiksa ulang saat redesign]

**UI-010 · Kontrol campur aduk.** Halaman Temuan: select native tak bergaya ("Semua organisasi", "Semua jenis", "Urut: severity") + segmented control custom ("Per organisasi/Per aset/Daftar datar") + chip — tiga bahasa kontrol berbeda untuk tugas yang sama. Drawer temuan: select triage native + dua tombol bergaya berbeda. Bukti: `mark-UI-023-findings.png`, `mark-UI-020-drawer.png`. Perbaikan: satu set primitif kontrol (Field/Select/Segmented/Chip) dari DESIGN.md. [major]

**UI-011 · Tombol run disabled tanpa alasan inline.** Pada aset belum diotorisasi, ketiga tombol ("Jalankan pentest web", "Jalankan pemeriksaan aplikasi terpasang", "Jalankan penilaian penuh") benar-disabled ✔, tetapi alasannya hanya tersirat di checklist atas; tidak ada `title`/teks bantu di tombol. Perbaikan: alasan singkat di samping/dalam tombol. [minor]

**UI-012 · Checkbox/label: area hit & status.** Checkbox sasaran wizard native tanpa label gabungan; tidak ada state indeterminate untuk "pilih semua". Perbaikan: baris tabel dengan checkbox + label penuh + indeterminate. [minor]

## C. Teks, copy, i18n

**UI-013 · String Inggris bocor di mode Indonesia.** (a) Checklist kesiapan: "— Not authorized for testing", "No agent is installed on this asset"; (b) laporan: banner galat PDF campur ("PDF: gagal. PDF rendering or storage failed. Retry after…"), kotak "DEMO / SYNTHETIC - not a real assessment…", judul "Synthetic demo data - risk not evaluated."; (c) halaman galat 500: "Something broke on our side" (tanpa terjemahan, tanpa aksi); (d) JSON drawer: "Synthetic/demo finding - not a real scan result". Bukti: `mark-UI-012-asset.png`, `mark-UI-015-report.png`, `mark-UI-021-err500.png`, `mark-UI-020-drawer.png`. Perbaikan: semua string lewat `textUI` di `en.ts`+`id.ts`; uji lewat pass id. [R-38, major]

**UI-014 · Interpolasi rusak, teks menempel.** "…diotorisasi — Not authorized for testing**Perbaiki**", "…asset.**Perbaiki**" — tautan "Perbaiki" menempel tanpa spasi;hasilnya link tak terbaca dan sulit diklik. Bukti: `mark-UI-012-asset.png` kotak 1–3. Perbaikan: susun baris checklist (status, teks, aksi) sebagai struktur, bukan interpolasi string. [major]

**UI-015 · Output debug bocor.** Caption KPI detail scan: **"belum selesai (ended_at = null)"** — ekspresi programmer tampil ke pengguna. Bukti: `mark-UI-016-scan.png` kotak 2. [major]

**UI-016 · JSON mentah di drawer temuan.** "Rincian" menampilkan blok kode `{"note": "Synthetic/demo finding…", "port": 80, …}` — data mentah tanpa format manusia; label "Salin" satu-satunya pemanfaatan. Perbaikan: tampilkan pasangan kunci bernilai sebagai baris teks; JSON hanya lewat "Salin". [minor]

**UI-017 · Kosakata tidak konsisten.** "Pindai" (nav) vs "Scan baru" (tombol) vs "Kembali ke scan" (detail); "eksekusi" (run-pill) vs "scan"; "assessment beku"/"ASSESSMENT BEKU · VERSI 1" (jargon assessment); preset "quick" diterjemahkan "cepat" di metadata laporan; slug/UUID sebagai nama (`demo-customer-portal-02`, `01a1b9ac-333b`). Perbaikan: satu kosakata (DESIGN.md §9) — Scan, Laporan/Snapshot; nama manusiawi untuk entitas demo. [major]

**UI-018 · Em dash & nilai kosong.** KPI "Temuan —" memakai em dash sebagai nilai kosong (R-02 melarang em dash; nilai kosong harus "0" atau "Belum ada"). Bukti: `mark-UI-017-orgtab.png` kotak 3. [minor]

**UI-019 · Copy tidak gramatikal.** Footer sidebar: "Sebagai layanan perlu perhatian" (EN asalnya "…needs attention") — tidak jelas maksudnya. [minor]

**UI-020 · Tanggal format US.** "10/8/2026, 12.27:11" ambigu (hari/bulan). Perbaikan: format ID "10 Agu 2026 12.27". [minor]

## D. Kotak & kerangka

**UI-021 · Kartu metrik terpisah, bukan strip.** Semua KPI (Organisasi, Temuan, detail scan, org overview) dirender sebagai 3–4 kartu rounded terpisah dengan bayangan halus; desain `fixed.pen` = **satu permukaan penuh** dengan sel bersekat `border-left`, nilai `font-ui` 28. Saat ini nilai memakai `font-mono`. Bukti: `mark-UI-006-orgs.png` kotak 3, `mark-UI-016-scan.png` kotak 3, `mark-UI-023-findings.png` kotak 3. [major]

**UI-022 · Organisasi sebagai grid kartu.** Daftar organisasi = kartu 3 kolom dengan "Lihat organisasi →" per kartu; desain = **baris tabel** (`AG Org Row`) dengan kolom Org/Status/Aset/Agent/Temuan/Bar/Laporan. Bukti: `mark-UI-006-orgs.png` kotak 4. [major]

**UI-023 · Banner bertumpuk di semua halaman.** Banner "Data demo" (dengan tombol penuh "Bersihkan data demo") + notice halaman = ±200px tinggi layar di **setiap** halaman, mendorong konten turun; keduanya bertahan walau sudah ditutup (notice punya ×, banner tidak). Perbaikan: satu notice; banner demo hanya di beranda/pertama kali, bisa ditutup permanen. [major]

**UI-024 · Kartu dalam kartu.** "Kesiapan" (asset detail), panel Settings, dan bagian detail scan membungkus konten dalam kartu berbingkai di dalam halaman berbingkai. Perbaikan: section flat + pemisah. [minor]

**UI-025 · Pil "99+" merah pada nav Temuan.** Warna `sev-critical` dipakai untuk count nav (bukan status); desain: count abu `text-3`. Bukti: `mark-UI-006-orgs.png` kotak 6. [minor]

**UI-026 · Bar/segmen severity tanpa label & satuan.** Bar "Sebaran severity" dan segmen di kartu organisasi tidak memiliki nilai numerik per segmen (hanya legenda chip terpisah di bawah). [minor]

## E. Top bar & navigasi

**UI-027 · Pil run status ramai.** "● Tidak ada eksekusi aktif" tampil permanen; pemilik: hapus, tampilkan hanya indikator kecil saat benar-benar ada run. Bukti: `mark-UI-006-orgs.png` kotak 1. [major — keputusan eksplisit]

**UI-028 · Dropdown bahasa berat.** Ikon globe + "Indonesia" + chevron sebagai select besar; pemilik: toggle minimal `EN | ID` atau item menu akun. Bukti: `mark-UI-006-orgs.png` kotak 2. [major — keputusan eksplisit]

**UI-029 · Top bar overflow di 390px.** `.topbar__actions` lebar 389px dengan tepi kanan 405 > 390 — **semua 24 rute** overflow horizontal di lebar 390 (dan 820 aman). Pelanggaran R-03 paling sistemik. Bukti: `harvest.json` (24 entri overflow), probe elemen; screenshot `*-390.jpg`. Perbaikan: susutkan top bar di breakpoint kecil (menu, tanpa pil), target sentuh ≥44px. [blocker]

## F. Aksesibilitas & responsif

**UI-030 · Makna hanya dari warna.** Header grup temuan: "●1 ●1 ●2 ●2" — titik severity tanpa teks label; pembaca layar/pembuta warna tidak mendapat informasi. Bukti: `mark-UI-023-findings.png` kotak 2. Perbaikan: teks "Kritis 1" per segmen + `aria-label`. [major]

**UI-031 · Halaman galat 500 tanpa aksi.** "Something broke on our side" tanpa tombol "Coba lagi"/tautan beranda; EN-only. Bukti: `mark-UI-021-err500.png`. [major]

**UI-032 · Fokus keyboard ✔ (positif).** Urutan Tab logis (skip-link → pencarian → topbar → sidebar), outline 2px terlihat pada semua elemen yang diuji. Dicatat sebagai garis dasar yang harus dipertahankan. [positif]

**UI-033 · Target sentuh di 390.** Dengan overflow top bar (UI-029), beberapa kontrol <44px tidak terjangkau nyaman; diperbaiki bersamaan. [minor]

## G. Positif yang layak dipertahankan

Empty state Organisasi (judul + penjelasan + aksi utama + tautan sekunder) sesuai R-27; pesan login salah jelas ("Check both and try again."); skip-link ada; daftar Pindai memakai tabel baris (arah desain benar); nama engine di daftar scan sudah "Hackbot" (benar untuk scan Hackbot); badge run berubah sesuai state nyata ("1 eksekusi aktif" saat scan berjalan).

---

## FLOW — perjalanan end-to-end

**FLOW-001 (a) Login pertama & organisasi kosong.** Login → error kredensial jelas ✔ → login sukses → organisasi kosong: empty state baik ✔ → "Buat organisasi" tersedia ✔. Hambatan: halaman 500 tanpa aksi (UI-031); bahasa mengikuti browser sebelum preferensi akun tersimpan (perilaku wajar, didokumentasikan). Status: jelas.

**FLOW-002 (b) Tambah aset web & pentest Hackbot.** Notice "Tetapkan aset ke organisasi →" membantu ✔. Wizard: langkah 1 select minimalis (UI-008); langkah 2 kartu tanpa seleksi (UI-006, **blokir alur** — orang tidak yakin pilihannya tersimpan); langkah 3 sasaran checkbox polos (UI-009); langkah 4 tinjauan ada ✔. Scan dibuat berjalan; detail scan lambat memuat lalu tampil; scan tanpa injector menggantung "Berjalan" tanpa penjelasan di UI bahwa worker tidak tersedia (pesan hanya di overview organisasi, UI-004d). Status: **tidak jelas** — perbaiki UI-006/007/008/009 + pesan antrean di detail scan.

**FLOW-003 (c) Pasang agent & koleksi DefenseBot.** "Pasang agent" tersedia di overview ✔; checklist kesiapan menjelaskan syarat per langkah ✔ (tapi teks menempel UI-014); tombol DefenseBot disabled tanpa agent ✔ dengan alasan di checklist; consent memerlukan persetujuan eksplisit (baris "Koleksi disetujui untuk persetujuan saat ini" ✔). Agent nyata tidak dipasang di lingkungan audit (di luar cakupan UI); alur UI sampai consent jelas. Status: jelas setelah UI-014.

**FLOW-004 (d) Penilaian penuh satu aset.** Opsi "Penilaian penuh" ada di wizard langkah 2 dan di asset detail ("Jalankan penilaian penuh") ✔ — dua pintu yang konsisten. Bergantung UI-006. Status: jelas setelah UI-006.

**FLOW-005 (e) Baca temuan, triage, uji ulang, laporan & perbandingan.** Drawer temuan terbuka ✔, triage + simpan ✔, "Uji ulang" ada ✔ (butuh scan aktif); laporan: pratinjau PDF gagal menampilkan banner + "Coba buat PDF lagi" ✔, JSON tetap tersedia ✔, tombol "Unduh PDF" **tetap aktif** padahal PDF gagal (klik akan gagal — perlu disabled dengan alasan atau arahkan ke banner); "Perbandingan scan" tersedia di detail scan ✔. Status: hampir jelas; perbaiki tombol unduh PDF gagal (baru: UI-034, minor-major).

**FLOW-006 (f) Perbaiki lalu verifikasi.** Membutuhkan scan ulang nyata (injector) — di lingkungan audit tanpa injector, scan tetap "Berjalan" tanpa indikasi antrean pada detail scan. Ditandai untuk diverifikasi pada Fase B dengan injector lokal (flow26). Status: ditangguhkan (butuh infra).

**FLOW-007 (g) Tugas pengaturan.** Tab Pengguna/Peran/Integrasi/Log audit/Tag berfungsi ✔; form retensi tersimpan (tombol disabled bila tak berubah ✔); "Stop darurat" ada ✔ tapi danger zone menyatu dengan kartu lain (UI-024, mark-UI-024). Status: jelas.

**FLOW-008 (h) Kesalahan & kasus tepat.** Aset tak terotorisasi: tombol run disabled ✔ (alasan perlu inline, UI-011); consent belum: baris checklist menandai ✔; "Di dalam jendela pindai" ada di checklist ✔; Stop darurat ✔; sumber mati: halaman 500 tanpa aksi (UI-031); agent offline: jumlah "0 online" tampil ✔ (tanpa penjelasan dampak — minor). Status: sebagian jelas.

---

## Daftar perbaikan prioritas (masuk Fase B)

1. **Blokir:** UI-029 (overflow 390), UI-006 (seleksi wizard), UI-001 (login terkunci gelap).
2. **Keputusan pemilik:** UI-002 (radius 0), UI-003 (tema putih bawaan), UI-004 (nama engine), UI-027/028 (top bar tenang + EN|ID).
3. **Kerangka:** UI-021 (strip metrik), UI-022 (baris organisasi), UI-023 (banner bertumpuk), UI-024 (flat section).
4. **Copy/i18n:** UI-013/014/015/017 (EN leak, interpolasi, debug, kosakata) — plus UI-016/018/019/020.
5. **Kontrol:** UI-007/008/009/010 (stepper, langkah 1, sasaran, primitif kontrol), UI-011/012 (alasan disabled, checkbox).
6. **A11y:** UI-030 (label severity), UI-031 (500 dengan aksi), UI-033 (target sentuh).

Rincian gaya per halaman prioritas mengikuti `web/DESIGN.md`. Setiap catatan diberi tanda `fixed` / `deferred (alasan)` / `won't fix (alasan)` pada tabel status di akhir Fase B, dengan screenshot `after` berpasangan.

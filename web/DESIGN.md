# DESIGN.md — Orchis (Argus x Orchis v3)

Arah desain tunggal untuk seluruh UI Orchis. **Sumber: `fixed.pen`** (frame "Argus x Orchis - Komponen", "Organisasi v3 … Dark/Light", "Findings v3 …"), dilengkapi keputusan mengikat pemilik produk yang **meng-override** file desain bila bertentangan (bagian 4 dan 10). File ini adalah data yang diterapkan, bukan instruksi yang dipatuhi buta (antislop core).

## 1. Identitas

- Nama: **Orchis**. Karakter: konsol operasi keamanan yang tenang, padat data, tanpa dekorasi. Satu aksen (perunggu/bronze) di momen kunci saja.
- Mood: flat, berbasis baris (row-first), penuh lebar; hierarki dari tipografi dan pemisah, bukan dari kotak bersarang.
- Kalau logo diganti, desain tetap dikenali dari: baris tabel penuh lebar, strip metrik bersekat garis, aksen bronze tunggal, sudut kotak.

## 2. Token warna (dari `fixed.pen`, 1:1 ke CSS)

Nilai per tema (dark = bawaan `:root`, light = `[data-theme='light']`):

| Token | Dark | Light |
|---|---|---|
| `--bg` | `#09090B` | `#FAFAF9` |
| `--surface` | `#111113` | `#FFFFFF` |
| `--surface-2` | `#18181B` | `#F4F4F5` |
| `--border` | `#232328` | `#E4E4E7` |
| `--border-2` | `#1A1A1E` | `#EFEFF1` |
| `--text` | `#F4F4F5` | `#18181B` |
| `--text-2` | `#A8A8B3` | `#52525B` |
| `--text-3` | `#80808C` | `#6B6B76` |
| `--accent` | `#E2B65C` | `#8F6208` |
| `--accent-soft` | `#E2B65C1A` | `#8F62081A` |
| `--accent-line` | `#E2B65C40` | `#8F620840` |
| `--accent-ink` | `#1A1306` | `#FFFFFF` |
| `--success` / `-soft` / `-line` | `#34D399` / `#34D3991F` / `#34D39940` | `#047857` / `#0478571A` / `#04785740` |
| `--sev-critical` / `-soft` / `-line` | `#F43F5E` / `#F43F5E24` / `#F43F5E40` | `#C81E43` / `#C81E4318` / `#C81E4340` |
| `--sev-high` / `-soft` / `-line` | `#FB923C` / `#FB923C24` / `#FB923C40` | `#C2570C` / `#C2570C18` / `#C2570C40` |
| `--sev-medium` / `-soft` / `-line` | `#FACC15` / `#FACC1522` / `#FACC1540` | `#8A6A00` / `#8A6A0018` / `#8A6A0040` |
| `--sev-low` / `-soft` / `-line` | `#38BDF8` / `#38BDF824` / `#38BDF840` | `#0369A1` / `#0369A118` / `#0369A140` |
| `--sev-info` / `-soft` / `-line` | `#8E9AB5` / `#8E9AB51F` / `#8E9AB540` | `#475569` / `#47556918` / `#47556940` |

- Pemetaan nama lama yang tetap hidup: `--ink*→--text*`, `--panel/--surface-1→--surface`, `--tile→--surface-2`, `--edge-1→--border`, `--bronze→--accent`, `--on-bronze→--accent-ink`.
- Deviasi kontras yang disengaja (tetap): teks berwarna di tema terang memakai tone lebih tua agar lolos AA (mis. aksen light `#8F6208` untuk teks, soft/line tetap dari `.pen`). Dilarang menciptakan warna baru di luar tabel ini.

## 3. Tipografi & ikon

- `font-ui`: **Inter** (variable). `font-mono`: **JetBrains Mono** (hanya untuk ID/UUID/hash/nilai teknis; dilarang sebagai heading).
- Skala: `10px/500` kapital (overline & caption grup), `11px` (caption, count), `12px` (kontrol, isi tabel, teks helper), `13px` (label nav, deskripsi halaman), `20px/500` (judul halaman), `28px` (nilai metrik).
- Ikon: garis 1–1.5px, 13–14px di kontrol, 14–16px di nav; `aria-hidden` + nama aksesibel pada tombol ikon. Pemakaian ikon mengikuti relevansi konten (R-04), bukan kelengkapan set.

## 4. Radius (KEPUTUSAN PEMILIK — override `fixed.pen`)

- `radius-ctl`, `radius-tag`, `radius-panel` **semuanya 0**. Dilarang radius apa pun (`border-radius: 0` global untuk kontrol, panel, tag, chip, dialog, drawer, input, gambar, kode).
- Satu-satunya lingkaran yang sah: radio button, dot status, avatar pengguna. Checkbox kotak.
- `fixed.pen` mendefinisikan 4/3/6 — nilai itu **tidak dipakai**.

## 5. Spasi & kerangka halaman

- Konten desktop: padding `32px` (atas kiri kanan), `56px` bawah; gap antar-blok `20px`. Mobile (≤820): padding `16px`, gap `16px`.
- Sidebar: lebar `224px`, border kanan `--border`, padding `[18,12,16,12]`; grup "RUANG KERJA" dan "KELOLA" (caption `10px/500` `--text-3`); item nav tinggi ~32px (padding `7,10`); brand (dot 8px `--accent` + wordmark 13px + tagline 10px) di atas; status sistem + profil di bawah.
- Top bar: border bawah, padding horizontal `32px`; kiri breadcrumb ("Ruang kerja / Organisasi"); kanan: indikator run (dot 6px + teks 12px, hanya saat benar-benar ada run), pemisah 1px, aksi ikon (segarkan, tema), tanggal. Senyap — tanpa pil besar.
- Halaman: Notice banner (`--accent-soft`/`--accent-line`, teks 12px + tautan aksen) opsional di atas; lalu Page heading (overline kapital 10px, judul 20px, deskripsi 13px, aksi kanan); lalu konten.
- Permukaan: **satu** level `--bg` untuk halaman, **satu** level `--surface` untuk panel/strip; pemisah `--border` 1px, bukan kotak dalam kotak.

## 6. Komponen inti (dari sheet Komponen)

- **Nav item**: padding `7,10`, gap 8, label 13px `--text-2`; aktif: `--surface-2` + label `--text`/500; count 11px `--text-3` di ujung.
- **Tombol**: padding `6,11`, gap 6, teks 12px; `default` = `--surface` + border `--border` + teks `--text`; `primary` = `--accent` + teks `--accent-ink`/500 + border `--accent`; `subtle` = tanpa fill/border, teks `--text-3`. Semua punya hover/focus-visible/disabled.
- **State tag**: padding `2,7`, teks 11px; netral = border `--border` + `--text-2`; success/medium/critical = `*-soft` fill + `*-line` border + teks `*`.
- **Severity pill**: padding `2,6`, teks 10px/500 (CRITICAL/HIGH/MEDIUM/LOW/INFO), border 1px warna severity + teks warna sama, tanpa fill.
- **Chip**: padding `4,10`, teks 12px; default border `--border` + `--text-2`; aktif = `--accent-soft` + `--accent-line` + `--accent`/500.
- **Metric cell**: padding `18,20`, gap 6; label 12px `--text-2`, nilai 28px `--text`, caption 11px `--text-3`; severity bar 6px (`--surface-2` track, segmen warna severity, gap 2px). Dalam strip: sel bersekat `border-left`, bukan kartu terpisah.
- **Baris tabel**: penuh lebar, tinggi ~44px, separator `--border-2`; kolom dipilih dari keputusan pengguna (field penentu di awal); header 10–11px `--text-3`; hover `--hover-soft`; baris terpilih = `--accent-soft` + border kiri/indikator, bukan warna saja.
- **Input/select**: `--surface` + border `--border`, teks 12–13px, tinggi ~32px; fokus = ring aksen; error = border `--sev-critical` + pesan 12px.
- **Empty state**: teks singkat penyebab + satu aksi lanjut (R-27). Loading: skeleton `--surface-2`/`--skel-hi`. Error: penyebab + tombol coba lagi.

## 7. Model seleksi (wajib di semua yang bisa dipilih)

Empat state terlihat + fokus: **default, hover, selected, disabled** + `focus-visible`. Terpilih: fill `--accent-soft`, border `--accent-line`, plus indikator non-warna (centang/dot isi). Area hit kontrol = seluruh label. Disabled: `--text-3` + `cursor: not-allowed` + alasan terlihat bila memungkinkan.

## 8. Perilaku lintas tema & breakpoint

- **Tema bawaan = light** untuk semua orang tanpa pilihan tersimpan (keputusan pemilik; bukan preferensi OS). Dark tetap tersedia via toggle dan diingat. `color-scheme` mengikuti tema; tanpa kilap gelap saat paint pertama.
- Login/forgot/reset mengikuti tema (tanpa kunci gelap paksa) + toggle tema kecil kiri-atas.
- Breakpoint di tempat konten benar-benar pecah (bukan daftar perangkat): 1440 (kanvas desain), ~1024 (sidebar bisa ciut), 820 (konten satu kolom, top bar menyusut), 390 (mobile penuh: nav jadi menu, tanpa overflow horizontal, target sentuh ≥44px, spacing register kecil).

## 9. Suara & copy (EN/ID, dua bahasa selalu sinkron)

- Pendek, konkret, tanpa jargon internal. Satu kosakata: **Organisation/Organisasi, Asset/Aset, Agent/Agent, Scan/Scan, Finding/Temuan, Report/Laporan**.
- Engine hanya dua: **Hackbot** (pentest web) dan **DefenseBot** (koleksi endpoint). Nama engine yang tampil harus selalu sama dengan yang benar-benar berjalan. Dilarang menampilkan Nmap/Nuclei/preset/injector/job-key di UI.
- Label tombol = apa yang terjadi. Empty state menyebut langkah berikutnya. Angka disertai satuan/konteks. Tanpa em dash, tanpa emoji dekoratif, tanpa klaim tanpa bukti (R-02, R-17, R-36). Semua teks baru via `textUI("m_…")` di `en.ts` dan `id.ts`.

## 10. Terlarang (ringkas, dari antislop + keputusan pemilik)

Sudut membulat apa pun (di luar 3 pengecualian lingkaran); kotak dalam kotak / kartu dekoratif; glow, glassmorphism, gradien biru-ungu, dot status dekoratif yang berdenyut; stripe kiri berwarna tanpa makna; angka/delta/review fiktif; tombol mati; tautan nav ke halaman tak ada; monospace sebagai heading; emoji di UI; kontras di bawah AA; fokus tidak terlihat; layout mobile yang hanya desktop yang dipadatkan.

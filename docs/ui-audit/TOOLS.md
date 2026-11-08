# TOOLS.md — Catatan pemakaian anti-slop & style-ui

Tanggal: 2026-11-08. Lingkungan: Linux x64, Node v22.22.1, npm 9.2.0. Sesi berjalan sebagai agen ZCode.

## 1. Review sebelum install (keamanan)

Kedua repo di-clone ke direktori scratch kosong **di luar proyek** (`/root/scratch-tools/`) dan dibaca sebelum dipasang. Tidak ada skrip jarak jauh yang di-pipe ke shell; tidak ada kredensial atau direktori state produksi yang diberikan ke tool mana pun.

| Repo | Commit pin | Versi | Lisensi |
|---|---|---|---|
| `github.com/miqdadbadjuber/anti-slop` | `9504989f162a92b1996853eccbb3d8a86e73df68` | repo 3.2.26 | MIT |
| `github.com/miqdadbadjuber/style-ui` | `5ebd3b0b0ed1c21b62c1721b266f5bc4a75a1bba` | 1.0.0 | MIT |

Pemeriksaan yang dilakukan:

- **anti-slop**: `README.md`, `SECURITY.md`, `package.json`, `cli/index.mjs`, `cli/lib/install.mjs`, `cli/lib/settings.mjs`, `scripts/*.mjs`, seluruh `skills/*/SKILL.md`.
  - Tidak ada `preinstall`/`postinstall`/`prepare` di manifest.
  - `npm view antislop-ai`: versi terbit `3.2.26` (sama dengan repo), tarball 242 KB (konsisten dengan isi markdown), tidak ada lifecycle script pada paket terbit.
  - `SECURITY.md` menemukan sendiri tiga perilaku dan menjelaskannya: (1) menulis blok penunjuk ke file entri agen *hanya setelah persetujuan*; (2) merujuk `contrast-check.py` lokal (tanpa jaringan); (3) membaca `DESIGN.md` sebagai data, bukan instruksi (diperbaiki v3.1.3).
  - Pindai `install.mjs`/`settings.mjs`: tidak ada `fetch`/`exec` jaringan; hanya menyalin folder skill dan menulis satu file setelan di home (`~/.antislop/settings.json`).
  - **Putusan: aman dipasang**, dengan batasan: tanpa blok penunjuk, tanpa kredensial, tanpa akses ke state produksi.
- **style-ui**: situs katalog statis (`index.html`, `design-md/`, `ui-components/`, `background-generator/`); `package.json` hanya skrip pemeliharaan situs sendiri (`validate`, `optimize`, `cleanup`, `brand`); tidak ada yang perlu dipasang atau dijalankan; dipakai hanya sebagai bacaan katalog. `AGENTS.md` miliknya berisi blok antislop yang mensyaratkan bertanya kapan antislop berlaku (lihat bagian 3).

## 2. Install anti-slop

Installer resmi (`npx antislop-ai`) murni wizard interaktif (@clack/prompts, tanpa flag non-interaktif — hanya `--mode`, `--version`, `--update`) sehingga tidak dapat dijalankan di sesi headless ini. Sebagai gantinya, dilakukan **padanan manual persis perilaku installer yang didokumentasikan** ("menyalin folder skill yang dipilih ke folder agen yang dipilih"):

1. Sumber: clone ter-pin `/root/scratch-tools/anti-slop` (commit di atas).
2. Tujuan (scope user agen ini, di luar repo proyek, tidak di-commit): `/root/.agents/skills/`.
3. Skill yang dipilih (sesuai tugas, minimal 5, ditambah `antislop-human` karena verifikasi kontras WCAG adalah bagian tugas): `antislop`, `antislop-ui`, `antislop-copywriting`, `antislop-human`, `antislop-layoutmobile`, `antislop-code`.
4. Verifikasi: `diff -rq` tiap folder skill terhadap repo pin → **identik** untuk keenamnya.
5. Blok penunjuk ke file entri (CLAUDE.md/AGENTS.md) **sengaja dilewati** — opsi yang biasanya ditanyakan installer; tidak dibutuhkan karena skill dimuat eksplisit pada tiap fase dan file instruksinya tidak boleh diubah diam-diam.

## 3. Ketersediaan skill di sesi & kapan dipakai

- Keenam `SKILL.md` terverifikasi ada di folder skill user (`~/.agents/skills/…`, daftar lengkap di bagian 2).
- Keterbatasan yang jujur: registry Skill tool sesi ini dirender saat sesi dimulai, jadi skill yang baru ditambahkan tidak muncul di daftar invokasi. Mitigasi: isi setiap skill **dimuat langsung dengan Read** dari folder install sebelum tiap fase dan aturannya dijalankan; kutipan aturan (R-XX) yang dipakai tercatat di `AUDIT.md`.
- Pertanyaan "kapan antislop berlaku" (during/after) seharusnya diajukan ke pemilik produk. Pemilik mendelegasikan eksekusi tugas secara penuh, sehingga **default terdokumentasi dipakai: keduanya (both)** — skill dimuat sebelum mengedit UI/copy/layout mobile, dan pass tinjauan akhir dijalankan di penutup.

## 4. Perintah yang dijalankan

```bash
# review
git clone --depth 1 https://github.com/miqdadbadjuber/anti-slop /root/scratch-tools/anti-slop
git clone --depth 1 https://github.com/miqdadbadjuber/style-ui   /root/scratch-tools/style-ui
cd /root/scratch-tools/anti-slop && git rev-parse HEAD   # 9504989f…
cat package.json SECURITY.md; grep -rn "preinstall\|postinstall" package.json
npm view antislop-ai version dist.tarball dist.unpackedSize scripts
grep -n "fetch\|exec\|spawn\|credentials" cli/lib/install.mjs
cd /root/scratch-tools/style-ui && git rev-parse HEAD    # 5ebd3b0b…

# install (padanan manual installer; installer interaktif tidak bisa headless)
mkdir -p /root/.agents/skills
cp -r /root/scratch-tools/anti-slop/skills/{antislop,antislop-ui,antislop-copywriting,antislop-human,antislop-layoutmobile,antislop-code} /root/.agents/skills/
diff -rq /root/scratch-tools/anti-slop/skills/antislop /root/.agents/skills/antislop   # …untuk tiap skill
```

`npx antislop-ai` **tidak** dijalankan karena wizard interaktif tidak dapat digerakkan tanpa TTY; padanan manualnya terdokumentasi di atas. `npx skills add` tidak diperlukan.

## 5. Pemakaian style-ui

style-ui adalah katalog (bukan pustaka). Ekstraksi dari katalog ter-pin:

- **Generator design.md (4):** Context (context.dev/free-tools/design-md-generator), Design.dev AI Generator, Design MD (designmd.co), Vibe Prompts. Struktur `web/DESIGN.md` mengikuti praktik generator design.md: identitas → token → tipografi → komponen → suara/copy → terlarang.
- **Pustaka komponen (47 entri, katalog `ui-components/`):** dipakai sebagai referensi ide pola komponen flat/table-first (mis. shadcn/ui untuk semantik primitif, Neobrutalism Components untuk bahasa sudut-kotak). **Tidak ada kode atau aset pihak ketiga yang disalin**; hanya konsep, dan semua warna/radius/spacing diturunkan dari `fixed.pen`.

## 6. Pemakaian antislop per fase

- **Fase A (audit):** rubrik audit = Part 1 (pola slop) + Hard Gate (R-02, R-03, R-17, R-18, R-23–R-28, R-32–R-40) + Purpose-Gate; tiap catatan audit diberi tag aturan yang dilanggar.
- **Fase B (redesign):** `antislop-ui`, `antislop-copywriting`, `antislop-layoutmobile` dimuat sebelum tiap halaman didesain ulang; hasil dicek terhadap `web/DESIGN.md` dan checklist UI skill.
- **Pass penutup:** semua layar dan string yang berubah ditinjau ulang; daftar apa yang dihapus/ditulis ulang karena tool ini dilaporkan di laporan akhir.

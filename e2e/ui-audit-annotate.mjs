// Anotasi screenshot before: kotak bernomor di atas elemen bermasalah (tanpa dependensi gambar).
import { chromium } from 'playwright';
import { readFile } from 'node:fs/promises';
import path from 'node:path';

const DIR = '/root/work/orchis-ui-v3/docs/ui-audit/shots/before';
// Koordinat pada 1440x900 (viewport screenshot), ditandai saat review manual.
const JOBS = [
  { src: 'login--light--id--1440.jpg', out: 'mark-UI-001-login-gelap.png', boxes: [[0, 0, 1440, 900, '1: tema terang tetap gelap; tanpa toggle tema']] },
  { src: 'organizations--light--id--1440.jpg', out: 'mark-UI-006-orgs.png', boxes: [
    [533, 10, 350, 48, '1: pil "Tidak ada eksekusi aktif" ramai'],
    [1063, 10, 120, 48, '2: dropdown bahasa berat'],
    [280, 465, 1128, 106, '3: kartu metrik terpisah + nilai mono (desain: strip bersekat)'],
    [280, 640, 1128, 250, '4: kartu organisasi rounded; desain: baris tabel'],
    [186, 60, 1214, 130, '5: banner demo + notice bertumpuk'],
    [186, 275, 30, 24, '6: pil "99+" merah untuk count'],
  ] },
  { src: 'new-scan--light--id--1440.jpg', out: 'mark-UI-010-wizard1.png', boxes: [
    [280, 500, 450, 42, '1: langkah 1 = satu select kecil'],
    [281, 530, 368, 100, '2: ruang kosong raksasa, hierarki lemah'],
    [300, 555, 400, 30, '3: slug internal "demo-vantage-0" tampil'],
    [296, 505, 410, 24, '4: stepper tanpa state aktif'],
  ] },
  { src: 'int-wizard-step2.jpg', out: 'mark-UI-011-wizard2.png', boxes: [
    [288, 630, 850, 260, '1: kartu engine tanpa status terpilih (style identik sebelum/sesudah klik)'],
    [296, 505, 380, 24, '2: langkah aktif (2) tidak disorot'],
  ] },
  { src: 'asset-detail--light--id--1440.jpg', out: 'mark-UI-012-asset.png', boxes: [
    [432, 596, 280, 18, '1: EN bocor + teks menempel "testingPerbaiki"'],
    [432, 620, 290, 18, '2: "No agent is installed...Perbaiki"'],
    [432, 700, 270, 18, '3: "Not authorized for testingPerbaiki"'],
    [305, 395, 400, 45, '4: judul mono slug internal'],
    [300, 745, 680, 55, '5: tombol run disabled tanpa alasan inline'],
  ] },
  { src: 'report-detail--light--id--1440.jpg', out: 'mark-UI-015-report.png', boxes: [
    [296, 380, 190, 20, '1: "ASSESSMENT BEKU" jargon'],
    [280, 555, 1120, 80, '2: pesan galat campur EN/ID'],
    [280, 520, 1120, 34, '3: "DEMO / SYNTHETIC - not a real..." EN di mode ID'],
    [312, 475, 160, 22, '4: format tanggal US "10/8/2026" + preset "cepat"'],
  ] },
  { src: 'scan-detail--light--id--1440.jpg', out: 'mark-UI-016-scan.png', boxes: [
    [281, 375, 260, 20, '1: overline "INJECTOR SERVER · standard"'],
    [296, 595, 250, 30, '2: caption debug "(ended_at = null)"'],
    [280, 520, 1120, 110, '3: kartu metrik terpisah + mono'],
  ] },
  { src: 'int-org-tab-overview.jpg', out: 'mark-UI-017-orgtab.png', boxes: [
    [280, 735, 1120, 70, '1: "Worker scan tidak tersedia: orchis-defense-agent, Hackbot Lite, Nmap, Nuclei" - nama internal'],
    [280, 645, 1120, 60, '2: copy arsitektur "container worker scan"'],
    [694, 500, 60, 55, '3: nilai kosong "—" (em dash)'],
    [1076, 585, 200, 20, '4: preset "standard" di KPI'],
  ] },
  { src: 'int-findings-drawer.jpg', out: 'mark-UI-020-drawer.png', boxes: [
    [1000, 545, 400, 130, '1: JSON mentah di "Rincian"'],
    [987, 155, 420, 100, '2: triage select native + tombol tak seragam'],
    [1090, 460, 40, 22, '3: "×3" tanpa keterangan'],
  ] },
  { src: 'err-organizations--light--id--1440.jpg', out: 'mark-UI-021-err500.png', boxes: [[430, 390, 580, 130, '1: halaman 500 hanya EN, tanpa tombol coba lagi']] },
  { src: 'int-login-error.jpg', out: 'mark-UI-022-loginhero.png', boxes: [
    [56, 570, 432, 120, '1: terminal palsu (pola slop)'],
    [533, 35, 172, 40, '2: pil "All systems operational" tanpa sumber'],
    [56, 745, 350, 70, '3: tagline marketing'],
  ] },
  { src: 'findings--light--id--1440.jpg', out: 'mark-UI-023-findings.png', boxes: [
    [280, 655, 1120, 55, '1: kontrol campur: select native + segmented custom'],
    [287, 838, 300, 30, '2: titik severity tanpa label di header grup'],
    [280, 465, 1120, 100, '3: kartu metrik terpisah + mono'],
  ] },
  { src: 'settings--light--id--1440.jpg', out: 'mark-UI-024-settings.png', boxes: [
    [300, 490, 300, 24, '1: tab horizontal (desain: sub-nav kiri flat)'],
    [280, 630, 1120, 90, '2: danger zone tak terpisah dari kartu lain'],
    [280, 555, 1120, 30, '3: "worker scan" jargon di notice'],
  ] },
];

const browser = await chromium.launch();
const page = await browser.newPage();
for (const job of JOBS) {
  const img = await readFile(path.join(DIR, job.src));
  const b64 = 'data:image/jpeg;base64,' + img.toString('base64');
  const boxes = job.boxes.map(([x, y, w, h, label], i) =>
    `<rect x="${x}" y="${y}" width="${w}" height="${h}" fill="none" stroke="#FF2D55" stroke-width="3"/><rect x="${x}" y="${Math.max(0, y - 26)}" width="${Math.min(w, 26 + label.length * 9)}" height="26" fill="#FF2D55"/><text x="${x + 8}" y="${Math.max(0, y - 26) + 19}" font-family="Inter,sans-serif" font-size="15" font-weight="700" fill="#fff">${i + 1}. ${label.replace(/&/g, '&amp;').replace(/</g, '&lt;')}</text>`).join('');
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="1440" height="900">${boxes}</svg>`;
  await page.setContent(`<body style="margin:0"><div style="position:relative;width:1440px;height:900px"><img src="${b64}" style="width:1440px;height:900px;display:block"/><div style="position:absolute;inset:0">${svg}</div></div></body>`);
  await page.screenshot({ path: path.join(DIR, job.out), clip: { x: 0, y: 0, width: 1440, height: 900 } });
  console.log('ok', job.out);
}
await browser.close();

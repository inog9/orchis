// Membandingkan ukuran elemen di browser dengan angka di design/orchis.pen (frame 4.1 dan 1.1).
import { chromium } from 'playwright';
import { readFileSync } from 'node:fs';
const pen = JSON.parse(readFileSync(new URL('../design/orchis.pen', import.meta.url), 'utf8'));
const find = (n, f) => (f(n) ? n : (n.children ?? []).map((c) => find(c, f)).find(Boolean));
const frame = (id) => pen.children.find((t) => t.id === id);
const named = (root, name) => find(root, (n) => n.name === name);
const A = frame('mV6Xk');
const want = {
  'topbar tinggi': [named(A, 'Top Bar').height, null],
  'judul halaman (px)': [named(A, 'Page Title').fontSize, null],
  'search lebar': [named(A, 'Search').width, null],
  'search tinggi': [named(A, 'Search').height, null],
  'list head tinggi': [named(A, 'List Head').height, null],
  'ikon aset': [named(A, 'Endpoint Icon').width, null],
  'padding kiri konten': [84, null],
};
console.log('desain:', JSON.stringify(Object.fromEntries(Object.entries(want).map(([k, [v]]) => [k, v]))));
const b = await chromium.launch();
const p = await b.newPage({ viewport: { width: 1440, height: 900 } });
await p.goto((process.env.E2E_BASE ?? 'http://127.0.0.1:8090') + '/login');
await p.getByLabel('Email').fill('admin@orchis.local');
await p.getByLabel('Password', { exact: true }).fill('Orchis-Dev-Pass-1');
await p.getByRole('button', { name: /sign in/i }).click();
await p.waitForURL('**/');
await p.goto((process.env.E2E_BASE ?? 'http://127.0.0.1:8090') + '/assets');
await p.waitForSelector('.page__title');
const got = await p.evaluate(() => {
  const w = (s, k = 'width') => { const e = document.querySelector(s); return e ? Math.round(e.getBoundingClientRect()[k]) : null; };
  const px = (s, prop) => { const e = document.querySelector(s); return e ? parseFloat(getComputedStyle(e)[prop]) : null; };
  return { 'topbar tinggi': w('.topbar', 'height'), 'judul halaman (px)': px('.page__title', 'fontSize'), 'search lebar': w('.search-input'), 'search tinggi': w('.search-input', 'height'), 'list head tinggi': w('.list__head', 'height'), 'padding kiri konten': px('.content', 'paddingLeft') };
});
let bad = 0;
for (const [k, [v]] of Object.entries(want)) {
  const g = got[k];
  if (g === undefined || g === null) { console.log(`  (lewati) ${k}: tidak ada di halaman kosong`); continue; }
  const same = Math.abs(g - v) <= 1; if (!same) bad++;
  console.log(`${same ? 'SAMA  ' : 'BEDA  '} ${k}: desain=${v} browser=${g}`);
}
await b.close(); process.exit(bad ? 1 : 0);

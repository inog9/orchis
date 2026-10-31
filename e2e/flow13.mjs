// E2E T2: data demo. DB kosong -> tanpa banner -> demo-seed (CLI) -> banner, organisasi/laporan berlabel -> axe -> purge dari UI -> bersih.
import { chromium } from 'playwright';
import AxeBuilder from '@axe-core/playwright';
import { execFileSync } from 'node:child_process';
import { mkdirSync } from 'node:fs';

const BASE = process.env.E2E_BASE ?? 'http://127.0.0.1:18090';
const SHOTS = new URL('./shots/demo/', import.meta.url).pathname;
mkdirSync(SHOTS, { recursive: true });
let pass = 0, fail = 0;
const ok = (value, msg) => { value ? pass++ : fail++; console.log((value ? 'LULUS: ' : 'GAGAL: ') + msg); };
const server = (cmd, env = {}) => execFileSync('./bin/server', [cmd], { env: { ...process.env, ...env }, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] });
const demoJSON = (cmd, env) => JSON.parse(server(cmd, env).trim().split('\n').pop());

const browser = await chromium.launch();
const ctx = await browser.newContext({ viewport: { width: 1280, height: 800 } });
const p = await ctx.newPage();
const errors = [];
p.on('pageerror', (e) => errors.push(String(e)));
const api = async (method, path, data) => {
  const r = await ctx.request.fetch(BASE + '/api/v1' + path, { method, data, headers: { 'X-Requested-With': 'orchis-web' } });
  if (!r.ok()) throw new Error(`${method} ${path} -> ${r.status()} ${await r.text()}`);
  return r.json();
};
const banner = p.getByRole('status', { name: 'Demo data notice' });
const axe = async (label) => {
  for (const width of [1280, 834]) {
    await p.setViewportSize({ width, height: width === 834 ? 1112 : 800 });
    await p.waitForLoadState('networkidle');
    const res = await new AxeBuilder({ page: p }).analyze();
    ok(res.violations.length === 0, `axe nol temuan: ${label} @${width}` + (res.violations.length ? ' ' + res.violations.map((v) => v.id).join(',') : ''));
    const overflow = await p.evaluate(() => document.documentElement.scrollWidth > window.innerWidth + 1);
    ok(!overflow, `tanpa scroll horizontal: ${label} @${width}`);
  }
  await p.setViewportSize({ width: 1280, height: 800 });
};

try {
  await p.goto(BASE + '/login');
  await p.getByLabel('Email').fill('admin@orchis.local');
  await p.getByLabel('Password', { exact: true }).fill('Orchis-Dev-Pass-1');
  await p.getByRole('button', { name: /sign in/i }).click();
  await p.getByRole('heading', { name: 'Home', exact: true }).waitFor();

  ok((await api('GET', '/demo/status')).has_demo === false, 'DB baru: status demo kosong');
  ok(await banner.count() === 0, 'DB baru: tanpa banner demo');

  // Seed lewat CLI (satu-satunya jalur seed; tidak ada endpoint seed).
  const seeded = demoJSON('demo-seed', { DEMO_SEED: '2' });
  ok(seeded.has_demo && seeded.seed === 2 && seeded.counts.organizations === 5 && seeded.counts.scenarios === 3 && seeded.counts.alerts === 3, 'demo-seed membuat 5 organisasi, 3 skenario, 3 alert');
  let rejected = false;
  try { server('demo-seed', { DEMO_SEED: '3' }); } catch { rejected = true; }
  ok(rejected, 'seed berbeda tanpa purge ditolak CLI');
  ok(demoJSON('demo-seed', { DEMO_SEED: '2' }).counts.assets === 50, 'seed sama diulang tidak menduplikasi');

  await p.goto(BASE + '/');
  await banner.waitFor();
  await p.getByRole('heading', { name: 'Home', exact: true }).waitFor();
  await p.locator('.skeleton').first().waitFor({ state: 'hidden', timeout: 15000 }).catch(() => {});
  ok((await banner.innerText()).includes('synthetic demo data'), 'banner demo tampil dan menjelaskan data sintetis');
  await p.screenshot({ path: SHOTS + 'home-1280.png' });
  await axe('home dengan banner');

  await p.goto(BASE + '/organizations');
  await banner.waitFor();
  await p.getByText('(Demo)').first().waitFor({ timeout: 15000 }).catch(() => {});
  ok(await p.getByText('(Demo)').first().isVisible(), 'organisasi demo diberi tanda (Demo)');
  await p.screenshot({ path: SHOTS + 'organizations-1280.png' });
  await axe('organisasi');

  const reports = (await api('GET', '/reports?limit=50')).items;
  const ready = reports.find((r) => r.status === 'ready');
  const failed = reports.find((r) => r.status === 'failed');
  ok(reports.length === 15 && ready && failed, '15 laporan demo (tiga versi per organisasi) termasuk satu yang gagal');
  ok(reports.every((r) => r.snapshot.is_demo === true && /DEMO/.test(r.snapshot.label)), 'setiap snapshot laporan berlabel sintetis');
  const html = await (await ctx.request.get(`${BASE}/api/v1/reports/${ready.id}/export/html?preview=1`)).text();
  ok(/DEMO \/ SYNTHETIC/.test(html), 'ekspor HTML berlabel DEMO');
  const csv = await (await ctx.request.get(`${BASE}/api/v1/reports/${ready.id}/export/csv`)).text();
  ok(csv.split('\n')[0].endsWith('synthetic') && csv.split('\n').slice(1).filter(Boolean).every((l) => l.endsWith(',true')), 'ekspor CSV punya kolom synthetic=true');
  const json = await (await ctx.request.get(`${BASE}/api/v1/reports/${ready.id}/export/json`)).json();
  ok(json.is_demo === true && /DEMO/.test(json.label), 'ekspor JSON memuat is_demo dan label');

  await p.goto(BASE + `/reports/${ready.id}`);
  await banner.waitFor();
  await p.getByRole('heading', { level: 1 }).waitFor({ timeout: 15000 });
  await p.waitForLoadState('networkidle');
  await p.screenshot({ path: SHOTS + 'report-1280.png' });
  await axe('laporan demo');

  const scenarios = (await api('GET', '/scenarios?limit=50')).items;
  ok(scenarios.filter((s) => /^Demo:/.test(s.name)).length === 3, 'tiga skenario demo terlihat di API');
  await p.goto(BASE + '/scenarios');
  await p.waitForLoadState('networkidle');
  await p.screenshot({ path: SHOTS + 'scenarios-1280.png' });
  await axe('skenario');

  // Purge dari banner.
  await p.goto(BASE + '/');
  await banner.waitFor();
  await banner.getByRole('button', { name: 'Purge demo data' }).click();
  const dlg = p.getByRole('dialog');
  await dlg.waitFor();
  await p.screenshot({ path: SHOTS + 'purge-confirm-1280.png' });
  await dlg.getByRole('button', { name: 'Cancel' }).click();
  ok((await api('GET', '/demo/status')).has_demo === true, 'batal pada konfirmasi tidak menghapus apa pun');
  await banner.getByRole('button', { name: 'Purge demo data' }).click();
  await p.getByRole('dialog').getByRole('button', { name: 'Purge demo data' }).click();
  await banner.waitFor({ state: 'detached', timeout: 20000 });
  const after = await api('GET', '/demo/status');
  ok(after.has_demo === false && Object.values(after.counts).every((n) => n === 0), 'purge dari UI menghapus semua data demo');
  ok((await api('GET', '/organizations?limit=50')).items.length === 0, 'tidak ada organisasi tersisa setelah purge');

  // Setelah purge seed berbeda diperbolehkan lagi, lalu bersihkan lewat CLI.
  ok(demoJSON('demo-seed', { DEMO_SEED: '3' }).seed === 3, 'setelah purge, seed berbeda diterima');
  ok(demoJSON('demo-purge').has_demo === false, 'demo-purge CLI membersihkan');
  ok(errors.length === 0, 'tanpa galat halaman' + (errors.length ? ': ' + errors.join(' | ') : ''));
} catch (e) {
  fail++;
  console.log('GAGAL: pengecualian: ' + (e?.stack ?? e));
} finally {
  await browser.close();
}
console.log(`\nHasil flow13: ${pass} lulus, ${fail} gagal`);
process.exit(fail ? 1 : 0);

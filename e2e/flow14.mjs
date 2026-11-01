// E2E T3: halaman MITRE dengan bundel Enterprise ATT&CK penuh (berkas beku + hash), tiga tingkat cakupan, filter, detail, axe.
import { chromium } from 'playwright';
import AxeBuilder from '@axe-core/playwright';
import { execFileSync } from 'node:child_process';
import { mkdirSync } from 'node:fs';

const BASE = process.env.E2E_BASE ?? 'http://127.0.0.1:18090';
const PORT = process.env.E2E_TARGET_PORT ?? '18081';
const SHOTS = new URL('./shots/mitre/', import.meta.url).pathname;
mkdirSync(SHOTS, { recursive: true });
let pass = 0, fail = 0;
const ok = (value, msg) => { value ? pass++ : fail++; console.log((value ? 'LULUS: ' : 'GAGAL: ') + msg); };
const WANT = ['reconnaissance', 'resource-development', 'initial-access', 'execution', 'persistence', 'privilege-escalation', 'stealth', 'defense-impairment', 'credential-access', 'discovery', 'lateral-movement', 'collection', 'command-and-control', 'exfiltration', 'impact'];

const browser = await chromium.launch();
const ctx = await browser.newContext({ viewport: { width: 1280, height: 800 } });
const p = await ctx.newPage();
const errors = [];
p.on('pageerror', (e) => errors.push(String(e)));
const api = async (method, path, data) => {
  const r = await ctx.request.fetch(BASE + '/api/v1' + path, { method, data, headers: { 'X-Requested-With': 'orchis-web' } });
  if (!r.ok() && r.status() !== 202) throw new Error(`${method} ${path} -> ${r.status()} ${await r.text()}`);
  return r.status() === 202 || r.status() === 204 ? null : r.json();
};
const axe = async (label) => {
  for (const width of [1280, 834]) {
    await p.setViewportSize({ width, height: width === 834 ? 1112 : 800 });
    await p.waitForLoadState('networkidle');
    const res = await new AxeBuilder({ page: p }).analyze();
    ok(res.violations.length === 0, `axe nol temuan: ${label} @${width}` + (res.violations.length ? ' ' + res.violations.map((v) => v.id).join(',') : ''));
    ok(!(await p.evaluate(() => document.documentElement.scrollWidth > window.innerWidth + 1)), `tanpa scroll horizontal halaman: ${label} @${width}`);
    const details = p.getByRole('region', { name: 'Technique details' });
    if (await details.count()) {
      const title = await details.getByRole('heading', { level: 2 }).boundingBox();
      const close = await details.getByRole('button', { name: 'Close details' }).boundingBox();
      const description = await details.getByRole('region', { name: 'Technique description' }).boundingBox();
      const height = p.viewportSize().height;
      ok(title && title.y >= 0 && title.y + title.height <= height, `judul detail terlihat dalam viewport @${width}`);
      ok(close && close.y >= 0 && close.y + close.height <= height, `tombol tutup detail terlihat dalam viewport @${width}`);
      ok(description && description.height >= 80 && description.y >= 0 && description.y + description.height <= height, `deskripsi teknik tidak menyusut atau keluar viewport @${width}`);
    }
  }
  await p.setViewportSize({ width: 1280, height: 800 });
};

try {
  await p.goto(BASE + '/login');
  await p.getByLabel('Email').fill('admin@orchis.local');
  await p.getByLabel('Password', { exact: true }).fill('Orchis-Dev-Pass-1');
  await p.getByRole('button', { name: /sign in/i }).click();
  await p.getByRole('heading', { name: 'Home', exact: true }).waitFor();

  await api('POST', '/mitre/import');
  const started = Date.now();
  let m;
  for (;;) {
    m = await api('GET', '/mitre/matrix');
    if (m.imported && m.last_run?.status === 'succeeded') break;
    if (m.last_run?.status === 'failed') throw new Error('impor gagal: ' + m.last_run.error);
    if (Date.now() - started > 120000) throw new Error('impor melebihi 120 dtk');
    await new Promise((r) => setTimeout(r, 1000));
  }
  console.log(`info: impor selesai dalam ${Math.round((Date.now() - started) / 1000)} dtk`);
  ok(m.tactics.map((t) => t.shortname).join(',') === WANT.join(','), 'kolom matriks = 15 taktik dalam urutan resmi (stealth, defense-impairment ...)');
  ok(m.total === 227, `227 teknik induk aktif (dapat ${m.total})`);
  ok(m.mapped === 8 && m.exercised === 0 && (m.simulated ?? 0) === 0, `awal: mapped=8 trained=0 simulated=0 (dapat ${m.mapped}/${m.exercised}/${m.simulated})`);
  ok(m.last_run.source_version === '20.0' && m.last_run.source_sha256?.length === 64, 'versi ATT&CK 20.0 dan hash sumber tercatat');
  const seen = new Map();
  for (const t of m.tactics) for (const c of t.techniques) seen.set(c.id, (seen.get(c.id) ?? 0) + 1);
  ok([...seen.values()].some((n) => n > 1), 'teknik yang berada di beberapa taktik tampil di setiap kolomnya');
  ok(!seen.has('T1086'), 'teknik revoked (T1086 PowerShell lama) tidak muncul');

  const t0 = Date.now();
  await p.goto(BASE + '/mitre');
  await p.getByRole('heading', { name: 'MITRE ATT&CK', exact: true }).waitFor();
  await p.getByRole('button', { name: /^T1046 / }).waitFor({ timeout: 30000 });
  console.log(`info: matriks penuh tampil dalam ${Date.now() - t0} ms`);
  ok(Date.now() - t0 < 15000, 'matriks penuh tampil dalam 15 dtk');
  ok(await p.locator('section.mcol').count() === 15, 'UI menampilkan 15 kolom taktik');
  await p.getByText('What do the colours mean?').click();
  ok(await p.getByText(/never counts as trained/).first().isVisible(), 'legenda menjelaskan tiga tingkat cakupan');
  await p.screenshot({ path: SHOTS + 'matrix-1280.png' });
  await axe('matriks penuh');

  // Pencarian dan filter platform.
  await p.getByRole('searchbox', { name: 'Search' }).fill('system information');
  ok(await p.getByRole('button', { name: /^T1082 / }).isVisible() && await p.getByRole('button', { name: /^T1046 / }).count() === 0, 'pencarian "system information" menyisakan T1082 (dan bukan T1046)');
  await p.getByRole('searchbox', { name: 'Search' }).fill('');
  await p.getByLabel('Platform').selectOption('Windows');
  const win = await p.locator('button.mcell').count();
  await p.getByLabel('Platform').selectOption('');
  const all = await p.locator('button.mcell').count();
  ok(win > 0 && win < all, `filter platform Windows mengurangi sel (${win} dari ${all})`);
  const onlyMapped = p.locator('label.check', { hasText: 'Only mapped techniques' });
  await onlyMapped.click();
  ok(await p.locator('button.mcell').count() >= 4 && await p.locator('button.mcell').count() < 30, 'filter "only mapped" menyisakan teknik yang dipetakan saja');
  await onlyMapped.click();

  // Detail teknik: pemetaan beralasan, mitigasi, sub-teknik.
  await p.getByRole('button', { name: /^T1046 / }).first().click();
  const panel = p.getByRole('region', { name: 'Technique details' });
  await panel.getByText(/enumerates open ports/).waitFor();
  ok(await panel.getByText('nmap', { exact: false }).first().isVisible() || await panel.getByText(/Nmap/i).first().isVisible(), 'detail T1046 menyebut aksi Nmap sebagai pemetaan');
  ok(await panel.getByText(/Mitigations \(/).isVisible(), 'detail T1046 memuat mitigasi resmi');
  ok(await panel.getByRole('link', { name: /View on MITRE/ }).getAttribute('href') === 'https://attack.mitre.org/techniques/T1046', 'tautan sumber resmi benar');
  await p.screenshot({ path: SHOTS + 'detail-T1046-1280.png' });
  await axe('detail teknik');
  await p.getByRole('button', { name: /^T1595 / }).first().click();
  await panel.getByText(/Sub-techniques \(3\)/).waitFor();
  await panel.getByRole('button', { name: 'T1595.002' }).click();
  await panel.getByText(/Sub-technique of/).waitFor();
  ok(await panel.getByText(/Vulnerability Scanning/).first().isVisible(), 'sub-teknik T1595.002 bisa dibuka dari induknya');

  // Tingkat simulated: demo menambah tanpa pernah menjadi trained.
  const seeded = JSON.parse(execFileSync('./bin/server', ['demo-seed'], { env: { ...process.env, DEMO_SEED: '1' }, encoding: 'utf8' }).trim().split('\n').pop());
  ok(seeded.has_demo, 'demo-seed berhasil');
  await p.goto(BASE + '/mitre');
  await p.getByRole('button', { name: /^T1190 / }).waitFor({ timeout: 30000 });
  ok((await p.getByRole('button', { name: /^T1190 / }).getAttribute('aria-label')).includes('simulated'), 'T1190 ditandai simulated (hanya dari data demo)');
  const m2 = await api('GET', '/mitre/matrix');
  ok(m2.simulated > 0 && m2.exercised === 0 && m2.mapped === 8, `setelah demo: simulated=${m2.simulated} trained=0 mapped=8`);
  const cellFor = (matrix, id) => matrix.tactics.flatMap((t) => t.techniques).find((c) => c.id === id);
  const simulatedDiscovery = cellFor(m2, 'T1046');
  ok(simulatedDiscovery.mapped && simulatedDiscovery.simulated && simulatedDiscovery.simulated_runs > 0 && simulatedDiscovery.runs === 0, 'T1046 menampilkan mapped dan simulated tanpa menambah run nyata');
  const discoveryButton = p.getByRole('button', { name: /^T1046 / }).first();
  ok((await discoveryButton.getAttribute('aria-label')).includes('simulated') && await discoveryButton.getByText('Demo', { exact: true }).isVisible(), 'badge Demo tetap terlihat pada teknik mapped');
  await p.screenshot({ path: SHOTS + 'matrix-demo-1280.png' });
  await axe('matriks dengan data demo');

  // Satu run Nmap sungguhan ke fixture lokal membuktikan irisan tiga status.
  const asset = await api('POST', '/assets', { category: 'host', name: 'LOCAL MITRE E2E FIXTURE', testing_authorized: true, addresses: [{ kind: 'ip', value: '127.0.0.1' }] });
  const test = await api('POST', '/atomic-tests', { name: 'LOCAL real MITRE coverage', contract_key: 'nmap.tcp_connect', target_asset_id: asset.id, params: { profile: 'custom', ports: PORT } });
  await api('POST', `/atomic-tests/${test.id}/run`, {});
  const deadline = Date.now() + 60000;
  let finished;
  do {
    finished = await api('GET', `/atomic-tests/${test.id}`);
    if (finished.last_execution?.state === 'ended') break;
    await new Promise((resolve) => setTimeout(resolve, 500));
  } while (Date.now() < deadline);
  ok(finished.last_execution?.outcome === 'success', 'Nmap nyata menyelesaikan satu pemeriksaan target lokal');
  if (finished.last_execution?.outcome !== 'success') throw new Error('pemeriksaan Nmap lokal tidak selesai sukses');
  const executed = await api('GET', `/executions/${finished.last_execution.id}`);
  ok(executed.findings.some((finding) => finding.type === 'open_port' && finding.value === `tcp/${PORT}`), 'hasil nyata memuat port fixture yang terbuka');
  const m3 = await api('GET', '/mitre/matrix');
  const trainedDiscovery = cellFor(m3, 'T1046');
  ok(trainedDiscovery.mapped && trainedDiscovery.exercised && trainedDiscovery.simulated && trainedDiscovery.runs === 1 && trainedDiscovery.simulated_runs === simulatedDiscovery.simulated_runs, 'mapped/trained/simulated beririsan; hanya satu run nyata dihitung trained');
  await p.goto(BASE + '/mitre');
  const trainedButton = p.getByRole('button', { name: /^T1046 .*trained:.*simulated:/ }).first();
  await trainedButton.click();
  await panel.getByText('1 successful run', { exact: true }).waitFor();
  ok(await trainedButton.getByText('Demo', { exact: true }).isVisible() && await panel.getByText('Demo / synthetic evidence:', { exact: true }).isVisible(), 'detail dan badge membedakan bukti nyata dari data sintetis');
  const detail = await api('GET', '/mitre/techniques/T1046');
  ok(detail.runs === 1 && detail.simulated && detail.simulated_runs === simulatedDiscovery.simulated_runs, 'API detail dan matriks menghitung bukti yang sama');
  await p.waitForLoadState('networkidle');
  await p.screenshot({ path: SHOTS + 'overlap-1280.png' });
  await p.setViewportSize({ width: 834, height: 1112 });
  await p.waitForLoadState('networkidle');
  await p.screenshot({ path: SHOTS + 'overlap-834.png' });
  await axe('bukti nyata dan sintetis beririsan');
  ok(errors.length === 0, 'tanpa galat halaman' + (errors.length ? ': ' + errors.join(' | ') : ''));
} catch (e) {
  fail++;
  console.log('GAGAL: pengecualian: ' + (e?.stack ?? e));
} finally {
  await browser.close();
}
console.log(`\nHasil flow14: ${pass} lulus, ${fail} gagal`);
process.exit(fail ? 1 : 0);

// E2E Fase 3: atomic test Nmap dan Nuclei lewat RabbitMQ dan injector (container), finding, Elasticsearch, hentikan scan.
import { chromium } from 'playwright';
import { execSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { mkdirSync } from 'node:fs';
const SHOTS = fileURLToPath(new URL('./shots/', import.meta.url));
mkdirSync(SHOTS, { recursive: true });
const BASE = process.env.E2E_BASE ?? 'http://127.0.0.1:8090';
const TARGET_PORT = process.env.E2E_TARGET_PORT ?? '18081';
let pass = 0, fail = 0;
const ok = (c, m) => { (c ? pass++ : fail++); console.log((c ? 'LULUS: ' : 'GAGAL: ') + m); };
const b = await chromium.launch();
const ctx = await b.newContext({ viewport: { width: 1440, height: 900 } });
const p = await ctx.newPage();
const errors = [];
const expected = /Failed to load resource: the server responded with a status of (401|404|409|422)/;
p.on('pageerror', (e) => errors.push('pageerror: ' + e.message));
p.on('console', (m) => m.type() === 'error' && !expected.test(m.text()) && errors.push('console: ' + m.text()));
const api = (method, path, data) => ctx.request.fetch(BASE + path, { method, data, headers: { 'X-Requested-With': 'orchis-web' } });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const sh = (c) => { try { return execSync(c, { encoding: 'utf8' }); } catch { return ''; } };

await p.goto(BASE + '/login');
await p.getByLabel('Email').fill('admin@orchis.local');
await p.getByLabel('Password', { exact: true }).fill('Orchis-Dev-Pass-1');
await p.getByRole('button', { name: /sign in/i }).click();
await p.waitForURL(BASE+'/overview');await p.goto(BASE+'/organizations');

await p.goto(BASE+'/overview');
// Overview describes the current empty workspace without inventing scan coverage.
await p.getByRole('heading',{name:'Overview',exact:true}).waitFor();
ok(await p.getByRole('link',{name:'Add your first asset'}).isVisible(),'empty Overview offers asset setup');
ok(await p.getByText('No critical findings awaiting triage.',{exact:true}).isVisible(),'empty triage queue does not claim safety');
await p.screenshot({ path: SHOTS + 'home-empty.png' });

// 1. navigasi aktif + keadaan kosong + injector terhubung
await p.goto(BASE+'/atomic');
// 2. aset sasaran: belum diotorisasi -> tidak boleh dipilih
const created = await (await api('POST', '/api/v1/assets', { category: 'host', name: 'e2e-target', criticality: 'high',
  addresses: [{ kind: 'ip', value: '127.0.0.1' }, { kind: 'url', value: `http://127.0.0.1:${TARGET_PORT}` }] })).json();
const blackhole = await (await api('POST', '/api/v1/assets', { category: 'host', name: 'e2e-blackhole', testing_authorized: true, addresses: [{ kind: 'ip', value: '192.0.2.1' }] })).json();
await p.getByRole('button', { name: /create the first test/i }).click();
let dlg = p.getByRole('dialog');
await dlg.getByLabel('Name').fill('Open ports of e2e-target');
await dlg.getByLabel('Target asset').selectOption({ label: 'e2e-blackhole' }).catch(() => {});
const options = await dlg.getByLabel('Target asset').locator('option').allTextContents();
ok(options.includes('e2e-blackhole') && !options.includes('e2e-target'), 'dialog hanya menawarkan aset yang diotorisasi: ' + options.join(' | '));
await dlg.getByRole('button', { name: 'Cancel' }).click();
await dlg.waitFor({ state: 'detached' });

// 3. admin mengotorisasi, lalu membuat test nmap dengan port khusus
let cur = await (await api('GET', `/api/v1/assets/${created.id}`)).json();
const authz = await api('PATCH', `/api/v1/assets/${created.id}`, { version: cur.version, testing_authorized: true });
ok(authz.status() === 200, 'admin mengotorisasi aset sasaran');
await p.getByRole('button', { name: /new single test/i }).or(p.getByRole('button', { name: /create the first test/i })).first().click();
dlg = p.getByRole('dialog');
await dlg.getByLabel('Name').fill('Open ports of e2e-target');
await dlg.getByLabel('Target asset').selectOption({ label: 'e2e-target' });
await dlg.getByLabel('Ports').selectOption('custom');
await dlg.getByLabel('Port list').fill('22; id');
await dlg.getByRole('button', { name: 'Create test' }).click();
await dlg.getByText(/use ports like 22,80/i).waitFor({ timeout: 5000 }).catch(() => {});
ok(await dlg.getByText(/use ports like 22,80/i).isVisible(), 'port list berbahaya ditolak server dengan pesan jelas');
await dlg.getByLabel('Port list').fill(TARGET_PORT);
await dlg.getByLabel(/detect services/i).check();
await dlg.getByRole('button', { name: 'Create test' }).click();
await p.waitForURL(/\/atomic\/[0-9a-f-]{36}$/);
await p.getByRole('heading', { name: 'Open ports of e2e-target' }).waitFor();
ok(await p.getByText(/never been run|has not run yet/i).isVisible(), 'detail test baru: belum pernah dijalankan');

// 4. jalankan: queued/running -> succeeded, finding tampil
await p.getByRole('button', { name: /^run$/i }).click();
await p.locator('.panel .status').filter({ hasText: /Succeeded/ }).first().waitFor({ timeout: 60000 });
const portTitle = p.getByText(`Open port ${TARGET_PORT}/tcp`, { exact: false }).first();
await portTitle.waitFor({ timeout: 10000 });
ok(await portTitle.isVisible(), `finding "Open port ${TARGET_PORT}/tcp" tampil di hasil run`);
ok(await p.getByText(/started by the nmap injector/i).isVisible(), 'log menunjukkan injector nmap yang menjalankan');
ok(await p.getByRole('button', { name: /^run$/i }).isEnabled(), 'tombol Run kembali aktif setelah selesai');
await p.screenshot({ path: SHOTS + 'atomic-nmap-result.png' });

// 5. halaman Findings + pencarian lewat Elasticsearch
await p.getByRole('link', { name: 'Findings' }).click();
await p.waitForURL('**/findings');
await p.getByRole('button',{name:'Critical',exact:true}).click();
const rowsLoc = p.getByRole('list', { name: 'Findings' }).getByRole('listitem');
await rowsLoc.first().waitFor();
await rowsLoc.first().waitFor({state:'visible'});
ok((await rowsLoc.count()) >= 1, 'halaman Findings menampilkan temuan');
let esOk = false;
for (let i = 0; i < 30 && !esOk; i++) {
  const r = await (await api('GET', '/api/v1/findings?q=e2e-target')).json();
  esOk = r.search === 'elasticsearch' && r.items.length >= 1;
  if (!esOk) await sleep(1000);
}
ok(esOk, 'pencarian teks dijawab Elasticsearch (bukan fallback database)');
await p.getByLabel('Search findings').fill('e2e-target');
await p.getByRole('list', { name: 'Findings' }).getByText(`tcp/${TARGET_PORT}`).first().waitFor();
ok(true, 'mencari di UI menemukan finding berdasarkan nama aset');
await p.getByLabel('Search findings').fill('zzz-no-such-thing');
await p.getByRole('heading', { name: 'No findings match' }).waitFor();
ok(true, 'pencarian tanpa hasil menampilkan keadaan kosong yang jelas');
await p.getByLabel('Search findings').fill('');

// 6. Nuclei: template e2e menemukan /.env bocor
await p.goto(BASE + '/atomic');
await p.getByRole('button', { name: /new single test/i }).click();
dlg = p.getByRole('dialog');
await dlg.getByLabel('Name').fill('Web exposure of e2e-target');
await dlg.getByLabel('Action').selectOption('nuclei.scan');
await dlg.getByLabel('Target asset').selectOption({ label: 'e2e-target' });
ok(await dlg.getByRole('button', { name: 'High', exact: true }).getAttribute('aria-pressed') === 'true', 'severity bawaan Nuclei: medium, high, critical');
await dlg.getByRole('button', { name: 'Create test' }).click();
await p.waitForURL(/\/atomic\/[0-9a-f-]{36}$/);
await p.getByRole('button', { name: /^run$/i }).click();
await p.locator('.panel .status').filter({ hasText: /Succeeded|Failed/ }).first().waitFor({ timeout: 120000 });
const vuln = p.getByText('E2E exposed .env file');
ok(await vuln.first().isVisible().catch(() => false), 'Nuclei menemukan "E2E exposed .env file" lewat injector container');
ok(await p.locator('.finds .sev--high').first().isVisible().catch(() => false), 'severity High tampil pada finding Nuclei');
await p.screenshot({ path: SHOTS + 'atomic-nuclei-result.png' });
ok(!(await p.content()).includes('hunter2'), 'isi .env (rahasia) tidak pernah sampai ke UI');

// 7. finding yang sama dilihat lagi: tetap satu baris, "Seen 2×"
await p.goto(BASE + '/atomic');
await p.getByRole('link', { name: 'Open ports of e2e-target' }).click();
await p.getByRole('button', { name: /^run$/i }).click();
await p.locator('.runs__item').nth(1).waitFor({ timeout: 10000 });
await p.waitForFunction(() => document.querySelectorAll('.runs__item .status--ok').length >= 2, null, { timeout: 60000 });
await p.getByRole('link', { name: 'Findings' }).click();
await p.getByRole('button',{name:'Critical',exact:true}).click();
const seenRow = p.getByRole('list', { name: 'Findings' }).getByRole('listitem').filter({ hasText: `tcp/${TARGET_PORT}` }).filter({ hasText: 'Open port' });
await seenRow.first().waitFor();
ok((await seenRow.count()) === 1 && /Seen 2×/.test(await seenRow.first().innerText()), 'menjalankan ulang tidak menggandakan finding: satu baris, "Seen 2×"');
await p.getByRole('button',{name:'High',exact:true}).click();
await p.getByText('E2E exposed .env file').first().waitFor();
ok((await p.getByRole('list', { name: 'Findings' }).getByRole('listitem').count()) === 1, 'filter severity High hanya menyisakan temuan Nuclei');
await p.screenshot({ path: SHOTS + 'findings.png' });

// 8. Stop: scan ke alamat yang tidak menjawab dihentikan dan prosesnya benar-benar mati di container
await p.goto(BASE + '/atomic');
await p.getByRole('button', { name: /new single test/i }).click();
dlg = p.getByRole('dialog');
await dlg.getByLabel('Name').fill('Slow scan');
await dlg.getByLabel('Target asset').selectOption({ label: 'e2e-blackhole' });
await dlg.getByLabel('Ports').selectOption('top1000');
await dlg.getByRole('button', { name: 'Create test' }).click();
await p.waitForURL(/\/atomic\/[0-9a-f-]{36}$/);
await p.getByRole('button', { name: /^run$/i }).click();
await p.getByRole('button', { name: /stop/i }).waitFor();
const nmapRunning = () => sh(`docker top ${process.env.E2E_NMAP_CONTAINER} 2>/dev/null | grep -c "[n]map -sT" || true`).trim();
let started = false;
for (let i = 0; i < 20 && !started; i++) { started = nmapRunning() === '1'; if (!started) await sleep(500); }
ok(started, 'proses nmap benar-benar berjalan di container injector');
await p.getByRole('button', { name: /stop/i }).click();
await p.locator('.panel .status').filter({ hasText: 'Canceled' }).first().waitFor({ timeout: 10000 });
ok(true, 'Stop langsung menandai run sebagai Canceled');
let gone = false;
for (let i = 0; i < 30 && !gone; i++) { gone = nmapRunning() === '0'; if (!gone) await sleep(1000); }
ok(gone, 'proses nmap dihentikan di container setelah dibatalkan (heartbeat memberi sinyal)');
await p.screenshot({ path: SHOTS + 'atomic-canceled.png' });
ok(await p.getByRole('button', { name: /^run$/i }).isVisible(), 'setelah dibatalkan, tombol Run tersedia lagi');

// The Overview uses the same findings and inventory as the detailed views.
await p.goto(BASE+'/overview');await p.getByRole('heading',{name:'Overview',exact:true}).waitFor();
const home=await (await api('GET','/api/v1/home')).json();
ok(home.assets.total===2,'Overview inventory contains both fixture assets');
ok(await p.getByRole('heading',{name:'Vulnerability landscape'}).isVisible(),'current severity summary displayed');
ok(await p.getByRole('heading',{name:'Testing authorisation'}).isVisible(),'authorisation is shown separately from audit results');
await p.screenshot({path:SHOTS+'home-filled.png'});

ok(errors.length === 0, 'tidak ada error konsol/halaman tak terduga' + (errors.length ? ': ' + errors.join(' | ') : ''));
await b.close();
console.log(`\n${pass} lulus, ${fail} gagal`);
process.exit(fail ? 1 : 0);

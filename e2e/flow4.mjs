// E2E Fase 5-6: scenario, workspace chain, peluncuran simulasi, trigger/join/skip, attack path, jeda/lanjut, hentikan.
import { chromium } from 'playwright';
import { execSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { mkdirSync } from 'node:fs';
const SHOTS = fileURLToPath(new URL('./shots/', import.meta.url));
mkdirSync(SHOTS, { recursive: true });
const BASE = process.env.E2E_BASE ?? 'http://127.0.0.1:8090';
const PORT = process.env.E2E_TARGET_PORT ?? '18081';
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
const card = (title) => p.getByRole('button', { name: `Action ${title}`, exact: true });

await p.goto(BASE + '/login');
await p.getByLabel('Email').fill('admin@orchis.local');
await p.getByLabel('Password', { exact: true }).fill('Orchis-Dev-Pass-1');
await p.getByRole('button', { name: /sign in/i }).click();
await p.waitForURL(BASE+'/overview');await p.goto(BASE+'/organizations');

// aset sasaran: BELUM diotorisasi
const target = await (await api('POST', '/api/v1/assets', { category: 'host', name: 'e2e-target', criticality: 'high',
  addresses: [{ kind: 'ip', value: '127.0.0.1' }, { kind: 'url', value: `http://127.0.0.1:${PORT}` }] })).json();

// Advanced legacy engine fixture; Scenario route/create UI has been removed.
const createdScenario = await (await api('POST','/api/v1/scenarios',{name:'E2E kill chain',severity:'high'})).json();
const sid=createdScenario.id;
const scenarioUrl=BASE+`/chains/${sid}/setup`;
let dlg;
await p.goto(scenarioUrl);
await p.getByRole('heading', { name: 'E2E kill chain' }).waitFor();
ok(await p.getByText('Not ready to launch yet.').isVisible(), 'scenario baru berstatus Draft dengan alasan belum siap');
ok(await p.getByRole('button', { name: /launch simulation/i }).isDisabled(), 'tombol Launch nonaktif selama belum siap');

// 2. pilih target (belum diotorisasi): kesiapan menyebutnya; setelah diotorisasi masalah itu hilang
await p.getByRole('button', { name: 'Edit targets' }).click();
dlg = p.getByRole('dialog');
await dlg.getByLabel(/e2e-target/).check();
ok(await dlg.getByText('Not authorized for testing yet').isVisible(), 'dialog target menandai aset yang belum diotorisasi');
await dlg.getByRole('button', { name: 'Save targets' }).click();
await dlg.waitFor({ state: 'detached' });
await p.getByText('Asset e2e-target is not authorized for testing.').waitFor();
ok(true, 'kesiapan menyebut aset sasaran yang belum diotorisasi');
let cur = await (await api('GET', `/api/v1/assets/${target.id}`)).json();
await api('PATCH', `/api/v1/assets/${target.id}`, { version: cur.version, testing_authorized: true });

// 3. workspace chain: bangun graf lewat UI
await p.getByRole('link', { name: 'Open chain workspace' }).click();
await p.waitForURL(/\/chains\/[0-9a-f-]{36}$/);
await p.getByRole('heading', { name: 'This chain is empty' }).waitFor();
await p.getByRole('button', { name: 'Add the first action' }).click();
await p.getByLabel('Title').fill('Port scan');
await p.getByLabel('Ports', { exact: true }).selectOption('custom');
await p.getByLabel('Port list').fill('22; id');
await p.getByRole('button', { name: 'Save chain' }).click();
await p.getByText('Fix these before saving').waitFor();
ok(await p.getByText(/use ports like 22,80/i).isVisible(), 'port berbahaya ditolak server dan ditampilkan di workspace');
await p.getByLabel('Port list').fill(PORT);

await p.getByRole('button', { name: 'Add action' }).click();
await p.getByLabel('Title').fill('Web exposure');
await p.getByLabel('Action', { exact: true }).selectOption('nuclei.scan');
await p.getByLabel('Starts').selectOption('on_finding');
await p.getByLabel('Finding type').selectOption('open_port');
await p.getByLabel('Only these values (optional)').fill(`tcp/${PORT}`);
await p.getByLabel('Port scan', { exact: true }).check(); // induk
await p.getByRole('button', { name: 'Add action' }).click();
await p.getByLabel('Title').fill('SSH check');
await p.getByLabel('Action', { exact: true }).selectOption('nuclei.scan');
await p.getByLabel('Starts').selectOption('on_finding');
await p.getByLabel('Only these values (optional)').fill('tcp/9');
await p.getByLabel('Port scan', { exact: true }).check();
await p.getByRole('button', { name: 'Add action' }).click();
await p.getByLabel('Title').fill('Follow-up');
await p.getByLabel('SSH check', { exact: true }).check();
ok((await p.getByText('Unsaved changes').count()) === 1, 'perubahan belum disimpan ditandai');

// pencegahan siklus: induk "Port scan" tidak boleh bergantung pada "Follow-up"
await card('Port scan').click();
const loopOpt = p.getByLabel(/^Follow-up/);
ok(await loopOpt.isDisabled(), 'memilih induk yang membentuk siklus dinonaktifkan di UI');
await p.getByRole('button', { name: 'Save chain' }).click();
await p.getByText('Unsaved changes').waitFor({ state: 'detached' });
ok(true, 'chain 4 aksi tersimpan');
await p.screenshot({ path: SHOTS + 'chain-workspace.png' });
// muat ulang: tersimpan di server dengan tata letak berlapis
await p.reload();
await card('Follow-up').waitFor();
const xs = {};
for (const t of ['Port scan', 'Web exposure', 'SSH check', 'Follow-up']) xs[t] = (await card(t).boundingBox()).x;
ok(xs['Web exposure'] > xs['Port scan'] && xs['Follow-up'] > xs['SSH check'] && xs['SSH check'] > xs['Port scan'], 'tata letak berlapis: anak selalu di kanan induknya');

// 4. scenario kini siap; luncurkan
await p.goto(scenarioUrl);
await p.getByText('Ready', { exact: true }).first().waitFor();
ok(await p.getByRole('button', { name: /launch simulation/i }).isEnabled(), 'scenario siap: tombol Launch aktif');
ok((await p.locator('.steps-flow li').count()) === 4, 'ringkasan scenario menampilkan 4 aksi');
await p.getByRole('button', { name: /launch simulation/i }).click();
await p.getByRole('dialog').getByRole('button', { name: 'Launch simulation' }).click();
await p.waitForURL(/\/simulations\/legacy\/[0-9a-f-]{36}$/);
const simUrl = p.url();
await p.getByRole('heading', { name: 'E2E kill chain', exact: true, level: 1 }).waitFor();
await p.locator('.page__sub').filter({ hasText: /OP-\d{4}/ }).waitFor({ timeout: 15000 }).catch(() => {});
ok(/OP-\d{4}/.test(await p.locator('.page__sub').innerText()), 'simulasi punya kode OP-nnnn');
await p.locator('.page__title-row .status').filter({ hasText: /Finished/ }).waitFor({ timeout: 240000 });
ok(true, 'simulasi berjalan sampai Finished lewat RabbitMQ dan dua container injector');
const stepRow = (t) => p.getByRole('row').filter({ hasText: t });
ok(await stepRow('Port scan').getByText('Done', { exact: true }).isVisible(), 'langkah awal: Done');
ok(await stepRow('Web exposure').getByText('Done', { exact: true }).isVisible(), 'trigger on-finding (tcp/port) memicu Web exposure: Done');
ok(await stepRow('SSH check').getByText('Skipped', { exact: true }).isVisible(), 'aturan tak terpenuhi (tcp/9): SSH check Skipped');
ok(await stepRow('Follow-up').getByText('Blocked', { exact: true }).isVisible(), 'anak dari langkah yang dilewati: Blocked');
ok(/100%/.test(await p.getByRole('progressbar').locator('xpath=preceding-sibling::div[contains(@class,"kpis")]').innerText()), 'progres 100% saat semua langkah berstatus akhir');
await p.screenshot({ path: SHOTS + 'simulation-overview.png' });

// 5. temuan dan attack path
await p.getByRole('tab', { name: /Findings/ }).click();
await p.getByRole('list', { name: 'Simulation findings' }).waitFor();
const fl = p.getByRole('list', { name: 'Simulation findings' });
ok((await fl.getByText(`Open port ${PORT}/tcp`, { exact: false }).count()) >= 1 && (await fl.getByText('E2E exposed .env file').count()) === 1, 'tab Findings: port terbuka dan kerentanan Nuclei milik simulasi ini');
await p.getByRole('tab', { name: 'Attack path' }).click();
const graph = p.getByRole('group', { name: /Attack path with/ });
await graph.waitFor();
const label = await graph.getAttribute('aria-label');
const nodes = Number(/with (\d+) nodes/.exec(label)[1]), conns = Number(/and (\d+) connections/.exec(label)[1]);
ok(nodes === 7 && conns === 6, `attack path: ${nodes} simpul (4 aksi, 1 aset, 2 temuan) dan ${conns} koneksi (sasaran 1, temuan 2, pemicu 1, dependensi 2) tanpa garis redundan`);
ok(await graph.getByText('E2E exposed .env file').isVisible() && await graph.getByText('Web exposure').isVisible(), 'attack path menampilkan temuan dan aksi lanjutan yang dipicunya');
ok((await graph.locator('.pathedge--triggers').count()) >= 1, 'ada tepi "temuan memicu aksi" pada graf');
await p.screenshot({ path: SHOTS + 'attack-path.png' });

// 6. jeda dan lanjut: aksi awal ditunda 20 dtk supaya sempat dijeda
const delayed = await (await api('POST', '/api/v1/scenarios', { name: 'E2E delayed', severity: 'low' })).json();
await api('PUT', `/api/v1/scenarios/${delayed.id}/targets`, { version: delayed.version, asset_ids: [target.id], group_ids: [] });
const d2 = await (await api('GET', `/api/v1/scenarios/${delayed.id}`)).json();
const gr = await api('PUT', `/api/v1/scenarios/${delayed.id}/graph`, { version: d2.version, edges: [], steps: [
  { key: 'scan', title: 'Delayed scan', contract_key: 'nmap.tcp_connect', trigger_mode: 'after_parents', is_enabled: true, delay_seconds: 20, params: { profile: 'custom', ports: PORT } }] });
ok(gr.status() === 200, 'scenario dengan jeda 20 detik dibuat');
await p.goto(BASE + `/chains/${delayed.id}/setup`);
await p.getByRole('button', { name: /launch simulation/i }).click();
await p.getByRole('dialog').getByRole('button', { name: 'Launch simulation' }).click();
await p.waitForURL(/\/simulations\/legacy\/[0-9a-f-]{36}$/);
await p.getByRole('button', { name: 'Pause' }).click();
await p.locator('.page__title-row .status').filter({ hasText: 'Paused' }).waitFor();
ok(await p.getByRole('button', { name: 'Resume' }).isVisible(), 'jeda: tombol berganti ke Resume');
await sleep(24000); // melewati waktu tunggu: pesan sampai ke injector saat dijeda
ok(await p.getByRole('row').filter({ hasText: 'Delayed scan' }).getByText('Queued', { exact: true }).isVisible(), 'selama dijeda langkah tidak berjalan (tetap Queued)');
await p.getByRole('button', { name: 'Resume' }).click();
await p.locator('.page__title-row .status').filter({ hasText: /Finished/ }).waitFor({ timeout: 90000 });
ok(true, 'setelah Resume simulasi berjalan sampai Finished (dispatch diterbitkan ulang)');

// 7. hentikan: scan ke alamat yang tidak menjawab
const hole = await (await api('POST', '/api/v1/assets', { category: 'host', name: 'e2e-blackhole', testing_authorized: true, addresses: [{ kind: 'ip', value: '192.0.2.1' }] })).json();
const slow = await (await api('POST', '/api/v1/scenarios', { name: 'E2E slow', severity: 'low' })).json();
await api('PUT', `/api/v1/scenarios/${slow.id}/targets`, { version: slow.version, asset_ids: [hole.id], group_ids: [] });
const s2 = await (await api('GET', `/api/v1/scenarios/${slow.id}`)).json();
await api('PUT', `/api/v1/scenarios/${slow.id}/graph`, { version: s2.version, edges: [{ parent: 'scan', child: 'after' }], steps: [
  { key: 'scan', title: 'Slow scan', contract_key: 'nmap.tcp_connect', trigger_mode: 'after_parents', is_enabled: true, delay_seconds: 0, params: { profile: 'top1000' } },
  { key: 'after', title: 'After slow', contract_key: 'nmap.tcp_connect', trigger_mode: 'after_parents', is_enabled: true, delay_seconds: 0, params: {} }] });
await p.goto(BASE + `/chains/${slow.id}/setup`);
await p.getByRole('button', { name: /launch simulation/i }).click();
await p.getByRole('dialog').getByRole('button', { name: 'Launch simulation' }).click();
await p.waitForURL(/\/simulations\/legacy\/[0-9a-f-]{36}$/);
const nmapRunning = () => sh(`docker top ${process.env.E2E_NMAP_CONTAINER} 2>/dev/null | grep -c "[n]map -sT" || true`).trim();
let started = false;
for (let i = 0; i < 30 && !started; i++) { started = nmapRunning() === '1'; if (!started) await sleep(500); }
ok(started, 'scan lambat benar-benar berjalan di container');
await p.getByRole('button', { name: 'Stop' }).click();
await p.getByRole('dialog').getByRole('button', { name: 'Stop simulation' }).click();
await p.locator('.page__title-row .status').filter({ hasText: 'Canceled' }).waitFor({ timeout: 15000 });
ok(await p.getByRole('row').filter({ hasText: 'Slow scan' }).getByText('Canceled', { exact: true }).isVisible(), 'langkah yang dihentikan berstatus Canceled');
ok(await p.getByRole('row').filter({ hasText: 'After slow' }).getByText('Blocked', { exact: true }).isVisible(), 'langkah berikutnya tidak pernah jalan (Blocked)');
let gone = false;
for (let i = 0; i < 30 && !gone; i++) { gone = nmapRunning() === '0'; if (!gone) await sleep(1000); }
ok(gone, 'proses nmap di container dihentikan setelah simulasi dihentikan');
ok((await p.getByRole('button', { name: 'Pause' }).count()) === 0, 'simulasi yang dihentikan tidak punya kontrol Pause/Stop lagi');

// 8. daftar simulasi dan filter
await p.getByRole('navigation', { name: 'Primary' }).getByRole('link', { name: 'Scans', exact: true }).click();
await p.waitForURL('**/simulations');
await p.locator('details').filter({has:p.locator('summary').getByText('Earlier runs',{exact:true})}).locator('summary').click();
await p.getByRole('table', { name: 'Simulations' }).waitFor();
ok((await p.getByRole('table', { name: 'Simulations' }).getByRole('row').count()) === 4, 'daftar memuat 3 simulasi + baris judul');
await p.getByRole('button', { name: 'Finished', exact: true }).click();
await p.waitForFunction(() => document.querySelectorAll('[aria-label=Simulations] [role=row]').length === 3);
ok(true, 'filter Finished menyisakan 2 simulasi');
await p.getByRole('button', { name: 'Failed', exact: true }).click();
await p.getByRole('heading', { name: 'No simulations with this status' }).waitFor();
ok(true, 'filter tanpa hasil menampilkan keadaan kosong');
await p.screenshot({ path: SHOTS + 'simulations-list.png' });

ok(errors.length === 0, 'tidak ada error konsol/halaman tak terduga' + (errors.length ? ': ' + errors.join(' | ') : ''));
await b.close();
console.log(`\n${pass} lulus, ${fail} gagal`);
process.exit(fail ? 1 : 0);

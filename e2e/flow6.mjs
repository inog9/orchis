// E2E Fase 4: agent Go SUNGGUHAN (biner hasil agentrelease): pasang dari perintah di konsol, heartbeat, aksi di endpoint,
// chain dengan langkah agent, update diri bertanda tangan, penolakan rilis berkunci lain, dan pencabutan.
import { chromium } from 'playwright';
import { spawn } from 'node:child_process';
import { copyFileSync, mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { mkdirSync } from 'node:fs';
const SHOTS = fileURLToPath(new URL('./shots/', import.meta.url));
mkdirSync(SHOTS, { recursive: true });
const BASE = process.env.E2E_BASE ?? 'http://127.0.0.1:8090';
const REL = process.env.E2E_RELEASE_DIR;
const PORT = new URL(BASE).port;
let pass = 0, fail = 0;
const ok = (c, m) => { (c ? pass++ : fail++); console.log((c ? 'LULUS: ' : 'GAGAL: ') + m); };
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const b = await chromium.launch();
const ctx = await b.newContext({ viewport: { width: 1440, height: 900 } });
const p = await ctx.newPage();
const errors = [];
const expected = /Failed to load resource: the server responded with a status of (401|404|409|422)/;
p.on('pageerror', (e) => errors.push('pageerror: ' + e.message));
p.on('console', (m) => m.type() === 'error' && !expected.test(m.text()) && errors.push('console: ' + m.text()));
const api = (method, path, data) => ctx.request.fetch(BASE + path, { method, data, headers: { 'X-Requested-With': 'orchis-web' } });

const workdir = mkdtempSync(join(tmpdir(), 'orchis-e2e-agent-'));
let agent = null;
let agentOut = '';
const killAgent = () => { if (agent && agent.pid) { try { process.kill(-agent.pid, 'SIGKILL'); } catch { /* sudah mati */ } } };
process.on('exit', () => { killAgent(); rmSync(workdir, { recursive: true, force: true }); });
const useManifest = (name) => copyFileSync(join(REL, name), join(REL, 'manifest.json'));
const agentVersion = async () => {
  const items = (await (await api('GET', '/api/v1/agents')).json()).items;
  return items[0]?.version;
};
const waitFor = async (fn, ms, step = 500) => { const end = Date.now() + ms; while (Date.now() < end) { const v = await fn(); if (v) return v; await sleep(step); } return null; };

await p.goto(BASE + '/login');
await p.getByLabel('Email').fill('admin@orchis.local');
await p.getByLabel('Password', { exact: true }).fill('Orchis-Dev-Pass-1');
await p.getByRole('button', { name: /sign in/i }).click();
await p.waitForURL(BASE + '/organizations');

// 1. tab Agents + keadaan kosong
await p.goto(BASE + '/assets');
await p.getByRole('link', { name: 'Agents', exact: true }).click();
await p.waitForURL('**/assets/agents');
await p.getByRole('heading', { name: 'No agents yet' }).waitFor();
ok(true, 'tab Agents di bawah Assets, dengan keadaan kosong yang mengajak memasang');

// 2. dialog pasang: perintah dengan checksum, token di header
await p.getByRole('button', { name: 'Install the first agent' }).click();
let dlg = p.getByRole('dialog');
await dlg.getByLabel('Run as').selectOption('user');
await dlg.getByRole('button', { name: 'Create install command' }).click();
const cmd = (await dlg.getByTestId('install-command').innerText()).replace(/ \\\n\s*&& /g, ' && ');
ok(/Authorization: Bearer oen_/.test(cmd) && /sha256sum -c -/.test(cmd) && !/\?token=/.test(cmd), 'perintah memakai token di header dan memeriksa SHA-256 sebelum menjalankan');
ok(await dlg.getByText('Waiting for the agent…').isVisible(), 'dialog menunggu agent terhubung');
await p.screenshot({ path: SHOTS + 'agent-install.png' });

// 3. jalankan perintah itu SUNGGUHAN di folder sementara
agent = spawn('bash', ['-c', cmd], { cwd: workdir, detached: true, stdio: ['ignore', 'pipe', 'pipe'] });
agent.stdout.on('data', (d) => { agentOut += d; });
agent.stderr.on('data', (d) => { agentOut += d; });
await dlg.getByText('Agent connected.').waitFor({ timeout: 60000 });
ok(true, 'dialog berubah menjadi "Agent connected." setelah agent sungguhan mendaftar');
await dlg.getByRole('button', { name: 'Done' }).click();
const row = p.getByRole('row').filter({ hasText: 'Linux' });
await row.getByText('Online').waitFor();
ok(/1\.0\.0/.test(await row.innerText()), 'daftar Agents: Online, versi 1.0.0');
await p.screenshot({ path: SHOTS + 'agents-list.png' });

// 4. aset yang terbentuk
await p.goto(BASE + '/assets');
await p.locator('.asset-row__meta', { hasText: 'Agent online' }).first().waitFor();
ok(true, 'daftar aset menandai endpoint yang punya agent: "Agent online"');
await p.locator('.asset-row__name').first().click();
await p.getByText('Agent online', { exact: true }).waitFor();
ok(true, 'detail aset menampilkan Executor: Agent online');
const assetId = p.url().split('/').pop();
const agentless = await (await api('POST', '/api/v1/assets', { category: 'host', name: 'plain-box', testing_authorized: true, addresses: [{ kind: 'ip', value: '10.9.9.9' }] })).json();
let cur = await (await api('GET', `/api/v1/assets/${assetId}`)).json();
ok((await api('PATCH', `/api/v1/assets/${assetId}`, { version: cur.version, testing_authorized: true })).status() === 200, 'admin mengotorisasi aset agent untuk pengujian');

// 5. atomic test dengan aksi agent
await p.goto(BASE + '/atomic');
await p.getByRole('button', { name: /create the first test/i }).click();
dlg = p.getByRole('dialog');
await dlg.getByLabel('Name').fill('Listening ports');
await dlg.getByLabel('Action').selectOption('agent.listening_ports');
const opts = await dlg.getByLabel('Target asset').locator('option').allTextContents();
ok(opts.length === 2 && !opts.includes('plain-box'), 'aksi agent hanya menawarkan aset yang punya agent: ' + opts.join(' | '));
ok(await dlg.getByText(/takes no settings/).isVisible(), 'aksi agent tidak punya pengaturan yang bisa disalahgunakan');
await dlg.getByLabel('Target asset').selectOption({ index: 1 });
await dlg.getByRole('button', { name: 'Create test' }).click();
await p.waitForURL(/\/atomic\/[0-9a-f-]{36}$/);
await p.getByRole('heading', { name: 'Listening ports', exact: true }).waitFor();
await p.getByRole('button', { name: /^run$/i }).click();
await p.locator('.panel .status').filter({ hasText: /Succeeded|Failed/ }).first().waitFor({ timeout: 60000 });
ok(await p.locator('.panel .status').filter({ hasText: 'Succeeded' }).first().isVisible(), 'agent mengambil tugas dan menyelesaikannya (Succeeded)');
const portTitle = p.getByText(`Listening port ${PORT}/tcp`, { exact: false }).first();
await portTitle.waitFor({ timeout: 10000 });
ok(/loopback only/.test(await portTitle.innerText()), `agent melihat port ${PORT} milik server uji di endpoint (loopback only)`);
await p.screenshot({ path: SHOTS + 'agent-ports.png' });

// 6. informasi sistem
await p.goto(BASE + '/atomic');
await p.getByRole('button', { name: /new single test/i }).click();
dlg = p.getByRole('dialog');
await dlg.getByLabel('Name').fill('System info');
await dlg.getByLabel('Action').selectOption('agent.system_info');
await dlg.getByLabel('Target asset').selectOption({ index: 1 });
await dlg.getByRole('button', { name: 'Create test' }).click();
await p.waitForURL(/\/atomic\/[0-9a-f-]{36}$/);
await p.getByRole('button', { name: /^run$/i }).click();
await p.getByText('System information:', { exact: false }).first().waitFor({ timeout: 60000 });
ok(true, 'aksi informasi sistem menghasilkan temuan teks dari endpoint');

// 7. chain dengan dua langkah agent
const sc = await (await api('POST', '/api/v1/scenarios', { name: 'On-host chain', severity: 'low' })).json();
await api('PUT', `/api/v1/scenarios/${sc.id}/targets`, { version: sc.version, asset_ids: [assetId], group_ids: [] });
const sc2 = await (await api('GET', `/api/v1/scenarios/${sc.id}`)).json();
await api('PUT', `/api/v1/scenarios/${sc.id}/graph`, { version: sc2.version, edges: [{ parent: 'ports', child: 'info' }], steps: [
  { key: 'ports', title: 'Listening ports', contract_key: 'agent.listening_ports', trigger_mode: 'after_parents', is_enabled: true, delay_seconds: 0, params: {} },
  { key: 'info', title: 'System info', contract_key: 'agent.system_info', trigger_mode: 'after_parents', is_enabled: true, delay_seconds: 0, params: {} }] });
const wrong = await (await api('POST', '/api/v1/scenarios', { name: 'Wrong target', severity: 'low' })).json();
await api('PUT', `/api/v1/scenarios/${wrong.id}/targets`, { version: wrong.version, asset_ids: [agentless.id], group_ids: [] });
const w2 = await (await api('GET', `/api/v1/scenarios/${wrong.id}`)).json();
await api('PUT', `/api/v1/scenarios/${wrong.id}/graph`, { version: w2.version, edges: [], steps: [{ key: 'ports', title: 'Listening ports', contract_key: 'agent.listening_ports', trigger_mode: 'after_parents', is_enabled: true, delay_seconds: 0, params: {} }] });
await p.goto(BASE + `/chains/${wrong.id}/setup`);
await p.getByRole('button', { name: /launch simulation/i }).click();
await p.getByRole('dialog').getByRole('button', { name: 'Launch simulation' }).click();
await p.getByText(/has no agent/).waitFor();
ok(true, 'peluncuran ke aset TANPA agent ditolak di muka dengan penjelasan');
await p.getByRole('dialog').getByRole('button', { name: 'Cancel' }).click();
await p.goto(BASE + `/chains/${sc.id}/setup`);
await p.getByRole('button', { name: /launch simulation/i }).click();
await p.getByRole('dialog').getByRole('button', { name: 'Launch simulation' }).click();
await p.waitForURL(/\/simulations\/legacy\/[0-9a-f-]{36}$/);
await p.locator('.page__title-row .status').filter({ hasText: /Finished/ }).waitFor({ timeout: 90000 });
ok(await p.getByRole('row').filter({ hasText: 'Listening ports' }).getByText('Done', { exact: true }).isVisible() && await p.getByRole('row').filter({ hasText: 'System info' }).getByText('Done', { exact: true }).isVisible(), 'simulasi dua langkah agent berjalan berurutan sampai Finished');

// 8. update diri bertanda tangan
ok((await agentVersion()) === '1.0.0', 'sebelum update: agent 1.0.0');
useManifest('manifest-1.1.0.json');
const updated = await waitFor(async () => (await agentVersion()) === '1.1.0', 60000, 1000);
ok(!!updated, 'agent memperbarui diri ke 1.1.0 (tanda tangan sah, uji asap lulus, restart otomatis)');
ok(!agent.killed && agent.exitCode === null, 'proses agent tetap hidup setelah update (menjalankan dirinya ulang)');
await p.goto(BASE + '/assets/agents');
await p.getByRole('row').filter({ hasText: 'Linux' }).getByText('1.1.0').waitFor();
ok(true, 'daftar Agents menampilkan versi 1.1.0');
await p.goto(BASE + '/atomic');
await p.getByRole('link', { name: 'Listening ports' }).click();
await p.getByRole('button', { name: /^run$/i }).click();
await p.locator('.runs__item').nth(1).waitFor({ timeout: 10000 });
await p.waitForFunction(() => document.querySelectorAll('.runs__item .status--ok').length >= 2, null, { timeout: 60000 });
ok(true, 'agent hasil update tetap menjalankan tugas');

// 9. rilis yang ditandatangani kunci LAIN ditolak
useManifest('manifest-1.2.0-bad.json');
const refused = await waitFor(async () => /TIDAK cocok/.test(agentOut), 60000, 1000);
ok(!!refused, 'rilis berkunci lain ditolak agent: "tanda tangan TIDAK cocok"');
await sleep(1500);
ok((await agentVersion()) === '1.1.0', 'agent tetap di 1.1.0 setelah menolak rilis palsu');
useManifest('manifest-1.1.0.json');

// 10. pencabutan
await p.goto(BASE + '/assets/agents');
await p.getByRole('button', { name: /Revoke agent on/ }).click();
await p.getByRole('dialog').getByRole('button', { name: 'Revoke agent' }).click();
await p.getByRole('row').filter({ hasText: 'Linux' }).getByText('Revoked').waitFor();
ok(true, 'pencabutan dari konsol: status Revoked');
const wiped = await waitFor(async () => { try { const c = JSON.parse(readFileSync(join(workdir, 'agent-config.json'), 'utf8')); return c.revoked === true && c.secret === ''; } catch { return false; } }, 40000, 1000);
ok(!!wiped, 'agent yang dicabut menghapus kredensialnya dari disk');
ok(/dicabut/.test(agentOut) && agent.exitCode === null, 'agent yang dicabut berhenti bekerja tetapi tidak keluar (tidak memicu restart berulang)');
await p.goto(BASE + '/atomic');
await p.getByRole('link', { name: 'Listening ports' }).click();
await p.getByRole('button', { name: /^run$/i }).click();
await p.getByText(/has no agent/).waitFor();
ok(true, 'setelah dicabut, menjalankan test di aset itu ditolak ("has no agent")');
await p.screenshot({ path: SHOTS + 'agent-revoked.png' });
killAgent();

ok(errors.length === 0, 'tidak ada error konsol/halaman tak terduga' + (errors.length ? ': ' + errors.join(' | ') : ''));
await b.close();
console.log(`\n${pass} lulus, ${fail} gagal`);
process.exit(fail ? 1 : 0);

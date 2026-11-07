// Flow 26: alur produk ujung-ke-ujung (PROMPT-E2E-FLOW.md) — 8 skenario bernomor.
// (1) organisasi baru + empty state; (2) aset web -> wizard -> tampilan engine tanpa nama Nmap/Nuclei;
// (3) agent (palsu) -> checklist kesiapan menyala bertahap; (4) setujui koleksi -> DefenseBot -> temuan + banner;
// (5) Full assessment -> "seen by scanner" dan "also found by"; (6) retest + triage; (7) dashboard cakupan;
// (8) dua bahasa dan dua tema. Flow berperan sebagai agent dan injector palsu lewat API resmi (pola test Go);
// tidak ada host nyata dan tidak ada layanan kerentanan eksternal yang dihubungi.
import assert from 'node:assert/strict';
import { mkdirSync } from 'node:fs';
import { chromium } from 'playwright';
const BASE = process.env.E2E_BASE;
if (!BASE) throw Error('Requires isolated flow wrapper');
let checks = 0;
const check = (v, m) => { assert(v, m); checks++; console.log('LULUS: ' + m); };
const shots = 'e2e/shots/flow26';
mkdirSync(shots, { recursive: true });
const browser = await chromium.launch();
const ctx = await browser.newContext({ viewport: { width: 1280, height: 900 } });
const page = await ctx.newPage();
const consoleErrors = [];
page.on('pageerror', (e) => consoleErrors.push('pageerror: ' + e.message));
page.on('console', (m) => m.type() === 'error' && !/Failed to load resource.*\((400|401|403|404|409|422)\)/.test(m.text()) && consoleErrors.push('console: ' + m.text()));
const headers = { 'X-Requested-With': 'orchis-web' };
const api = async (method, path, data, extra = {}) => {
  const r = await ctx.request.fetch(BASE + '/api/v1' + path, { method, data, headers: { ...headers, ...extra } });
  if (!r.ok() && r.status() !== 204) throw new Error(`${method} ${path} -> ${r.status()} ${await r.text()}`);
  return r.status() === 204 ? null : r.json();
};
const wait = async (fn, ms = 30000) => { const end = Date.now() + ms; while (Date.now() < end) { const v = await fn(); if (v) return v; await new Promise((r) => setTimeout(r, 250)); } throw Error('timed out'); };
const inj = (token, path, body) => fetch(BASE + '/api/v1/injector/' + path, { method: 'POST', headers: { Authorization: 'Bearer ' + token, 'Content-Type': 'application/json', 'X-Requested-With': 'orchis-web' }, body: JSON.stringify(body ?? {}) });
const agent = (secret, path, body) => fetch(BASE + '/api/v1/agent/' + path, { method: 'POST', headers: { Authorization: 'Bearer ' + secret, 'Content-Type': 'application/json', 'X-Requested-With': 'orchis-web' }, body: JSON.stringify(body ?? {}) });
async function answerCollections(secret, rows) {
  for (let i = 0; i < 12; i++) {
    const r = await agent(secret, 'executions/claim');
    if (r.status !== 200) return;
    const spec = await r.json();
    await agent(secret, `executions/${spec.execution_id}/result`, { exit_code: 0, datasets: [{ query: spec.contract_key, status: 'ok', rows: rows(spec.contract_key) }] });
  }
}
const defenseRows = (key) => ({
  'osquery.os_release': [{ name: 'Ubuntu', version: '24.04.1 LTS', major: '24', minor: '4', platform: 'ubuntu', platform_like: 'debian', codename: 'noble' }],
  'osquery.packages': [{ name: 'libssl3t64', source_name: 'openssl', version: '3.0.13-0ubuntu3.4', arch: 'amd64', pkg_type: 'deb', epoch: '', release: '' }],
  'osquery.exposure': [{ port: '443', protocol: '6', address: '0.0.0.0', process: 'nginx', exe_path: '/usr/sbin/nginx', owner_package: 'nginx' }],
  'osquery.hardening': [{ item: 'sysctl', target: 'kernel.randomize_va_space', value: '2' }],
  'osquery.firewall': [{ iptables_rules: '4', active_firewall_units: '1' }],
}[key] ?? []);

await page.goto(BASE + '/login');
await page.getByLabel('Email').fill('admin@orchis.local');
await page.getByLabel('Password', { exact: true }).fill('Orchis-Dev-Pass-1');
await page.getByRole('button', { name: /sign in/i }).click();
await page.waitForURL(BASE + '/organizations');

const org = await api('POST', '/organizations', { name: 'Flow laboratory', slug: 'flow-laboratory' });

// ---------- Skenario 1: organisasi baru menunjukkan langkah berikutnya ----------
await page.goto(`${BASE}/simulations/new?org=${org.id}`);
await page.getByRole('option', { name: 'Flow laboratory' }).waitFor();
check(await page.getByText('This organisation has no assets yet.').isVisible(), 'S1: wizard organisasi kosong meminta menambah aset');
await page.goto(`${BASE}/organizations/${org.id}`);
await page.getByRole('heading', { name: 'Coverage' }).waitFor();
check(await page.getByText('Every asset is ready to be assessed.').isVisible(), 'S1: cakupan organisasi kosong menyatakan semua siap');
await page.screenshot({ path: `${shots}/s1-empty.png`, fullPage: true });

// ---------- Skenario 2: aset web -> wizard -> tanpa nama Nmap/Nuclei ----------
const webAsset = await api('POST', '/assets', { name: 'portal.flow.test', category: 'web_application', organization_id: org.id, testing_authorized: true, addresses: [{ kind: 'url', value: 'http://127.0.0.1:18081/' }] });
await page.goto(`${BASE}/simulations/new?org=${org.id}&asset=${webAsset.id}`);
await page.getByRole('button', { name: 'Continue' }).click();
await page.getByRole('radio', { name: 'Web pentest (Hackbot)' }).click();
await page.getByRole('button', { name: 'Continue' }).click();
const webBox = page.getByRole('checkbox', { name: /portal.flow.test/ });
await webBox.waitFor();
check(await webBox.isChecked(), 'S2: aset dari ?asset= sudah terpilih');
await page.getByRole('button', { name: 'Continue' }).click();
check(await page.getByText('Web pentest (Hackbot)').isVisible(), 'S2: tinjauan menyebut engine');
await page.getByRole('button', { name: 'Launch scan' }).click();
await page.waitForURL(/\/simulations\/[0-9a-f-]{36}$/);
const scan2URL = page.url();
const scan2 = scan2URL.split('/').pop();
await page.getByRole('table').waitFor();
const body2 = await page.locator('body').textContent();
check(!/\bnmap\b|\bnuclei\b/i.test(body2), 'S2: detail scan tidak menyebut Nmap atau Nuclei');
check(await page.getByText('Hackbot', { exact: true }).isVisible(), 'S2: baris engine Hackbot tampil');
await page.screenshot({ path: `${shots}/s2-web-wizard.png`, fullPage: true });

// Selesaikan pipeline Hackbot lewat API injector (token resmi per tool).
const integs = (await api('GET', '/integrations')).items;
const tokenFor = async (key) => (await api('POST', `/integrations/${integs.find((i) => i.key === key).id}/token`)).token;

// ---------- Skenario 3: agent terdaftar -> checklist menyala ----------
const tok = (await api('POST', '/agents/enrollment-tokens', { os: 'linux', arch: 'x86_64', mode: 'service' })).token;
const enr = await (await agent(null, 'enroll', { token: tok, hostname: 'flow-box', os: 'linux', arch: 'x86_64', version: '1.0.0', protocol_version: 1 })).json();
await agent(enr.secret, 'heartbeat', { version: '1.0.0', protocol_version: 1 });
const cur = await api('GET', `/assets/${enr.asset_id}`);
await api('PATCH', `/assets/${enr.asset_id}`, { version: cur.version, testing_authorized: true, organization_id: org.id });
await page.goto(`${BASE}/assets/${enr.asset_id}`);
await page.getByRole('heading', { name: 'Readiness' }).waitFor();
check(await page.getByText('Agent installed and online').locator('..').locator('.status--ok').isVisible(), 'S3: langkah agent hijau di checklist');
check(await page.getByText('Approved web URL').locator('..').getByText('○').isVisible(), 'S3: langkah URL masih terbuka untuk aset agent');
await page.screenshot({ path: `${shots}/s3-checklist-agent.png`, fullPage: true });

// ---------- Skenario 4: setujui koleksi -> DefenseBot -> temuan + banner belum lengkap ----------
const catalog = await api('GET', '/osquery/catalog');
const consent = { version: catalog.consent.version, sha256: catalog.consent.sha256 };
const settings = (await api('GET', `/organizations/${org.id}/osquery`)).items;
for (const q of catalog.queries.filter((q) => q.os?.includes('linux') && !q.pack)) {
  const s = settings.find((i) => i.key === q.key);
  if (!s) continue;
  await api('PUT', `/organizations/${org.id}/osquery/${q.key}`, { enabled: true, consent, version: s.version });
}
for (const pack of catalog.packs.filter((pk) => pk.platform === 'linux')) {
  const keys = catalog.queries.filter((q) => q.pack === pack.name && q.os?.includes('linux'));
  if (!keys.length) continue;
  const versions = {};
  for (const q of keys) { const s = settings.find((i) => i.key === q.key); if (s) versions[q.key] = s.version; }
  if (!Object.keys(versions).length) continue;
  await api('PUT', `/organizations/${org.id}/osquery/${keys[0].key}`, { enabled: true, consent, versions });
}
await page.goto(`${BASE}/simulations/new?org=${org.id}&asset=${enr.asset_id}`);
await page.getByRole('button', { name: 'Continue' }).click();
await page.getByRole('radio', { name: 'Installed software check (DefenseBot)' }).click();
await page.getByRole('button', { name: 'Continue' }).click();
const defBox = page.getByRole('checkbox', { name: /flow-box/ });
await defBox.waitFor();
await defBox.click();
await page.getByRole('button', { name: 'Continue' }).click();
check(await page.getByText('Approved for the current document').isVisible(), 'S4: tinjauan menyatakan consent disetujui');
await page.getByRole('button', { name: 'Launch scan' }).click();
await page.waitForURL(/\/simulations\/[0-9a-f-]{36}$/);
const scan4 = page.url().split('/').pop();
await answerCollections(enr.secret, defenseRows);
const defTok = await tokenFor('defensebot');
await wait(async () => {
  const d = await api('GET', `/scans/${scan4}`);
  const job = (d.jobs ?? []).find((j) => j.tool === 'defensebot');
  if (job?.status !== 'running') return null;
  const claim = await inj(defTok, `executions/${job.simulation_id}/claim`);
  if (!claim.ok) return null;
  const spec = await claim.json();
  const details = { cve: 'CVE-2024-0001', package: 'libssl3t64', installed_version: '3.0.13-0ubuntu3.4', fixed_version: '3.0.13-0ubuntu3.5', kev: true, exposure: 'exposed', method: 'osv-distro' };
  await inj(defTok, `executions/${job.simulation_id}/result`, { exit_code: 0, defense: { schema: 1, disposition: 'limited', platform: 'ubuntu', software_checked: 2, by_severity: { high: 1 }, findings: 1, known_exploited: 1, notes: [{ code: 'apps_unmapped', count: 1, limiting: true }], provenance: { engine: 'defensebot-1', ecosystem: 'Ubuntu:24.04:LTS', queried_at: new Date().toISOString(), osv_pairs: 2, severity_policy: 'advisory', sources: ['OSV (osv.dev)'] } }, findings: [{ type: 'vulnerability', value: 'defensebot:ubuntu:24.04:lts:openssl:cve-2024-0001', title: 'CVE-2024-0001 in libssl3t64 3.0.13-0ubuntu3.4', severity: 'high', details }] });
  return true;
});
await wait(async () => (await api('GET', `/scans/${scan4}`)).status === 'incomplete');
await page.goto(`${BASE}/simulations/${scan4}`);
check(await page.getByText('DefenseBot', { exact: true }).isVisible(), 'S4: baris engine DefenseBot tampil');
await page.goto(`${BASE}/findings?org=${org.id}`);
await page.getByText('CVE-2024-0001 in libssl3t64 3.0.13-0ubuntu3.4').waitFor();
check(await page.getByText('Known exploited', { exact: false }).first().isVisible().catch(() => false) || await page.locator('.fnd-def--kev').first().isVisible().catch(() => false), 'S4: temuan DefenseBot membawa penanda KEV');
await page.screenshot({ path: `${shots}/s4-defensebot-findings.png`, fullPage: true });

// ---------- Skenario 5: Full assessment -> urutan + "also found by" ----------
await api('PATCH', `/assets/${enr.asset_id}`, { version: (await api('GET', `/assets/${enr.asset_id}`)).version, addresses: [{ kind: 'ip', value: '127.0.0.1' }, { kind: 'url', value: 'http://127.0.0.1:18081/' }] });
await page.goto(`${BASE}/simulations/new?org=${org.id}&asset=${enr.asset_id}`);
await page.getByRole('button', { name: 'Continue' }).click();
await page.getByRole('radio', { name: 'Full assessment' }).click();
await page.getByRole('button', { name: 'Continue' }).click();
const fullBox = page.getByRole('checkbox', { name: /flow-box/ });
await fullBox.waitFor();
await fullBox.click();
await page.getByRole('button', { name: 'Continue' }).click();
await page.getByRole('button', { name: 'Launch scan' }).click();
await page.waitForURL(/\/simulations\/[0-9a-f-]{36}$/);
const scan5 = page.url().split('/').pop();
// Jalankan pipeline lengkap: koleksi agent dijawab, nmap+nuclei selesai, hackbot dan defensebot menghasilkan temuan.
const nmapTok = await tokenFor('nmap'); const nucTok = await tokenFor('nuclei'); const hackTok = await tokenFor('hackbot');
await wait(async () => {
  const d = await api('GET', `/scans/${scan5}`);
  const byTool = (t) => (d.jobs ?? []).filter((j) => j.tool === t);
  for (const job of [...byTool('nmap'), ...byTool('nuclei')].filter((j) => j.status === 'running')) {
    const tok2 = job.tool === 'nmap' ? nmapTok : nucTok;
    if ((await inj(tok2, `executions/${job.simulation_id}/claim`)).ok) {
      await inj(tok2, `executions/${job.simulation_id}/result`, { exit_code: 0, findings: [{ type: 'open_port', value: 'tcp/443', title: 'Open port 443/tcp', severity: 'info', details: { port: 443 } }] });
    }
  }
  for (const job of byTool('hackbot').filter((j) => j.status === 'running')) {
    if ((await inj(hackTok, `executions/${job.simulation_id}/claim`)).ok) {
      await inj(hackTok, `executions/${job.simulation_id}/result`, { exit_code: 0, summary: 'Explored public routes of the fixture.', findings: [{ type: 'vulnerability', value: 'CVE-2024-0001', title: 'CVE-2024-0001 in fixture portal', severity: 'high', details: { cve: 'CVE-2024-0001', template_id: 'CVE-2024-0001', method: 'web-test' } }] });
    }
  }
  const defense = byTool('defensebot')[0];
  if (defense?.status === 'running' && defense.simulation_id) {
    const claim = await inj(defTok, `executions/${defense.simulation_id}/claim`);
    if (claim.ok) {
      const details = { cve: 'CVE-2024-0001', package: 'libssl3t64', installed_version: '3.0.13-0ubuntu3.4', fixed_version: '3.0.13-0ubuntu3.5', kev: true, exposure: 'exposed', method: 'osv-distro' };
      await inj(defTok, `executions/${defense.simulation_id}/result`, { exit_code: 0, defense: { schema: 1, disposition: 'complete', platform: 'ubuntu', software_checked: 2, by_severity: { high: 1 }, findings: 1, known_exploited: 1, notes: [], provenance: { engine: 'defensebot-1', ecosystem: 'Ubuntu:24.04:LTS', queried_at: new Date().toISOString(), osv_pairs: 2, severity_policy: 'advisory', sources: ['OSV (osv.dev)'] } }, findings: [{ type: 'vulnerability', value: 'defensebot:ubuntu:24.04:lts:openssl:cve-2024-0001', title: 'CVE-2024-0001 in libssl3t64 3.0.13-0ubuntu3.4', severity: 'high', details }] });
    }
  }
  return ['finished', 'incomplete'].includes(d.status) ? d.status : null;
}, 60000);
await answerCollections(enr.secret, defenseRows);
const final5 = await wait(async () => { const d = await api('GET', `/scans/${scan5}`); return ['finished', 'incomplete'].includes(d.status) ? d : null; }, 60000);
check(final5.status === 'finished', 'S5: full assessment selesai');
const tools5 = [...new Set((final5.jobs ?? []).map((j) => j.tool))];
check(!tools5.includes('defensebot') || (final5.jobs ?? []).every((j) => ['finished', 'incomplete'].includes(j.status)), 'S5: semua engine selesai, DefenseBot tidak menunggu selamanya');
await page.goto(`${BASE}/simulations/${scan5}`);
check(await page.getByText('DefenseBot', { exact: true }).isVisible() && await page.getByText('Hackbot', { exact: true }).isVisible(), 'S5: kedua engine tampil sebagai baris terpisah');
const report5 = (await api('GET', `/reports?assessment_id=${scan5}`)).items[0];
const snap = await api('GET', `/reports/${report5.id}/export/json`);
const defFinding = (snap.assets ?? []).flatMap((a) => a.findings ?? []).find((f) => (f.evidence?.method ?? '').startsWith('osv'));
check(!!defFinding, 'S5: temuan sisi dalam ada di laporan');
check((defFinding?.evidence?.confirmed_by ?? '').length > 0, 'S5: temuan dalam ditandai "also found by" (' + (defFinding?.evidence?.confirmed_by ?? '') + ')');
check((defFinding?.evidence?.exposure ?? '') === 'exposed', 'S5: tag "seen by scanner" (exposure) terisi dari hasil pemindai');
await page.screenshot({ path: `${shots}/s5-full-assessment.png`, fullPage: true });

// ---------- Skenario 6: retest + triage ----------
await page.goto(`${BASE}/findings?org=${org.id}`);
await page.getByText('CVE-2024-0001 in libssl3t64 3.0.13-0ubuntu3.4').first().click();
await page.getByRole('button', { name: 'Retest' }).click();
await page.waitForURL(/\/simulations\/[0-9a-f-]{36}$/);
check(true, 'S6: retest membuka detail scan baru');
await page.goto(`${BASE}/findings?org=${org.id}`);
await page.getByText('CVE-2024-0001 in libssl3t64 3.0.13-0ubuntu3.4').first().click();
await page.getByLabel('Triage status').selectOption('accepted_risk');
await page.getByPlaceholder('Reason').fill('Compensating control reviewed by security');
await page.getByLabel('Accepted until').fill('2027-01-01');
await page.getByRole('button', { name: 'Save triage' }).click();
await page.getByText('Accepted risk').first().waitFor();
await page.keyboard.press('Escape');
await page.getByRole('button', { name: 'To handle' }).click();
await page.waitForTimeout(600);
check(!(await page.getByText('CVE-2024-0001 in libssl3t64 3.0.13-0ubuntu3.4').first().isVisible().catch(() => false)), 'S6: risiko diterima disembunyikan dari daftar yang perlu ditangani');
await page.screenshot({ path: `${shots}/s6-triage.png`, fullPage: true });

// ---------- Skenario 7: dashboard cakupan ----------
await page.goto(`${BASE}/organizations/${org.id}`);
await page.getByRole('heading', { name: 'Coverage' }).waitFor();
check(await page.getByText('No agent').first().isVisible(), 'S7: cakupan menandai aset tanpa agent');
await page.screenshot({ path: `${shots}/s7-coverage.png`, fullPage: true });

// ---------- Skenario 8: dua bahasa dan dua tema ----------
await page.locator('.ws__button').click();
await page.locator('#language-preference').selectOption('id');
await page.waitForFunction(() => document.documentElement.lang === 'id');
await page.goto(`${BASE}/simulations/new?org=${org.id}`);
await page.getByRole('heading', { name: 'Pindai baru' }).waitFor();
check(true, 'S8: judul wizard dalam Bahasa Indonesia');
await page.evaluate(() => { localStorage.setItem('orchis.theme', 'light'); });
await page.reload();
check(await page.evaluate(() => document.documentElement.dataset.theme) === 'light', 'S8: tema terang aktif');
await page.screenshot({ path: `${shots}/s8-id-light.png`, fullPage: true });
check(consoleErrors.length === 0, 'S8: nol error konsol (' + consoleErrors.slice(0, 3).join(' | ') + ')');

console.log(`Flow26 selesai: ${checks} pemeriksaan lulus`);
await browser.close();

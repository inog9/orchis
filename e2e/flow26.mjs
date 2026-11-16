// Flow 26: alur produk ujung-ke-ujung (PROMPT-E2E-FLOW.md) — 8 skenario bernomor.
// (1) organisasi baru + empty state; (2) aset web -> wizard -> tampilan engine tanpa nama Nmap/Nuclei;
// (3) agent (palsu) -> checklist kesiapan menyala bertahap; (4) setujui koleksi -> orchis-defense-agent -> temuan;
// (5) Full assessment -> "seen by scanner" dan "also found by"; (6) retest + triage; (7) dashboard cakupan;
// (8) dua bahasa dan dua tema. Flow berperan sebagai agent (polling) dan injector palsu (protokol asli:
// konsumsi dispatch exec.* dari RabbitMQ, klaim + hasil lewat API injector dengan token integrasi).
// Tidak ada host nyata dan tidak ada layanan kerentanan eksternal yang dihubungi.
import assert from 'node:assert/strict';
import { mkdirSync } from 'node:fs';
import { chromium } from 'playwright';
import amqp from 'amqplib';
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
page.on('console', (m) => m.type() === 'error' && !/Failed to load resource: the server responded with a status of (400|401|403|404|409|422)/.test(m.text()) && consoleErrors.push('console: ' + m.text()));
const headers = { 'X-Requested-With': 'orchis-web' };
const api = async (method, path, data, extra = {}) => {
  const r = await ctx.request.fetch(BASE + '/api/v1' + path, { method, data, headers: { ...headers, ...extra } });
  if (!r.ok() && r.status() !== 204) throw new Error(`${method} ${path} -> ${r.status()} ${await r.text()}`);
  return r.status() === 204 ? null : r.json();
};
const wait = async (fn, ms = 45000) => { const end = Date.now() + ms; while (Date.now() < end) { const v = await fn(); if (v) return v; await new Promise((r) => setTimeout(r, 250)); } throw Error('timed out'); };
const inj = (token, path, body) => fetch(BASE + '/api/v1/injector/' + path, { method: 'POST', headers: { Authorization: 'Bearer ' + token, 'Content-Type': 'application/json', 'X-Requested-With': 'orchis-web' }, body: JSON.stringify(body ?? {}) });
const agent = (secret, path, body) => fetch(BASE + '/api/v1/agent/' + path, { method: 'POST', headers: { Authorization: 'Bearer ' + secret, 'Content-Type': 'application/json', 'X-Requested-With': 'orchis-web' }, body: JSON.stringify(body ?? {}) });
const see = async (loc) => { try { await loc.waitFor({ timeout: 8000 }); return true; } catch { return false; } };
const tick = (box) => box.evaluate((el) => el.click());
async function answerCollections(secret, rows) {
  // Klaim sampai antrean kosong (batas wajar): koleksi berkala otomatis bisa mendahului
  // eksekusi scan di antrean agent yang sama, jadi jumlah tetap tidak cukup.
  for (let i = 0; i < 60; i++) {
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

// Injector palsu: mengikuti protokol injector asli — konsumsi dispatch exec.* dari RabbitMQ,
// klaim lewat API injector dengan token integrasi, lalu kirim hasil deterministik.
let discoveryGate, releaseDiscovery;
const AMQP = process.env.E2E_AMQP_URL ?? 'amqp://orchis:orchis@127.0.0.1:15672/';
async function startFakeInjector() {
  const integs = (await api('GET', '/integrations')).items;
  const tokenFor = async (key) => (await api('POST', `/integrations/${integs.find((i) => i.key === key).id}/token`)).token;
  const tokens = { nmap: await tokenFor('nmap'), nuclei: await tokenFor('nuclei'), hackbot: await tokenFor('hackbot'), defensebot: await tokenFor('defensebot') };
  const conn = await amqp.connect(AMQP);
  const ch = await conn.createChannel();
  const q = await ch.assertQueue('', { exclusive: true });
  for (const k of ['nmap', 'nuclei', 'hackbot', 'defensebot']) await ch.bindQueue(q.queue, 'orchis.events', 'exec.' + k);
  ch.consume(q.queue, async (msg) => {
    if (!msg) return;
    try {
      const payload = JSON.parse(msg.content.toString());
      const key = String(payload.integration ?? msg.fields.routingKey.replace('exec.', ''));
      const execID = payload.execution_id;
      const tok = tokens[key];
      if (tok && execID) {
        const claim = await inj(tok, `executions/${execID}/claim`);
        if (claim.ok) {
          if (key === 'nmap' && discoveryGate) await discoveryGate;
          if (key === 'defensebot') {
            const details = { cve: 'CVE-2024-0001', package: 'libssl3t64', installed_version: '3.0.13-0ubuntu3.4', fixed_version: '3.0.13-0ubuntu3.5', kev: true, exposure: 'exposed', method: 'osv-distro' };
            await inj(tok, `executions/${execID}/result`, { exit_code: 0, defense: { schema: 1, disposition: 'complete', platform: 'ubuntu', software_checked: 2, by_severity: { high: 1 }, findings: 1, known_exploited: 1, notes: [], provenance: { engine: 'defensebot-1', ecosystem: 'Ubuntu:24.04:LTS', queried_at: new Date().toISOString(), osv_pairs: 2, severity_policy: 'advisory', sources: ['OSV (osv.dev)'] } }, findings: [{ type: 'vulnerability', value: 'defensebot:ubuntu:24.04:lts:openssl:cve-2024-0001', title: 'CVE-2024-0001 in libssl3t64 3.0.13-0ubuntu3.4', severity: 'high', details }] });
          } else if (key === 'hackbot') {
            const res = await inj(tok, `executions/${execID}/result`, { exit_code: 0, hackbot: { schema: 1, session_id: 'session-1', disposition: 'complete', phase: 'report', summary: 'Explored public routes of the fixture.', phases: ['recon', 'report'], verified_tasks: 1, blockers: [], findings: [{ id: 'f1', title: 'CVE-2024-0001 in fixture portal', severity: 'high', value: 'http://127.0.0.1:18081/#f1', details: { verdict: 'VALIDATED', reason: 'Controlled fixture evidence', cve: 'CVE-2024-0001', template_id: 'CVE-2024-0001' } }], report: '# Controlled fixture report', validation: '# Internal validation ledger' } });
          } else {
            const res = await inj(tok, `executions/${execID}/result`, { exit_code: 0, findings: [{ type: 'open_port', value: 'tcp/443', title: 'Open port 443/tcp', severity: 'info', details: { port: 443 } }] });
          }
        }
      }
    } catch (e) { console.log('DBG injector:', e.message); }
    ch.ack(msg);
  });
  return { close: () => conn.close().catch(() => {}) };
}

await page.goto(BASE + '/login');
await page.getByLabel('Email').fill('admin@orchis.local');
await page.getByLabel('Password', { exact: true }).fill('Orchis-Dev-Pass-1');
await page.getByRole('button', { name: /sign in/i }).click();
await page.waitForURL(BASE + '/overview');

const org = await api('POST', '/organizations', { name: 'Flow laboratory', slug: 'flow-laboratory' });
const fakeInjector = await startFakeInjector();

// ---------- Skenario 1: organisasi baru menunjukkan langkah berikutnya ----------
await page.goto(`${BASE}/simulations/new?org=${org.id}`);
// Wizard baru memakai grup radio organisasi pada langkah 1 (bukan combobox).
await wait(async () => (await page.getByRole('radiogroup', { name: 'Organisation' }).getByRole('radio', { name: /Flow laboratory/ }).getAttribute('aria-checked')) === 'true' ? true : null);
await page.getByText('This organisation has no assets yet.').waitFor();
check(true, 'S1: wizard organisasi kosong meminta menambah aset');
await page.goto(`${BASE}/organizations/${org.id}`);
await page.getByRole('heading',{name:org.name,exact:true}).waitFor();
check(await page.getByRole('link',{name:'Add asset',exact:true}).first().isVisible(),'S1: empty organisation offers asset setup');
await page.screenshot({ path: `${shots}/s1-empty.png`, fullPage: true });

// ---------- Skenario 2: aset web -> wizard -> tanpa nama Nmap/Nuclei ----------
const webAsset = await api('POST', '/assets', { name: 'portal.flow.test', category: 'web_application', organization_id: org.id, testing_authorized: true, addresses: [{ kind: 'url', value: 'http://127.0.0.1:18081/' }] });
await page.goto(`${BASE}/simulations/new?org=${org.id}&asset=${webAsset.id}`);
await page.getByRole('button', { name: 'Continue' }).click();
await page.getByRole('radio', { name: 'Web pentest (orchis-redteam-agent)' }).click();
await page.getByRole('button', { name: 'Continue' }).click();
const webBox = page.getByRole('checkbox', { name: /portal.flow.test/ });
await webBox.waitFor();
await wait(async () => (await webBox.isChecked()) ? true : null);
check(true, 'S2: aset dari ?asset= sudah terpilih');
await page.getByRole('button', { name: 'Continue' }).click();
check(await page.getByText('Web pentest (orchis-redteam-agent)').isVisible(), 'S2: tinjauan menyebut engine');
await page.getByRole('button', { name: 'Launch scan' }).click();
await page.waitForURL(/\/simulations\/[0-9a-f-]{36}$/);
const scan2 = page.url().split('/').pop();
await page.getByRole('table').waitFor();
const body2 = await page.locator('body').textContent();
check(!/\bnmap\b|\bnuclei\b/i.test(body2), 'S2: detail scan tidak menyebut Nmap atau Nuclei');
check(await page.getByText('orchis-redteam-agent', { exact: true }).isVisible(), 'S2: baris engine Hackbot tampil');
await page.screenshot({ path: `${shots}/s2-web-wizard.png`, fullPage: true });
await wait(async () => (await api('GET', `/scans/${scan2}`)).status === 'finished');

// ---------- Skenario 3: agent terdaftar -> checklist menyala ----------
const tok = (await api('POST', '/agents/enrollment-tokens', { os: 'linux', arch: 'x86_64', mode: 'service' })).token;
const enr = await (await agent(null, 'enroll', { token: tok, hostname: 'flow-box', os: 'linux', arch: 'x86_64', version: '1.0.0', protocol_version: 1 })).json();
await agent(enr.secret, 'heartbeat', { version: '1.0.0', protocol_version: 1 });
const agentAsset = (await api('GET', '/agents')).items.find((a) => a.id === enr.agent_id).asset_id;
const cur = await api('GET', `/assets/${agentAsset}`);
await api('PUT', `/organizations/${org.id}/assets`, { assets: [{ id: agentAsset, version: cur.version }], testing_authorized: true });
await page.goto(`${BASE}/assets/${agentAsset}`);
await page.getByRole('heading', { name: 'Readiness' }).waitFor();
await page.locator('.ready__checks > summary').click();
check(await page.getByText('Agent installed and online').locator('..').locator('.status--ok').isVisible(), 'S3: langkah agent hijau di checklist');
check(await page.getByText('Approved web URL').locator('..').getByText('○').isVisible(), 'S3: langkah URL masih terbuka untuk aset agent');
await page.screenshot({ path: `${shots}/s3-checklist-agent.png`, fullPage: true });

// ---------- Skenario 4: setujui koleksi -> orchis-defense-agent -> temuan ----------
const catalog = await api('GET', '/osquery/catalog');
const consent = { version: catalog.consent.version, sha256: catalog.consent.sha256 };
// Server menanam baris orchis-defense-agent + pack runnable otomatis begitu aset terikat organisasi;
// versi dibaca ulang tepat sebelum tiap PUT agar tidak balapan dengan penanam baris.
const approve = async (key) => {
  const version = ((await api('GET', `/organizations/${org.id}/osquery`)).items.find((i) => i.key === key)?.version) ?? 0;
  await api('PUT', `/organizations/${org.id}/osquery/${key}`, { enabled: true, consent, version });
};
for (const q of catalog.queries.filter((q) => q.os?.includes('linux') && !q.pack)) await approve(q.key);
for (const pack of catalog.packs.filter((pk) => pk.platform === 'linux')) {
  for (const q of catalog.queries.filter((q) => q.pack === pack.name && q.os?.includes('linux'))) await approve(q.key);
}
await page.goto(`${BASE}/simulations/new?org=${org.id}&asset=${agentAsset}`);
await page.getByRole('button', { name: 'Continue' }).click();
await page.getByRole('radio', { name: 'Installed software check (orchis-defense-agent)' }).click();
await page.getByRole('button', { name: 'Continue' }).click();
const defBox = page.getByRole('checkbox', { name: /flow-box/ });
await defBox.waitFor();
await tick(defBox);
await page.getByRole('button', { name: 'Continue' }).click();
check(await page.getByText('Approved for the current document').isVisible(), 'S4: tinjauan menyatakan consent disetujui');
await page.getByRole('button', { name: 'Launch scan' }).click();
await page.waitForURL(/\/simulations\/[0-9a-f-]{36}$/);
const scan4 = page.url().split('/').pop();
await answerCollections(enr.secret, defenseRows);
const final4 = await wait(async () => { const d = await api('GET', `/scans/${scan4}`); return ['finished', 'incomplete'].includes(d.status) ? d : null; });
await page.goto(`${BASE}/simulations/${scan4}`);
check(await see(page.locator('.sims__jobs strong', { hasText: 'orchis-defense-agent' })), 'S4: baris engine orchis-defense-agent tampil');
check(final4.status === 'incomplete' || final4.status === 'finished', 'S4: orchis-defense-agent selesai dengan banner bila terbatas');
await page.goto(`${BASE}/findings?org=${org.id}`);
await page.getByRole('button',{name:'Critical',exact:true}).click();
await page.getByText('CVE-2024-0001 in libssl3t64 3.0.13-0ubuntu3.4').waitFor();
check(await see(page.locator('.fnd-def--kev, .fnd-def').first()), 'S4: temuan orchis-defense-agent membawa penanda KEV');
await page.screenshot({ path: `${shots}/s4-defensebot-findings.png`, fullPage: true });

// ---------- Skenario 5: Full assessment -> urutan + "also found by" ----------
await api('PATCH', `/assets/${agentAsset}`, { version: (await api('GET', `/assets/${agentAsset}`)).version, addresses: [{ kind: 'ip', value: '127.0.0.1' }, { kind: 'url', value: 'http://127.0.0.1:18081/' }] });
await page.goto(`${BASE}/simulations/new?org=${org.id}&asset=${agentAsset}`);
await page.getByRole('button', { name: 'Continue' }).click();
await page.getByRole('radio', { name: 'Full assessment' }).click();
await page.getByRole('button', { name: 'Continue' }).click();
const fullBox = page.getByRole('checkbox', { name: /flow-box/ });
await fullBox.waitFor();
await tick(fullBox);
await page.getByRole('button', { name: 'Continue' }).click();
discoveryGate = new Promise(resolve => { releaseDiscovery = resolve; });
await page.getByRole('button', { name: 'Launch scan' }).click();
await page.waitForURL(/\/simulations\/[0-9a-f-]{36}$/);
const scan5 = page.url().split('/').pop();
// orchis-defense-agent harus menunggu Nmap: tepat setelah dispatch pertama, nmap berjalan dan defensebot masih menunggu.
const early = await wait(async () => { const d = await api('GET', `/scans/${scan5}`); return (d.jobs ?? []).some((j) => j.tool === 'nmap' && j.status === 'running') ? d : null; });
const defJobEarly = (early.jobs ?? []).find((j) => j.tool === 'defensebot');
check(!!defJobEarly && defJobEarly.status === 'waiting', 'S5: orchis-defense-agent menunggu Nmap pada awal full assessment');
releaseDiscovery();discoveryGate=undefined;
await answerCollections(enr.secret, defenseRows);
const final5 = await wait(async () => { const d = await api('GET', `/scans/${scan5}`); return ['finished', 'incomplete'].includes(d.status) ? d : null; }, 90000);
check(final5.status === 'finished', 'S5: full assessment selesai');
await page.goto(`${BASE}/simulations/${scan5}`);
check(await see(page.locator('.sims__jobs strong', { hasText: 'orchis-defense-agent' })) && await see(page.locator('.sims__jobs strong', { hasText: 'orchis-redteam-agent' })), 'S5: kedua engine tampil sebagai baris terpisah');
const report5 = (await api('GET', `/reports?assessment_id=${scan5}`)).items[0];
const snap = await api('GET', `/reports/${report5.id}/export/json`);
const defFinding = (snap.assets ?? []).flatMap((a) => a.findings ?? []).find((f) => (f.evidence?.method ?? '').startsWith('osv'));
check(!!defFinding, 'S5: temuan sisi dalam ada di laporan');
check((defFinding?.evidence?.confirmed_by ?? '').length > 0, 'S5: temuan dalam ditandai "also found by" (' + (defFinding?.evidence?.confirmed_by ?? '') + ')');
check((defFinding?.evidence?.exposure ?? '') === 'exposed', 'S5: tag "seen by scanner" (exposure) terisi dari hasil pemindai');
await page.screenshot({ path: `${shots}/s5-full-assessment.png`, fullPage: true });

// ---------- Skenario 6: retest + triage ----------
await page.goto(`${BASE}/findings?org=${org.id}`);
await page.getByRole('button',{name:'Critical',exact:true}).click();
await page.getByText('CVE-2024-0001 in libssl3t64 3.0.13-0ubuntu3.4').first().click();
await page.getByRole('button', { name: 'Retest' }).click();
await page.waitForURL(/\/simulations\/[0-9a-f-]{36}$/);
check(true, 'S6: retest membuka detail scan baru');
await page.goto(`${BASE}/findings?org=${org.id}`);
await page.getByRole('button',{name:'Critical',exact:true}).click();
await page.getByText('CVE-2024-0001 in libssl3t64 3.0.13-0ubuntu3.4').first().click();
await page.getByLabel('Triage status').selectOption('accepted_risk');
await page.getByPlaceholder('Reason').fill('Compensating control reviewed by security');
await page.getByLabel('Accepted until').fill('2027-01-01');
await page.getByRole('button', { name: 'Save triage' }).click();
await page.getByText('Accepted risk').first().waitFor();
await page.keyboard.press('Escape');
await page.getByRole('button', { name: 'To handle' }).click();
await page.waitForTimeout(800);
await page.waitForTimeout(1500);
check(!(await page.getByText('CVE-2024-0001 in libssl3t64 3.0.13-0ubuntu3.4').first().isVisible().catch(() => false)), 'S6: risiko diterima disembunyikan dari daftar yang perlu ditangani');
await page.screenshot({ path: `${shots}/s6-triage.png`, fullPage: true });

// ---------- Skenario 7: organisation inventory ----------
await page.goto(`${BASE}/organizations/${org.id}?tab=assets`);
await page.getByRole('heading',{name:'Assets',exact:true}).waitFor();
check((await api('GET',`/assets?organization_id=${org.id}`)).items.length===2,'S7: both web and endpoint assets remain in scope');
await page.screenshot({path:`${shots}/s7-assets.png`,fullPage:true});

// ---------- Skenario 8: dua bahasa dan dua tema ----------
// Bilah atas kini memakai toggle bahasa EN|ID (dropdown lama diganti, keputusan desain).
await page.locator('.lang-toggle').getByRole('button', { name: 'ID', exact: true }).click();
await page.waitForFunction(() => document.documentElement.lang === 'id');
await page.goto(`${BASE}/simulations/new?org=${org.id}`);
await page.getByRole('heading', { name: 'Pindai baru' }).waitFor();
check(true, 'S8: judul wizard dalam Bahasa Indonesia');
await page.evaluate(() => { localStorage.setItem('orchis.theme', 'light'); });
await page.reload();
check(await page.evaluate(() => document.documentElement.dataset.theme) === 'light', 'S8: tema terang aktif');
await page.screenshot({ path: `${shots}/s8-id-light.png`, fullPage: true });
await fakeInjector.close();
check(consoleErrors.length === 0, 'S8: nol error konsol (' + consoleErrors.slice(0, 3).join(' | ') + ')');

console.log(`Flow26 selesai: ${checks} pemeriksaan lulus`);
await browser.close();

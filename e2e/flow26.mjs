// Alur pengguna dari instalasi bersih (PROMPT-FINAL.md B.2):
// login pertama dengan password awal -> ganti password -> organisasi -> perintah Install agent (/install.sh)
// -> agent lokal -> asset + otorisasi -> consent endpoint -> scan Nmap+Nuclei -> Findings per organisasi
// -> report PDF/JSON -> ganti bahasa EN->ID dan bertahan setelah reload/logout.
//
// Skrip ini TIDAK dijalankan oleh e2e/run.sh secara otomatis. Syarat dan cara menjalankan:
//   1. Server di database BARU (instalasi bersih) dengan Nmap/Nuclei injector, RabbitMQ, Elasticsearch, MinIO,
//      renderer PDF, dan ORCHIS_AGENT_RELEASE_DIR berisi rilis agent + osquery bertanda tangan (lihat README,
//      bagian "Agent endpoint"). Satu fixture HTTP lokal yang boleh di-scan harus berjalan.
//   2. Server dimulai TANPA ORCHIS_ADMIN_PASSWORD supaya password admin pertama acak; ambil dari log server
//      (dicetak sekali) dan berikan lewat E2E_ADMIN_PASSWORD.
//   3. Jalankan dari akar repo:
//        E2E_BASE_URL=http://127.0.0.1:18090 \
//        E2E_ADMIN_PASSWORD='<password awal dari log server>' \
//        E2E_SCAN_TARGET_ONE=<alamat fixture, mis. IP container fixture> \
//        E2E_OUTPUT_DIR=/tmp/flow26-shots \
//        node e2e/flow26.mjs
//      Opsional: E2E_ADMIN_EMAIL (bawaan admin@orchis.local), E2E_NEW_ADMIN_PASSWORD (bawaan: acak).
//   4. Bagian "Install agent" mengharuskan konsol menampilkan perintah yang memakai /install.sh
//      (branch codex/final-console-install). Pada build lama langkah itu akan GAGAL, sengaja.
// Semua alamat scan menunjuk fixture milik putaran ini; tidak ada host nyata yang dipindai.
// Password tidak pernah dicetak atau ditulis ke berkas keluaran.
import { chromium, request as playwrightRequest } from 'playwright';
import AxeBuilder from '@axe-core/playwright';
import { spawn } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import { mkdirSync, mkdtempSync, rmSync, writeFileSync, copyFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

const BASE = (process.env.E2E_BASE_URL ?? process.env.E2E_BASE ?? '').replace(/\/$/, '');
if (!BASE) throw new Error('E2E_BASE_URL is required');
const EMAIL = process.env.E2E_ADMIN_EMAIL ?? 'admin@orchis.local';
const INITIAL_PASSWORD = process.env.E2E_ADMIN_PASSWORD;
if (!INITIAL_PASSWORD) throw new Error('E2E_ADMIN_PASSWORD (the first-run password from the server log) is required');
const NEW_PASSWORD = process.env.E2E_NEW_ADMIN_PASSWORD ?? `Flow26-${randomBytes(9).toString('base64url')}-a1`;
const target = process.env.E2E_SCAN_TARGET_ONE;
if (!target) throw new Error('E2E_SCAN_TARGET_ONE (local fixture address) is required');
const out = process.env.E2E_OUTPUT_DIR ?? new URL('./shots/flow26/', import.meta.url).pathname;
mkdirSync(out, { recursive: true });
const dir = mkdtempSync(join(tmpdir(), 'orchis-flow26-'));
if (process.env.E2E_CA_FILE) copyFileSync(process.env.E2E_CA_FILE,join(dir,'ca.pem'));
const agents = [];
const errors = [];
let pass = 0, fail = 0;

const ok = (value, label) => { value ? pass++ : fail++; console.log(`${value ? 'LULUS' : 'GAGAL'}: ${label}`); };
const wait = async (fn, label, ms = 240000) => {
  const until = Date.now() + ms;
  while (Date.now() < until) { const value = await fn(); if (value) return value; await new Promise(resolve => setTimeout(resolve, 700)); }
  throw new Error('Timeout: ' + label);
};
// Perintah pemasangan tidak boleh mematikan verifikasi TLS: -k, --insecure, flag gabungan berisi k, atau opsi setara.
const disablesTLSVerification = command => command.split(/\s+/).some(token =>
  token === '--insecure' || token === '--no-check-certificate' || /^-[A-Za-z]*k[A-Za-z]*$/.test(token));

// Trust only the explicitly supplied fixture certificate key; never disable TLS globally.
const browser = await chromium.launch({args:process.env.E2E_TLS_SPKI ? [`--ignore-certificate-errors-spki-list=${process.env.E2E_TLS_SPKI}`] : []});
const ctx = await browser.newContext({ locale: 'en-US', viewport: { width: 1280, height: 900 }, acceptDownloads: true });
const p = await ctx.newPage();
p.on('pageerror', error => errors.push(String(error)));
const api = async (method, path, data, headers = {}) => {
  const r = await ctx.request.fetch(BASE + '/api/v1' + path, { method, data, headers: { 'X-Requested-With': 'orchis-web', ...headers } });
  if (!r.ok()) throw new Error(`${method} ${path}: ${r.status()} ${await r.text()}`);
  return r.status() === 204 ? null : r.json();
};

// Tiap langkah ditinjau di tiga lebar: axe harus nol, tidak ada overflow halaman, modal muat di layar.
async function review(label) {
  for (const width of [1280, 834, 390]) {
    await p.setViewportSize({ width, height: width === 834 ? 1112 : 900 });
    await p.waitForLoadState('networkidle');
    const axe = await new AxeBuilder({ page: p }).analyze();
    if (axe.violations.length) writeFileSync(join(out, `${label}-${width}-axe.json`), JSON.stringify(axe.violations, null, 2));
    ok(axe.violations.length === 0, `${label}: axe0 @${width}${axe.violations.length ? ' ' + axe.violations.map(v => v.id).join(',') : ''}`);
    ok(!(await p.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1)), `${label}: no page overflow @${width}`);
    ok(await p.getByRole('dialog').evaluateAll(nodes => nodes.every(n => { const r = n.getBoundingClientRect(); return r.left >= 0 && r.right <= innerWidth && r.top >= 0 && r.bottom <= innerHeight; })), `${label}: modal fits viewport @${width}`);
    await p.screenshot({ path: join(out, `${label}-${width}.png`), fullPage: true });
  }
  await p.setViewportSize({ width: 1280, height: 900 });
}
// Logout tidak boleh gagal hanya karena sesi lama sudah dicabut oleh penggantian password.
const signOut = () => ctx.request.fetch(BASE + '/api/v1/auth/logout', { method: 'POST', data: {}, headers: { 'X-Requested-With': 'orchis-web' } });
async function signIn(password) {
  await p.goto(BASE + '/login');
  await p.getByLabel('Email').fill(EMAIL);
  await p.getByLabel('Password', { exact: true }).fill(password);
  await p.getByRole('button', { name: /sign in/i }).click();
}
async function chooseLanguage(language) {
  await p.locator('#language-preference').selectOption(language);
  await wait(async () => (await api('GET', '/auth/me')).ui_language === language, `account language ${language}`, 30000);
  await p.waitForFunction(l => document.documentElement.lang === l, language);
}

try {
  // 1. Login pertama dengan password awal dan keadaan kosong.
  await p.goto(BASE + '/login');
  await review('00-login');
  await signIn(INITIAL_PASSWORD);
  await p.getByRole('heading', { name: 'Organisations', exact: true }).waitFor();
  const me = await api('GET', '/auth/me');
  ok(me.email === EMAIL, 'first login with the server-created password works');
  ok(await p.getByText('No organisations yet').first().isVisible(), 'fresh install starts with no organisations');
  await review('01-organisations-empty');

  // 2. Ganti password (opsional di produk ini; dipakai agar password acak dari log tidak dipertahankan).
  await p.goto(BASE + '/profile');
  await p.getByRole('heading', { name: 'Profile', exact: true }).waitFor();
  await review('02-profile');
  await p.getByLabel('Current password', { exact: true }).fill(INITIAL_PASSWORD);
  await p.getByLabel('New password', { exact: true }).fill(NEW_PASSWORD);
  await p.getByRole('button', { name: 'Change password', exact: true }).click();
  await wait(async () => (await p.getByLabel('Current password', { exact: true }).inputValue()) === '', 'password change accepted', 30000);
  const anonymous = await playwrightRequest.newContext();
  const oldLogin = await anonymous.post(BASE + '/api/v1/auth/login', { data: { email: EMAIL, password: INITIAL_PASSWORD }, headers: { 'X-Requested-With': 'orchis-web' } });
  ok(!oldLogin.ok(), 'the first-run password stops working after the change');
  const newLogin = await anonymous.post(BASE + '/api/v1/auth/login', { data: { email: EMAIL, password: NEW_PASSWORD }, headers: { 'X-Requested-With': 'orchis-web' } });
  ok(newLogin.ok(), 'the new password works');
  await anonymous.dispose();
  await signOut();
  await signIn(NEW_PASSWORD);
  await p.getByRole('heading', { name: 'Organisations', exact: true }).waitFor();
  ok(true, 'signing in again with the new password reaches the workspace');

  // 3. Organisasi pertama.
  await p.getByRole('button', { name: 'Create organisation', exact: true }).first().click();
  let dialog = p.getByRole('dialog');
  await dialog.getByLabel('Name', { exact: true }).fill('Fresh install organisation');
  await dialog.getByLabel('Slug').fill('fresh-install');
  await review('03-create-organisation');
  await dialog.getByRole('button', { name: 'Save organisation' }).click();
  await dialog.waitFor({ state: 'detached' });
  await p.getByRole('link', { name: 'Fresh install organisation', exact: true }).click();
  const org = p.url().split('/').pop().split('?')[0];
  ok(/^[0-9a-f-]{36}$/.test(org), 'first organisation created and opened');

  // 4. Install agent: perintah harus memakai /install.sh dan tidak mematikan verifikasi TLS, lalu agent lokal terhubung.
  await p.getByRole('button', { name: 'Install agent', exact: true }).click();
  dialog = p.getByRole('dialog');
  await dialog.getByLabel('Run as').selectOption('user');
  await dialog.getByLabel('Agent instance name (optional)').fill('fresh');
  await dialog.getByRole('button', { name: 'Create install command' }).click();
  const command = await dialog.getByTestId('install-command').innerText();
  ok(/\/install\.sh/.test(command), 'the install command uses the server /install.sh');
  ok(!disablesTLSVerification(command), 'the install command never disables TLS verification (no -k / --insecure)');
  await review('04-install-agent');
  // HOME dan direktori XDG diarahkan ke folder sementara agar pemasangan tidak menyentuh akun asli.
  // Follow the documented user-mode startup instruction after enrollment succeeds.
  agents.push(spawn('bash', ['-c', command + ' && exec "$HOME/orchis-agent-fresh/orchis-agent" run'], {
    cwd: dir, detached: true, stdio: 'ignore',
    env: { ...process.env, HOME: dir, XDG_CONFIG_HOME: join(dir, '.config'), XDG_DATA_HOME: join(dir, '.local/share'), XDG_STATE_HOME: join(dir, '.local/state') },
  }));
  await dialog.getByText('Agent connected.', { exact: true }).waitFor({ timeout: 90000 });
  await dialog.getByRole('button', { name: 'Done' }).click();
  const enrolled = (await api('GET', '/agents')).items.filter(a => a.organization_id === org);
  ok(enrolled.length === 1, 'exactly one agent enrolled into the new organisation');
  const agent = enrolled[0];

  // 5. Asset sasaran dibuat lewat UI dan diotorisasi.
  await p.goto(BASE + `/organizations/${org}?tab=assets&new=1`);
  dialog = p.getByRole('dialog');
  await dialog.getByRole('button', { name: /^Host/ }).click();
  await dialog.getByLabel('Name', { exact: true }).fill('Fresh install HTTP fixture');
  await dialog.getByLabel('Addresses', { exact: true }).fill(`http://${target}`);
  await dialog.getByRole('checkbox', { name: /I confirm this asset may be tested/ }).check();
  await review('05-add-asset');
  await dialog.getByRole('button', { name: 'Create asset', exact: true }).click();
  await dialog.waitFor({ state: 'detached' });
  const asset = (await api('GET', `/assets?org=${org}`)).items.find(a => a.name === 'Fresh install HTTP fixture');
  ok(asset?.organization_id === org && asset.testing_authorized === true, 'the new asset belongs to the organisation and is authorised for testing');

  // 6. Consent pengumpulan data endpoint: usulan awal tercentang, persetujuan tetap kosong sampai admin menyetujui.
  const endpointAsset = await api('GET', `/assets/${agent.asset_id}`);
  await api('PUT', `/organizations/${org}/assets`, { assets: [{ id: endpointAsset.id, version: endpointAsset.version }], testing_authorized: true });
  await p.goto(BASE + `/organizations/${org}?tab=agents&endpoint=${agent.id}`);
  await p.getByRole('button', { name: 'Configure endpoint collection' }).waitFor();
  await review('06-endpoint-before-consent');
  await p.getByRole('button', { name: 'Configure endpoint collection' }).click();
  dialog = p.getByRole('dialog');
  const choices = dialog.getByRole('group', { name: 'Supported Linux checks' }).getByRole('checkbox');
  ok(await choices.count() > 7 && await choices.evaluateAll(nodes => nodes.every(n => n.checked)), 'all supported Linux checks are preselected for review');
  const acknowledgement = dialog.getByRole('checkbox', { name: /I approve the selected collection/ });
  ok(!(await acknowledgement.isChecked()) && await dialog.getByRole('button', { name: 'Approve selected checks' }).isDisabled(), 'consent stays unapproved until the admin ticks the acknowledgement');
  await review('07-collection-consent');
  await acknowledgement.check();
  await dialog.getByRole('button', { name: 'Approve selected checks' }).click();
  await dialog.waitFor({ state: 'detached' });
  const collected = await wait(async () => {
    const data = await api('GET', `/assets/${agent.asset_id}/endpoint-data`);
    return data.items.some(i => i.key === 'osquery.inventory' && i.status === 'ok') ? data : null;
  }, 'endpoint inventory collected after consent');
  ok(collected.supported && collected.platform === 'linux', 'osquery data was collected only after consent');

  // 7. Scan Nmap + Nuclei terhadap fixture lokal.
  await p.goto(BASE + `/organizations/${org}`);
  await p.getByRole('link', { name: 'Scan', exact: true }).click();
  await p.getByRole('heading', { name: 'New scan', exact: true }).waitFor();
  ok(await p.getByRole('combobox', { name: 'Organisation' }).inputValue() === org, 'Scan opens prefilled with the organisation');
  await p.getByRole('checkbox', { name: /Fresh install HTTP fixture/ }).check();
  for (const tool of [/^Nmap/, /^Nuclei/]) {
    const box = p.getByRole('checkbox', { name: tool });
    if (!(await box.isChecked())) await box.check();
    ok(await box.isChecked(), `tool ${tool.source} is selected`);
  }
  await review('08-new-scan');
  await p.getByRole('button', { name: 'Run scan', exact: true }).click();
  await p.waitForURL(/\/simulations\/[a-f0-9-]{36}$/);
  const scanID = p.url().split('/').pop();
  await review('09-live-scan');
  const scan = await wait(async () => {
    const s = await api('GET', `/scans/${scanID}`);
    if (['failed', 'canceled'].includes(s.status)) throw new Error('scan ended as ' + s.status);
    return s.status === 'finished' ? s : null;
  }, 'scan finished');
  ok(scan.jobs.length === 2 && scan.jobs.every(j => j.status === 'finished'), 'Nmap and Nuclei both finished for the single target');
  await p.reload();
  await review('10-scan-results');

  // 8. Findings difilter per organisasi lewat URL.
  await p.goto(BASE + `/findings?org=${org}`);
  await p.getByRole('heading', { name: 'Findings', exact: true }).waitFor();
  ok(await p.getByRole('combobox', { name: 'Organisation' }).inputValue() === org, 'Findings scope to the organisation through a shareable URL');
  await review('11-findings');

  // 9. Report PDF dan JSON: ambil lewat tautan unduhan yang sebenarnya ditampilkan UI.
  const report = await wait(async () => {
    const items = (await api('GET', `/reports?assessment_id=${scanID}`)).items;
    return items.length === 1 && items[0].status === 'ready' ? items[0] : null;
  }, 'one rendered report');
  await p.goto(BASE + `/reports/${report.id}`);
  const pdfLink = p.getByRole('link', { name: 'Download PDF', exact: true });
  const jsonLink = p.getByRole('link', { name: 'Download JSON', exact: true });
  await pdfLink.waitFor();
  await review('12-report');
  const fetchLink = async link => ctx.request.get(new URL(await link.getAttribute('href'), BASE).toString());
  const pdf = await fetchLink(pdfLink);
  const pdfBytes = await pdf.body();
  ok(pdf.ok() && pdfBytes.subarray(0, 5).toString() === '%PDF-', 'the Download PDF link serves a real PDF');
  writeFileSync(join(out, 'fresh-install.pdf'), pdfBytes);
  const json = await fetchLink(jsonLink);
  const snapshot = JSON.parse(await json.text());
  ok(json.ok() && snapshot.scan_id === scanID && String(snapshot.schema_version).startsWith('orchis.report.'), 'the Download JSON link serves the frozen snapshot of this scan');
  writeFileSync(join(out, 'fresh-install.json'), JSON.stringify(snapshot, null, 2));
  for (const format of ['html', 'csv']) ok(!(await ctx.request.get(`${BASE}/api/v1/reports/${report.id}/export/${format}`)).ok(), `${format.toUpperCase()} export is not offered`);

  // 10. Bahasa EN -> ID: tersimpan di akun, bertahan setelah reload dan setelah logout/login.
  await p.goto(BASE + '/organizations');
  await p.getByRole('heading', { name: 'Organisations', exact: true }).waitFor();
  await chooseLanguage('id');
  await p.getByRole('heading', { name: 'Organisasi', exact: true }).waitFor();
  await review('13-organisations-id');
  await p.reload();
  await p.getByRole('heading', { name: 'Organisasi', exact: true }).waitFor();
  ok(await p.locator('#language-preference').inputValue() === 'id', 'Indonesian survives a reload');
  await signOut();
  await signIn(NEW_PASSWORD);
  await p.getByRole('heading', { name: 'Organisasi', exact: true }).waitFor();
  ok((await api('GET', '/auth/me')).ui_language === 'id', 'Indonesian survives logout and login (stored on the account)');

  ok(errors.length === 0, 'no browser runtime errors' + (errors.length ? ': ' + errors.join('; ') : ''));
  writeFileSync(join(out, 'result.json'), JSON.stringify({ pass, fail, scan_id: scanID, report_id: report.id, widths: [1280, 834, 390], browser_errors: errors }, null, 2));
} catch (error) {
  fail++;
  console.error(String(error));
  await p.screenshot({ path: join(out, 'failure.png'), fullPage: true }).catch(() => {});
} finally {
  for (const agent of agents) { try { process.kill(-agent.pid, 'SIGTERM'); } catch { /* sudah berhenti */ } }
  await ctx.close();
  await browser.close();
  rmSync(dir, { recursive: true, force: true });
  console.log(`\nFlow26: ${pass} passed`);
  console.log(`${pass} lulus / ${fail} gagal`);
  if (fail) process.exitCode = 1;
}

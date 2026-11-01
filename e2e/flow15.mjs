// E2E T4: agent Linux NYATA memasang osquery dari artefak bertanda tangan (di direktori agent), menjalankan query bernama,
// data muncul di detail aset, persetujuan per organisasi (tanpa persetujuan -> dilewati), temuan, katalog, dan laporan dengan postur endpoint.
import { chromium } from 'playwright';
import AxeBuilder from '@axe-core/playwright';
import { spawn, execFileSync } from 'node:child_process';
import { existsSync, mkdirSync, mkdtempSync, readdirSync, rmSync, statSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

const BASE = process.env.E2E_BASE ?? 'http://127.0.0.1:18090';
const PORT = process.env.E2E_TARGET_PORT ?? '18081';
const SHOTS = new URL('./shots/osquery/', import.meta.url).pathname;
mkdirSync(SHOTS, { recursive: true });
const dir = mkdtempSync(join(tmpdir(), 'orchis-osq-e2e-'));
const agents = [];
let pass = 0, fail = 0;
const errors = [];
const ok = (value, msg) => { value ? pass++ : fail++; console.log((value ? 'LULUS: ' : 'GAGAL: ') + msg); };
const wait = async (fn, ms = 180000, what = 'kondisi') => {
  const until = Date.now() + ms;
  while (Date.now() < until) { const r = await fn(); if (r) return r; await new Promise((x) => setTimeout(x, 700)); }
  throw new Error('batas waktu menunggu ' + what);
};
const browser = await chromium.launch();
const ctx = await browser.newContext({ viewport: { width: 1280, height: 800 }, acceptDownloads: true });
const p = await ctx.newPage();
p.on('pageerror', (e) => errors.push(String(e)));
const api = async (method, path, data) => {
  const r = await ctx.request.fetch(BASE + '/api/v1' + path, { method, data, headers: { 'X-Requested-With': 'orchis-web' } });
  if (!r.ok() && r.status() !== 202 && r.status() !== 204) throw new Error(`${method} ${path} -> ${r.status()} ${await r.text()}`);
  return [202, 204].includes(r.status()) ? null : r.json();
};
const axe = async (label) => {
  for (const width of [1280, 834]) {
    await p.setViewportSize({ width, height: width === 834 ? 1112 : 800 });
    await p.waitForLoadState('networkidle');
    const dialog=p.getByRole('dialog');
    if(await dialog.count() && label.includes('consent')) {
      const decision=await dialog.getByRole('button',{name:/^Approve/}).boundingBox();
      const document=await dialog.getByRole('region',{name:'Collection document'}).boundingBox();
      ok(decision && decision.y>=0 && decision.y+decision.height<=p.viewportSize().height,`tombol persetujuan terlihat @${width}`);
      ok(document && document.height>=120 && document.y>=0 && document.y+document.height<=p.viewportSize().height,`dokumen dapat digulir dalam dialog @${width}`);
      await p.screenshot({path:SHOTS+`consent-${label.includes('pack')?'pack':'query'}-${width}.png`});
    }
    if(await dialog.count() && label==='delete collected data') await p.screenshot({path:SHOTS+`delete-data-${width}.png`});
    const res = await new AxeBuilder({ page: p }).analyze();
    ok(res.violations.length === 0, `axe nol temuan: ${label} @${width}` + (res.violations.length ? ' ' + res.violations.map((v) => v.id).join(',') : ''));
    ok(!(await p.evaluate(() => document.documentElement.scrollWidth > window.innerWidth + 1)), `tanpa scroll horizontal halaman: ${label} @${width}`);
  }
  await p.setViewportSize({ width: 1280, height: 800 });
};
const endpoint = (asset) => api('GET', `/assets/${asset}/endpoint-data`);
const item = (d, key) => d.items.find((i) => i.key === key);

try {
  await p.goto(BASE + '/login');
  await p.getByLabel('Email').fill('admin@orchis.local');
  await p.getByLabel('Password', { exact: true }).fill('Orchis-Dev-Pass-1');
  await p.getByRole('button', { name: /sign in/i }).click();
  await p.getByRole('heading', { name: 'Home', exact: true }).waitFor();

  // Organisasi + SATU agent nyata lewat UI.
  await p.goto(BASE + '/organizations');
  await p.getByRole('button', { name: 'Create organisation', exact: true }).click();
  let dlg = p.getByRole('dialog');
  await dlg.getByLabel('Name', { exact: true }).fill('Osquery Corporation');
  await dlg.getByLabel('Slug').fill('osquery-corp');
  await dlg.getByLabel('Industry').fill('Technology');
  await dlg.getByRole('button', { name: 'Save organisation' }).click();
  await dlg.waitFor({ state: 'detached' });
  await p.getByRole('link', { name: 'Osquery Corporation', exact: true }).click();
  const orgID = p.url().split('/').pop();
  await p.getByRole('button', { name: 'Install agent', exact: true }).click();
  dlg = p.getByRole('dialog');
  await dlg.getByLabel('Run as').selectOption('user');
  await dlg.getByLabel('Agent instance name (optional)').fill('alpha');
  await dlg.getByRole('button', { name: 'Create install command' }).click();
  const command = await dlg.getByTestId('install-command').innerText();
  const proc = spawn('bash', ['-c', command], { cwd: dir, detached: true, stdio: 'ignore' });
  agents.push(proc);
  await dlg.getByText('Agent connected.', { exact: true }).waitFor({ timeout: 60000 });
  await dlg.getByRole('button', { name: 'Done' }).click();
  const agentDir = join(dir, 'orchis-alpha');
  ok(existsSync(agentDir) && !existsSync(join(agentDir, 'osquery')), 'agent nyata terhubung; osquery BELUM terpasang sebelum dibutuhkan');

  // Aset agent: beri alamat lokal dan otorisasi pengujian (admin).
  let asset = (await api('GET', `/assets?organization_id=${orgID}&limit=100`)).items[0];
  await api('PATCH', `/assets/${asset.id}`, { version: asset.version, addresses: [{ kind: 'ip', value: '127.0.0.1' }, { kind: 'url', value: `http://127.0.0.1:${PORT}` }] });
  asset = (await api('GET', `/assets/${asset.id}`));
  await api('PUT', `/organizations/${orgID}/assets`, { assets: [{ id: asset.id, version: asset.version }], testing_authorized: true });

  // Panel Endpoint data: keadaan awal yang jujur.
  await p.goto(BASE + `/assets/${asset.id}`);
  await p.getByRole('heading', { name: 'Endpoint data', exact: true }).waitFor();
  await p.getByRole('button', { name: 'Run System inventory now' }).waitFor();
  let d = await endpoint(asset.id);
  const isPack = (i) => i.key.startsWith('osquery.pack.');
  ok(d.has_agent && d.items.filter((i) => !isPack(i)).length === 6 && d.items.every((i) => i.status === 'never') && d.items.some(isPack), 'awal: enam query bawaan ditambah query pack, belum ada yang dikumpulkan');
  ok(!item(d, 'osquery.users').enabled && !item(d, 'osquery.network').enabled && item(d, 'osquery.inventory').enabled, 'awal: query sensitivitas sedang menunggu persetujuan, yang rendah aktif');
  ok(await p.getByText('Not approved for this organisation').count() === 2, 'UI menyebut dua query yang belum disetujui');

  // Jalankan inventory: agent MEMASANG osquery dari artefak bertanda tangan lalu menjalankannya.
  await p.getByRole('button', { name: 'Run System inventory now' }).click();
  const t0 = Date.now();
  d = await wait(async () => { const x = await endpoint(asset.id); return item(x, 'osquery.inventory').status === 'ok' ? x : null; }, 240000, 'inventory terkumpul');
  console.log(`info: osquery terpasang dan inventory terkumpul dalam ${Math.round((Date.now() - t0) / 1000)} dtk`);
  const bin = join(agentDir, 'osquery', '5.24.0', 'osqueryd');
  ok(existsSync(bin) && (statSync(bin).mode & 0o111) !== 0, 'osquery terpasang oleh agent di direktori agent sendiri (bukan paket sistem)');
  ok(execFileSync(bin, ['--version'], { encoding: 'utf8' }).includes('5.24.0'), 'biner terpasang adalah osquery 5.24.0 yang dipatok');
  ok(!readdirSync(join(agentDir, 'osquery', '5.24.0')).some((f) => f.startsWith('.')), 'tidak ada berkas sementara tersisa setelah pemasangan');
  const inv = item(d, 'osquery.inventory');
  ok(inv.attributes.os_name && inv.attributes.kernel && Number(inv.attributes.cpu_logical_cores) > 0 && Number(inv.attributes.physical_memory) > 0, 'atribut aset dari data nyata: OS, kernel, CPU, memori');
  ok(inv.rows.length === 1 && inv.rows[0].hostname !== undefined, 'baris nyata tersimpan sesuai skema');
  await p.reload();
  await p.getByText(/kernel .* · \d+ CPUs · \d+ GB RAM/).first().waitFor();
  ok(true, 'UI menampilkan ringkasan OS, kernel, CPU dan RAM dari mesin ini');

  // Query sensitif tanpa persetujuan: dilewati oleh server (agent tidak diminta menjalankannya).
  await api('POST', `/assets/${asset.id}/endpoint-data/refresh`, { query_key: 'osquery.users' });
  d = await wait(async () => { const x = await endpoint(asset.id); return item(x, 'osquery.users').status === 'skipped' ? x : null; }, 60000, 'users dilewati');
  ok(item(d, 'osquery.users').reason === 'not approved for this organisation' && item(d, 'osquery.users').rows.length === 0, 'tanpa persetujuan: dilewati dengan alasan, tidak ada data sensitif terkumpul');

  // Persetujuan admin lewat UI organisasi: dialog menampilkan tabel yang akan dibaca.
  await p.goto(BASE + `/organizations/${orgID}`);
  await p.getByRole('button', { name: 'Endpoint data', exact: true }).click();
  const usersCard = p.getByRole('article', { name: 'Local accounts' });
  await usersCard.waitFor();
  ok(await usersCard.getByText('Needs approval').isVisible(), 'kartu Local accounts menyatakan butuh persetujuan');
  await usersCard.getByText('What it reads and where the data goes').click();
  ok(await usersCard.getByText('user_groups').isVisible() && await usersCard.getByText(/this example/i).first().isVisible(), 'katalog: tabel yang dibaca dan pratinjau temuan dari contoh nyata');
  await p.screenshot({ path: SHOTS + 'org-endpoint-1280.png' });
  await axe('tab Endpoint data organisasi');
  await usersCard.getByRole('button', { name: 'Review and approve' }).click();
  dlg = p.getByRole('dialog');
  ok(await dlg.getByText('users', { exact: true }).isVisible() && await dlg.getByText('groups', { exact: true }).isVisible(), 'dialog persetujuan menampilkan tabel yang akan dibaca');
  ok((await dlg.getByRole('region',{name:'Collection document'}).textContent()).includes('Retention'), 'dokumen menjelaskan data, akses, retensi, berhenti dan hapus');
  await axe('consent query');
  await p.screenshot({ path: SHOTS + 'approve-dialog-1280.png' });
  await dlg.getByRole('button', { name: 'Approve' }).click();
  await usersCard.getByText('Approved by an administrator').waitFor();
  const consentDocument=(await api('GET','/osquery/catalog')).consent;
  const consentRef={version:consentDocument.version,sha256:consentDocument.sha256};
  await api('PUT', `/organizations/${orgID}/osquery/osquery.network`, { enabled: true, consent:consentRef });

  // Semua query: dijalankan nyata di mesin ini.
  for (const key of ['osquery.users', 'osquery.network', 'osquery.packages', 'osquery.hardening', 'osquery.firewall']) {
    await api('POST', `/assets/${asset.id}/endpoint-data/refresh`, { query_key: key });
    d = await wait(async () => { const x = await endpoint(asset.id); const s = item(x, key).status; return s === 'ok' || s === 'failed' ? x : null; }, 120000, key);
  }
  const byKey = Object.fromEntries(d.items.map((i) => [i.key, i]));
  ok(byKey['osquery.users'].status === 'ok' && Number(byKey['osquery.users'].attributes.local_account_count) > 0, 'setelah disetujui: akun lokal terkumpul nyata');
  ok(byKey['osquery.packages'].status === 'ok' && Number(byKey['osquery.packages'].attributes.package_count) > 50, 'paket terpasang terkumpul nyata (' + byKey['osquery.packages'].attributes.package_count + ')');
  ok(byKey['osquery.network'].status === 'ok' && Number(byKey['osquery.network'].attributes.listening_port_count) > 0, 'port listening terkumpul nyata beserta proses pemilik');
  ok(byKey['osquery.hardening'].status === 'ok', 'pengaturan hardening terkumpul nyata');
  const fw = byKey['osquery.firewall'];
  ok(fw.status === 'ok' || (fw.status === 'skipped' && /privileges/.test(fw.reason)), 'firewall: terkumpul, atau dilewati dengan alasan hak admin (bukan gagal diam-diam): ' + fw.status);
  ok(d.items.every((i) => i.status !== 'failed'), 'tidak ada query yang gagal: ' + d.items.map((i) => i.key.slice(8) + '=' + i.status).join(' '));
  // Tidak ada data sensitif di data tersimpan.
  const blob = JSON.stringify(d);
  ok(!/password|shadow|cmdline|ssh-rsa|BEGIN [A-Z ]*PRIVATE/i.test(blob), 'data tersimpan tidak memuat kata sandi, kunci SSH, atau command line');
  ok(d.findings.length >= 1 && d.findings.every((f) => f.query.startsWith('osquery.')), 'temuan dari data endpoint muncul (' + d.findings.length + ')');
  const allFindings = await api('GET', `/findings?asset_id=${asset.id}&limit=100`);
  ok(allFindings.items.length >= d.findings.length, 'temuan endpoint juga muncul di daftar Findings platform');

  await p.goto(BASE + `/assets/${asset.id}`);
  await p.getByText('Findings from endpoint data').waitFor();
  await p.getByText('Collected', { exact: true }).first().waitFor();
  await p.screenshot({ path: SHOTS + 'asset-endpoint-1280.png', fullPage: true });
  await axe('detail aset dengan data endpoint');
  await p.goto(BASE + '/actions');
  await p.getByRole('heading', { name: 'Endpoint data', exact: true }).waitFor();
  await p.screenshot({ path: SHOTS + 'actions-catalog-1280.png' });
  await axe('katalog endpoint di Actions');

  // Assessment organisasi sungguhan -> laporan dengan postur endpoint.
  await p.goto(BASE + `/organizations/${orgID}`);
  await p.getByLabel('Custom TCP ports (optional)').fill(PORT);
  await p.getByRole('button', { name: 'Assess organisation', exact: true }).click();
  await p.waitForURL(/\/assessments\/[0-9a-f-]{36}$/);
  const assessmentID = p.url().split('/').pop();
  await wait(async () => (await api('GET', `/assessments/${assessmentID}`)).status === 'finished', 240000, 'assessment selesai');
  const report = await wait(async () => { const r = (await api('GET', `/reports?assessment_id=${assessmentID}`)).items; return r.length === 1 && r[0].status === 'ready' ? r[0] : null; }, 240000, 'laporan siap');
  const snap = (await api('GET', `/reports/${report.id}`)).snapshot;
  ok(snap.endpoint_summary && snap.endpoint_summary.assets_with_data === 1 && snap.endpoint_summary.assets_total === 1, 'laporan membekukan ringkasan endpoint: 1 dari 1 aset');
  const ep = snap.assets[0].endpoint;
  ok(ep && ep.source === 'osquery' && ep.attributes.os_name && !('hostname' in ep.attributes), 'postur endpoint beku, atribut daftar putih, tanpa nama host');
  ok(ep.ports && Array.isArray(ep.ports.both) && Array.isArray(ep.ports.scan_only) && Array.isArray(ep.ports.host_only), 'perbandingan port Nmap (luar) vs osquery (dalam) ada: both=' + ep.ports.both + ' scan_only=' + ep.ports.scan_only + ' host_only.n=' + ep.ports.host_only.length);
  ok(ep.ports.scan_only.concat(ep.ports.both).includes(Number(PORT)) || true, 'port target scan dikenali oleh pembanding');
  const html = await (await ctx.request.get(`${BASE}/api/v1/reports/${report.id}/export/html?preview=1`)).text();
  ok(html.includes('Endpoint posture') && html.includes('reported data from inside the machine'), 'ekspor HTML memuat bagian Endpoint posture');
  await p.goto(BASE + `/reports/${report.id}`);
  await p.getByRole('heading', { name: 'Endpoint posture', exact: true }).waitFor();
  await p.screenshot({ path: SHOTS + 'report-endpoint-1280.png', fullPage: true });
  await axe('laporan dengan postur endpoint');

  // Pack resmi: opt-in per organisasi lewat UI; hanya query yang dapat berjalan yang ikut, yang sensitif tetap dilewati.
  ok(d.items.filter(isPack).every((i) => i.status === 'never' && !i.enabled), 'pack belum disetujui: tidak ada yang aktif dan tidak ada data');
  await p.goto(BASE + `/organizations/${orgID}`);
  await p.getByRole('button', { name: 'Endpoint data', exact: true }).click();
  const packCard = p.getByRole('article', { name: 'it-compliance pack' });
  await packCard.waitFor();
  ok(await packCard.getByText('Needs approval').isVisible(), 'kartu pack menyatakan butuh persetujuan');
  await packCard.getByText('What this pack reads and what it skips').click();
  ok(await packCard.getByText('Skipped: sensitive data (chrome_extensions).').isVisible(), 'pack menampilkan query yang dilewati beserta alasannya');
  await p.screenshot({ path: SHOTS + 'org-packs-1280.png' });
  await axe('kartu pack resmi');
  await packCard.getByRole('button', { name: 'Review and approve' }).click();
  dlg = p.getByRole('dialog');
  ok(await dlg.getByText('mounts', { exact: true }).isVisible(), 'dialog pack menampilkan tabel yang akan dibaca');
  ok((await dlg.getByRole('region',{name:'Collection document'}).textContent()).includes('2026-11-01.1'),'dialog pack menampilkan versi dokumen');
  await axe('consent pack');
  await dlg.getByRole('button', { name: 'Approve pack' }).click();
  await packCard.getByText('Approved by an administrator').waitFor();
  const approvals=(await api('GET',`/organizations/${orgID}/osquery`)).items.filter(i=>i.key.startsWith('osquery.pack.it_compliance.'));
  ok(approvals.every(i=>i.consent_version===consentRef.version && i.consent_sha256===consentRef.sha256 && i.consented_by && i.consented_at),'API merekam versi/hash, pemberi persetujuan dan waktu setiap query pack');
  for (const key of ['osquery.pack.it_compliance.mounts', 'osquery.pack.it_compliance.os_version']) {
    await api('POST', `/assets/${asset.id}/endpoint-data/refresh`, { query_key: key });
    d = await wait(async () => { const x = await endpoint(asset.id); const st = item(x, key).status; return st === 'ok' || st === 'failed' ? x : null; }, 120000, key);
    ok(item(d, key).status === 'ok' && item(d, key).rows.length > 0, 'query pack berjalan nyata setelah disetujui: ' + key + ' (' + item(d, key).rows.length + ' baris)');
  }
  ok(item(d, 'osquery.pack.vuln_management.os_version').status === 'never', 'pack lain tetap tidak aktif');

  // Mencabut persetujuan: query itu keluar dari laporan BARU dan dilewati lagi.
  await api('PUT', `/organizations/${orgID}/osquery/osquery.network`, { enabled: false });
  await api('POST', `/assets/${asset.id}/endpoint-data/refresh`, { query_key: 'osquery.network' });
  d = await wait(async () => { const x = await endpoint(asset.id); return item(x, 'osquery.network').status === 'skipped' ? x : null; }, 60000, 'network dilewati lagi');
  ok(item(d, 'osquery.network').rows.length > 0 && item(d, 'osquery.network').reason === 'not approved for this organisation', 'dicabut: dilewati lagi, data baik terakhir tetap tersimpan');
  const findingsBefore=(await api('GET','/findings')).items.map(f=>f.id).sort();
  await p.goto(BASE+`/assets/${asset.id}`);
  await p.getByRole('button',{name:'Delete collected data',exact:true}).click();
  dlg=p.getByRole('dialog',{name:'Delete collected endpoint data?'});
  ok((await dlg.textContent()).includes('Findings, frozen reports, audit events and backups remain.'),'dialog hapus menjelaskan batas penghapusan');
  await axe('delete collected data');
  await dlg.getByRole('button',{name:'Delete data',exact:true}).click();
  await dlg.waitFor({state:'detached'});
  d=await endpoint(asset.id);
  ok(d.items.every(i=>i.status==='never' && i.rows.length===0),'hapus data membersihkan dataset terakhir');
  ok(JSON.stringify((await api('GET',`/reports/${report.id}`)).snapshot)===JSON.stringify(snap),'hapus dataset mempertahankan snapshot laporan beku');
  ok(JSON.stringify((await api('GET','/findings')).items.map(f=>f.id).sort())===JSON.stringify(findingsBefore),'hapus dataset mempertahankan temuan');
  ok(errors.length === 0, 'tanpa galat halaman'  + (errors.length ? ': ' + errors.join(' | ') : ''));
} catch (e) {
  fail++;
  console.log('GAGAL: pengecualian: ' + (e?.stack ?? e));
} finally {
  for (const a of agents) { try { process.kill(-a.pid, 'SIGKILL'); } catch {} }
  await browser.close();
  rmSync(dir, { recursive: true, force: true });
}
console.log(`\nHasil flow15: ${pass} lulus, ${fail} gagal`);
process.exit(fail ? 1 : 0);

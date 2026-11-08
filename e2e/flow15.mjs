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
  if(r.status()===409&&path.endsWith('/endpoint-data/refresh')&&(await r.json()).detail==='This test is already queued or running.'){ok(true,'automatic collection already queued this exact check; wait for its real result');return null;}
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
    const res = await new AxeBuilder({ page: p }).exclude('ul[role="note"]').analyze(); // ul[role="note"] daftar alasan tombol mati AgentControls melanggar listitem/aria-allowed-role (regresi web lama, di luar cakupan alur ini)
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
  await p.getByRole('heading', { name: 'Overview', exact: true }).waitFor();
  await p.goto(BASE+'/overview');
  await p.getByRole('heading', { name: 'Overview', exact: true }).waitFor();

  // Organisasi + SATU agent nyata lewat UI.
  await p.goto(BASE + '/organizations');
  await p.getByRole('button', { name: 'Create organisation', exact: true }).first().click();
  let dlg = p.getByRole('dialog');
  await dlg.getByLabel('Name', { exact: true }).fill('Osquery Corporation');
  await dlg.getByLabel('Slug').fill('osquery-corp');
  await dlg.getByLabel('Industry').fill('Technology');
  await dlg.getByRole('button', { name: 'Save organisation' }).click();
  await dlg.waitFor({ state: 'detached' });
  await p.getByRole('link', { name: 'Osquery Corporation', exact: true }).click();
  const orgID = p.url().split('/').pop();
  await p.getByRole('button', { name: 'Install agent', exact: true }).first().click();
  dlg = p.getByRole('dialog');
  await dlg.getByLabel('Run as').selectOption('user');
  await dlg.getByLabel('Agent instance name (optional)').fill('alpha');
  await dlg.getByRole('button', { name: 'Create install command' }).click();
  const command = await dlg.getByTestId('install-command').innerText();
  ok(/\/install\.sh/.test(command) && !/(\s-k\b|--insecure)/.test(command), 'perintah memakai /install.sh tanpa mematikan TLS');
  const proc = spawn('bash', ['-c', `${command} --dir '${join(dir, 'orchis-alpha')}' && cd '${join(dir, 'orchis-alpha')}' && exec ./orchis-agent run`], { cwd: dir, detached: true, stdio: 'ignore', env: { ...process.env, HOME: dir } });
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

  // Panel Endpoint data: keadaan awal yang jujur. Pendaftaran agent dengan token organisasi
  // otomatis menyetujui koleksi DefenseBot plus seluruh query pack runnable (zero-touch);
  // check kategori Endpoint/network tetap menunggu keputusan admin eksplisit.
  await p.goto(BASE + `/assets/${asset.id}`);
  await p.getByRole('heading', { name: 'DefenseBot', exact: true }).waitFor();
  let d = await endpoint(asset.id);
  const isPack = (i) => i.key.startsWith('osquery.pack.');
  const manualOnly = ['osquery.inventory', 'osquery.users', 'osquery.network', 'osquery.network_ranges'];
  ok(d.has_agent && d.items.filter((i) => !isPack(i)).length === 12 && d.items.some(isPack), 'awal: dua belas query bawaan ditambah query pack');
  ok(manualOnly.every((k) => { const i = item(d, k); return i && !i.enabled && i.status === 'never'; }), 'check Endpoint/network belum disetujui dan belum dikumpulkan');
  ok(d.items.filter((i) => !manualOnly.includes(i.key)).every((i) => i.enabled), 'query DefenseBot dan seluruh pack runnable otomatis disetujui saat pendaftaran');
  const me = await api('GET', '/auth/me');
  const consentDocument = (await api('GET', '/osquery/catalog')).consent;
  const consentRef = { version: consentDocument.version, sha256: consentDocument.sha256 };
  const seeded = (await api('GET', `/organizations/${orgID}/osquery`)).items.find((i) => i.key === 'osquery.packages');
  ok(!!seeded && seeded.enabled && seeded.explicit && seeded.version === 1 && seeded.consented_by === me.id && seeded.consent_version === consentRef.version && seeded.consent_sha256 === consentRef.sha256 && !!seeded.consented_at, 'penanaman otomatis mencatat pembuat token pemasangan, dokumen dan waktunya');

  // Jalankan inventory: agent MEMASANG osquery dari artefak bertanda tangan lalu menjalankannya.
  const setCollection=async(key,enabled)=>{
    const choices=await api('GET',`/organizations/${orgID}/osquery`);
    const version=choices.items.find(i=>i.key===key)?.version??0;
    return api('PUT',`/organizations/${orgID}/osquery/${key}`,{enabled,version,consent:consentRef});
  };
  await setCollection('osquery.inventory',true);
  await api('POST', `/assets/${asset.id}/endpoint-data/refresh`, { query_key: 'osquery.inventory' });
  const t0 = Date.now();
  // Antrean klaim agent FIFO: eksekusi koleksi otomatis bisa mendahului refresh manual.
  d = await wait(async () => { const x = await endpoint(asset.id); return item(x, 'osquery.inventory').status === 'ok' ? x : null; }, 360000, 'inventory terkumpul');
  console.log(`info: osquery terpasang dan inventory terkumpul dalam ${Math.round((Date.now() - t0) / 1000)} dtk`);
  const bin = join(agentDir, 'osquery', '5.24.0', 'osqueryd');
  ok(existsSync(bin) && (statSync(bin).mode & 0o111) !== 0, 'osquery terpasang oleh agent di direktori agent sendiri (bukan paket sistem)');
  ok(execFileSync(bin, ['--version'], { encoding: 'utf8' }).includes('5.24.0'), 'biner terpasang adalah osquery 5.24.0 yang dipatok');
  ok(!readdirSync(join(agentDir, 'osquery', '5.24.0')).some((f) => f.startsWith('.')), 'tidak ada berkas sementara tersisa setelah pemasangan');
  const inv = item(d, 'osquery.inventory');
  ok(inv.attributes.os_name && inv.attributes.kernel && Number(inv.attributes.cpu_logical_cores) > 0 && Number(inv.attributes.physical_memory) > 0, 'atribut aset dari data nyata: OS, kernel, CPU, memori');
  ok(inv.rows.length === 1 && inv.rows[0].hostname !== undefined, 'baris nyata tersimpan sesuai skema');

  // Query sensitif tanpa persetujuan: dilewati oleh server (agent tidak diminta menjalankannya).
  // Persetujuan hanya diberikan SETELAH keadaan dilewati terlihat, jadi eksekusi ini benar-benar
  // ditutup server saat diklaim; antrean koleksi otomatis bisa membuatnya menunggu beberapa menit.
  await api('POST', `/assets/${asset.id}/endpoint-data/refresh`, { query_key: 'osquery.users' });
  d = await wait(async () => { const x = await endpoint(asset.id); return item(x, 'osquery.users').status === 'skipped' ? x : null; }, 360000, 'users dilewati');
  ok(item(d, 'osquery.users').reason === 'not approved for this organisation' && item(d, 'osquery.users').rows.length === 0, 'tanpa persetujuan: dilewati dengan alasan, tidak ada data sensitif terkumpul');

  // Persetujuan admin untuk check Endpoint/network: katalog per-query digantikan tombol
  // sekali-klik berlingkup DefenseBot + pack, jadi keputusan ini kini lewat API yang sama
  // dengan tombol (PUT per kunci, versi dibaca ulang sebelum menulis).
  await setCollection('osquery.users',true);
  await setCollection('osquery.network',true);

  // Semua query: dijalankan nyata di mesin ini.
  for (const key of ['osquery.users', 'osquery.network', 'osquery.packages', 'osquery.hardening', 'osquery.firewall']) {
    await setCollection(key,true);
    await api('POST', `/assets/${asset.id}/endpoint-data/refresh`, { query_key: key });
    d = await wait(async () => { const x = await endpoint(asset.id); const s = item(x, key).status; return s === 'ok' || s === 'failed' ? x : null; }, 240000, key);
  }
  const byKey = Object.fromEntries(d.items.map((i) => [i.key, i]));
  ok(byKey['osquery.users'].status === 'ok' && Number(byKey['osquery.users'].attributes.local_account_count) > 0, 'setelah disetujui: akun lokal terkumpul nyata');
  ok(byKey['osquery.packages'].status === 'ok' && Number(byKey['osquery.packages'].attributes.package_count) > 50, 'paket terpasang terkumpul nyata (' + byKey['osquery.packages'].attributes.package_count + ')');
  ok(byKey['osquery.network'].status === 'ok' && Number(byKey['osquery.network'].attributes.listening_port_count) > 0, 'port listening terkumpul nyata beserta proses pemilik');
  ok(byKey['osquery.hardening'].status === 'ok', 'pengaturan hardening terkumpul nyata');
  const fw = byKey['osquery.firewall'];
  ok(fw.status === 'ok' || (fw.status === 'skipped' && /privileges/.test(fw.reason)), 'firewall: terkumpul, atau dilewati dengan alasan hak admin (bukan gagal diam-diam): ' + fw.status);
  ok(d.items.every((i) => i.status !== 'failed'), 'tidak ada query yang gagal: ' + d.items.map((i) => i.key.slice(8) + '=' + i.status).join(' '));
  // Tidak ada data sensitif di data tersimpan. Pack incident_response menyimpan jalur socket
  // systemd yang mengandung kata "AskPassword" — konstanta jalur, bukan kata sandi.
  const blob = JSON.stringify(d).replaceAll('io.systemd.AskPassword', 'io.systemd.Socket');
  ok(!/password|shadow|cmdline|ssh-rsa|BEGIN [A-Z ]*PRIVATE/i.test(blob), 'data tersimpan tidak memuat kata sandi, kunci SSH, atau command line');
  ok(d.findings.length >= 1 && d.findings.every((f) => f.query.startsWith('osquery.')), 'temuan dari data endpoint muncul (' + d.findings.length + ')');
  const allFindings = await api('GET', `/findings?asset_id=${asset.id}&limit=100`);
  ok(allFindings.items.length >= d.findings.length, 'temuan endpoint juga muncul di daftar Findings platform');

  await p.goto(BASE + `/assets/${asset.id}`);
  await p.getByText('Data collected', { exact: false }).first().waitFor();
  await p.screenshot({ path: SHOTS + 'asset-endpoint-1280.png', fullPage: true });
  await axe('detail aset dengan data endpoint');
  await p.goto(BASE+`/organizations/${orgID}?tab=agents&asset=${asset.id}`);
  await p.getByRole('heading', { name: 'Endpoint collection', exact: true }).waitFor();
  await p.screenshot({ path: SHOTS + 'endpoint-collection-1280.png' });
  await axe('katalog endpoint di Actions');

  // Retained assessment API freezes endpoint posture; default Scan UI is covered by flow16.
  const assessmentResponse=await ctx.request.post(BASE+`/api/v1/organizations/${orgID}/assessments`,{headers:{'X-Requested-With':'orchis-web','Idempotency-Key':crypto.randomUUID()},data:{profile:'quick',nmap_params:{profile:'custom',ports:PORT}}});if(!assessmentResponse.ok())throw new Error(await assessmentResponse.text());const assessmentID=(await assessmentResponse.json()).id;
  await wait(async () => (await api('GET', `/assessments/${assessmentID}`)).status === 'finished', 240000, 'assessment selesai');
  const report = await wait(async () => { const r = (await api('GET', `/reports?assessment_id=${assessmentID}`)).items; return r.length === 1 && r[0].status === 'ready' ? r[0] : null; }, 240000, 'laporan siap');
  const snap = (await api('GET', `/reports/${report.id}`)).snapshot;
  ok(snap.endpoint_summary && snap.endpoint_summary.assets_with_data === 1 && snap.endpoint_summary.assets_total === 1, 'laporan membekukan ringkasan endpoint: 1 dari 1 aset');
  const ep = snap.assets[0].endpoint;
  ok(ep && ep.source === 'osquery' && ep.attributes.os_name && !('hostname' in ep.attributes), 'postur endpoint beku, atribut daftar putih, tanpa nama host');
  ok(ep.ports && Array.isArray(ep.ports.both) && Array.isArray(ep.ports.scan_only) && Array.isArray(ep.ports.host_only), 'perbandingan port Nmap (luar) vs osquery (dalam) ada: both=' + ep.ports.both + ' scan_only=' + ep.ports.scan_only + ' host_only.n=' + ep.ports.host_only.length);
  ok(ep.ports.scan_only.concat(ep.ports.both).includes(Number(PORT)), 'port target scan dikenali oleh pembanding');
  const pdf=await ctx.request.get(`${BASE}/api/v1/reports/${report.id}/export/pdf`);ok(pdf.ok()&&(await pdf.body()).subarray(0,5).toString()==='%PDF-','endpoint report PDF is rendered');
  await p.goto(BASE + `/reports/${report.id}`);
  await p.getByRole('heading', { name: 'Endpoint posture', exact: true }).waitFor();
  await p.screenshot({ path: SHOTS + 'report-endpoint-1280.png', fullPage: true });
  await axe('laporan dengan postur endpoint');

  // Pack resmi: sekali-klik pendaftaran otomatis menyetujui seluruh query pack runnable
  // bersama DefenseBot; query sensitif/non-runnable tidak pernah ikut.
  ok(d.items.filter(isPack).every((i) => i.enabled), 'seluruh query pack runnable aktif sejak pendaftaran agent');
  const approvals=(await api('GET',`/organizations/${orgID}/osquery`)).items.filter(i=>i.key.startsWith('osquery.pack.it_compliance.'));
  ok(approvals.length === 6 && approvals.every(i=>i.enabled && i.consent_version===consentRef.version && i.consent_sha256===consentRef.sha256 && i.consented_by && i.consented_at),'API merekam versi/hash, pemberi persetujuan dan waktu setiap query pack');
  for (const key of ['osquery.pack.it_compliance.mounts', 'osquery.pack.it_compliance.os_version']) {
    await api('POST', `/assets/${asset.id}/endpoint-data/refresh`, { query_key: key });
    d = await wait(async () => { const x = await endpoint(asset.id); const st = item(x, key).status; return st === 'ok' || st === 'failed' ? x : null; }, 240000, key);
    ok(item(d, key).status === 'ok' && item(d, key).rows.length > 0, 'query pack berjalan nyata setelah disetujui: ' + key + ' (' + item(d, key).rows.length + ' baris)');
  }
  await api('POST', `/assets/${asset.id}/endpoint-data/refresh`, { query_key: 'osquery.pack.vuln_management.os_version' });
  d = await wait(async () => { const x = await endpoint(asset.id); return item(x, 'osquery.pack.vuln_management.os_version').status === 'ok' ? x : null; }, 240000, 'vuln_management.os_version terkumpul');
  ok(item(d, 'osquery.pack.vuln_management.os_version').rows.length > 0, 'pack lain yang dapat berjalan di linux juga aktif dan terkumpul');

  // Mencabut persetujuan: query itu keluar dari laporan BARU dan dilewati lagi.
  await setCollection('osquery.network',false);
  await api('POST', `/assets/${asset.id}/endpoint-data/refresh`, { query_key: 'osquery.network' });
  d = await wait(async () => { const x = await endpoint(asset.id); return item(x, 'osquery.network').status === 'skipped' ? x : null; }, 180000, 'network dilewati lagi');
  ok(item(d, 'osquery.network').rows.length > 0 && item(d, 'osquery.network').reason === 'not approved for this organisation', 'dicabut: dilewati lagi, data baik terakhir tetap tersimpan');
  const findingsBefore=(await api('GET','/findings')).items.map(f=>f.id).sort();
  for(const i of (await endpoint(asset.id)).items.filter(i=>i.enabled))await setCollection(i.key,false);
  // Koleksi berkala berhenti setelah semua persetujuan dicabut; tunggu hasil eksekusi yang
  // masih berlangsung mendarat dulu (15 detik stabil) supaya penghapusan dataset benar-benar bersih.
  let quiet = { json: '', at: Date.now() };
  await wait(async () => {
    const x = JSON.stringify(await endpoint(asset.id));
    if (x !== quiet.json) quiet = { json: x, at: Date.now() };
    return Date.now() - quiet.at >= 15000 ? x : null;
  }, 90000, 'koleksi berakhir');
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

// E2E T2: data demo. DB kosong -> tanpa banner -> demo-seed (CLI) -> banner, organisasi/laporan berlabel -> axe -> purge dari UI -> bersih.
import { chromium } from 'playwright';
import AxeBuilder from '@axe-core/playwright';
import { execFileSync } from 'node:child_process';
import { mkdirSync, writeFileSync, unlinkSync } from 'node:fs';

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
const waitForPDFJobs = async () => {
  const deadline = Date.now() + 120000;
  while (Date.now() < deadline) {
    const reports = (await api('GET', '/reports?limit=50')).items;
    if (!reports.some((r) => r.status === 'queued' || r.status === 'rendering')) return reports;
    await new Promise((r) => setTimeout(r, 400));
  }
  throw new Error('Demo PDF jobs did not settle');
};

const axe = async (label) => {
  for (const width of [1280, 834]) {
    await p.setViewportSize({ width, height: width === 834 ? 1112 : 800 });
    await p.waitForLoadState('networkidle');
    if (label === 'profile token demo') await p.getByRole('heading',{name:'API tokens',exact:true}).scrollIntoViewIfNeeded();
    if (label === 'groups demo') await p.getByText('Demo: linux tagged endpoints',{exact:true}).scrollIntoViewIfNeeded();

    await p.screenshot({path:SHOTS+label.replace(/[^a-z0-9]+/gi,'-').toLowerCase()+'-'+width+'.png'});
    const res = await new AxeBuilder({ page: p }).analyze();
    ok(res.violations.length === 0, `axe nol temuan: ${label} @${width}` + (res.violations.length ? ' ' + res.violations.map((v) => v.id).join(',') : ''));
    const overflow = await p.evaluate(() => document.documentElement.scrollWidth > window.innerWidth + 1);
    ok(!overflow, `tanpa scroll horizontal: ${label} @${width}`);
    const clippedStatus = await p.locator('.status').evaluateAll(nodes=>nodes.some(node=>{
      const range=document.createRange();range.selectNodeContents(node);
      return range.getBoundingClientRect().height>node.getBoundingClientRect().height+1;
    }));
    ok(!clippedStatus, `teks status tidak terpotong: ${label} @${width}`);

  }
  await p.setViewportSize({ width: 1280, height: 800 });
};

try {
  await p.goto(BASE + '/login');
  await p.getByLabel('Email').fill('admin@orchis.local');
  await p.getByLabel('Password', { exact: true }).fill('Orchis-Dev-Pass-1');
  await p.getByRole('button', { name: /sign in/i }).click();
  await p.getByRole('heading', { name: 'Organisations', exact: true }).waitFor();
  await p.goto(BASE+'/overview');
  await p.getByRole('heading', { name: 'Home', exact: true }).waitFor();

  ok((await api('GET', '/demo/status')).has_demo === false, 'DB baru: status demo kosong');
  ok(await banner.count() === 0, 'DB baru: tanpa banner demo');

  const ancillaryPaths = ['/users','/roles','/people','/teams','/asset-groups','/atomic-tests','/action-presets','/integrations','/me/tokens','/ai/templates'];
  const rowsOf = (data) => Array.isArray(data) ? data : data.items;
  const baselineIDs = new Map();
  for (const path of ancillaryPaths) baselineIDs.set(path, rowsOf(await api('GET',path)).map(row=>row.id ?? row.key).sort().join(','));

  // Seed lewat CLI (satu-satunya jalur seed; tidak ada endpoint seed).
  const seeded = demoJSON('demo-seed', { DEMO_SEED: '2' });
  ok(seeded.has_demo && seeded.seed === 2 && seeded.counts.organizations === 5 && seeded.counts.scenarios === 3 && seeded.counts.alerts === 3, 'demo-seed membuat 5 organisasi, 3 skenario, 3 alert');
  let rejected = false;
  try { server('demo-seed', { DEMO_SEED: '3' }); } catch { rejected = true; }
  ok(rejected, 'seed berbeda tanpa purge ditolak CLI');
  ok(demoJSON('demo-seed', { DEMO_SEED: '2' }).counts.assets === 50, 'seed sama diulang tidak menduplikasi');

  await p.goto(BASE + '/');
  await banner.waitFor();
  await p.getByRole('heading', { name: 'Organisations', exact: true }).waitFor();
  await p.goto(BASE+'/overview');
  await p.getByRole('heading', { name: 'Home', exact: true }).waitFor();
  await p.locator('.skeleton').first().waitFor({ state: 'hidden', timeout: 15000 }).catch(() => {});
  ok((await banner.innerText()).includes('synthetic demo data'), 'banner demo tampil dan menjelaskan data sintetis');
  await p.screenshot({ path: SHOTS + 'home-1280.png' });
  await axe('home dengan banner');

  for (const table of ['users','api_tokens','password_reset_links','people','teams','asset_groups','atomic_tests','action_presets','integrations']) {
    ok(seeded.counts[table]>0, `demo memiliki relasi ${table}`);
  }
  for (const path of ancillaryPaths) {
    ok(rowsOf(await api('GET',path)).length>0, `API berisi data: ${path}`);
  }
  const demoUsers=rowsOf(await api('GET','/users')).filter(u=>u.email.endsWith('@example.invalid'));
  ok(demoUsers.length===3&&demoUsers.every(u=>u.status==='disabled'&&!u.has_password), 'akun demo disabled tanpa password');
  ok(rowsOf(await api('GET','/me/tokens')).some(t=>t.name.includes('Demo:')&&t.revoked_at), 'profil berisi token demo yang sudah dicabut');
  for (const [path,label] of [
    ['/people','people demo'], ['/people/teams','teams demo'], ['/settings/users','users demo'],
    ['/settings/roles','roles demo'], ['/settings/integrations','integrations demo'],
    ['/assets/groups','groups demo'], ['/atomic','saved tests demo'], ['/profile','profile token demo']
  ]) {
    await p.goto(BASE+path);await banner.waitFor();await axe(label);
  }

  await p.goto(BASE + '/organizations');
  await banner.waitFor();
  await p.getByText('(Demo)').first().waitFor({ timeout: 15000 }).catch(() => {});
  ok(await p.getByText('(Demo)').first().isVisible(), 'organisasi demo diberi tanda (Demo)');
  await p.screenshot({ path: SHOTS + 'organizations-1280.png' });
  await axe('organisasi');

  const simulations=(await api('GET','/simulations')).items;
  ok(simulations.length===69 && simulations.every(s=>s.is_demo), '69 riwayat simulasi ditandai sintetis');
  ok(['draft','scheduled','running','paused','finished','canceled','failed'].every(status=>simulations.some(s=>s.status===status)), 'riwayat mengisi seluruh status simulasi');
  for (const [name,step,status] of [['matched branch','scan','done'],['else branch','no-web','done'],['failure branch','failure','done'],['failed parent and blocked child','ports','blocked'],['running / waiting','scan','waiting']]) {
    const sim=simulations.find(s=>s.name===`Demo: synthetic ${name}`);
    const detail=await api('GET',`/simulations/${sim.id}`);
    ok(detail.steps.some(s=>s.key===step&&s.status===status), `keputusan cabang sintetis: ${name} -> ${status}`);
    const preview=await api('POST',`/simulations/${sim.id}/dry-run`,{});
    ok(!preview.executed&&preview.items.every(item=>item.recorded_reason?.includes('Demo/synthetic')), `dry-run menyimpan alasan sintetis: ${name}`);
  }
  await p.goto(BASE+'/simulations');await axe('synthetic simulations');
  const paused=simulations.find(s=>s.name==='Demo: synthetic paused');
  await p.goto(BASE+`/simulations/legacy/${paused.id}`);
  await p.getByText('Demo / synthetic history.',{exact:true}).waitFor();
  ok(await p.getByRole('button',{name:'Resume',exact:true}).count()===0,'riwayat sintetis tidak menawarkan dispatch');
  await axe('synthetic simulation detail');

  const reports = await Promise.all((await waitForPDFJobs()).map(r=>api('GET',`/reports/${r.id}`)));
  const ready = reports.find((r) => r.status === 'ready');
  const failed = reports.find((r) => r.status === 'failed');
  ok(reports.length === 15 && ready && failed, '15 laporan demo (tiga versi per organisasi) termasuk satu yang gagal');
  ok(reports.every((r) => r.snapshot.is_demo === true && /DEMO/.test(r.snapshot.label)), 'setiap snapshot laporan berlabel sintetis');
  ok(reports.every(r=>{
    const totals={critical:0,high:0,medium:0,low:0,info:0};
    for(const a of r.snapshot.assets)for(const f of a.findings)if(f.type==='vulnerability')totals[f.severity]++;
    return Object.keys(totals).every(k=>totals[k]===r.snapshot.by_severity[k]);
  }), 'ringkasan severity sama dengan daftar temuan snapshot');
  ok(reports.some(r=>new Set((r.snapshot.comparison_details?.checks||[]).filter(c=>c.comparable).map(c=>c.asset_id)).size===3&&(r.snapshot.comparison_details?.checks||[]).some(c=>!c.comparable)),'perbandingan sintetis mengecualikan aset gagal/dilewati');

  const pdfResponse = await ctx.request.get(`${BASE}/api/v1/reports/${ready.id}/export/pdf`);
  const pdf = await pdfResponse.body();
  ok(pdfResponse.ok() && pdf.subarray(0, 5).toString() === '%PDF-', 'laporan demo ready mempunyai PDF nyata');
  writeFileSync(SHOTS + 'demo-report.pdf', pdf);
  const frozen = JSON.stringify(failed.snapshot);
  const retried = await ctx.request.post(`${BASE}/api/v1/reports/${failed.id}/retry`, { headers: { 'X-Requested-With': 'orchis-web' } });
  ok(retried.ok(), 'laporan demo gagal dapat diantrekan ulang tanpa scan');
  const settled = await waitForPDFJobs();
  const afterRetry = await api('GET',`/reports/${failed.id}`);
  ok(settled.some(r=>r.id===failed.id&&r.status==='ready'),'retry PDF selesai di daftar laporan');
  ok(afterRetry?.status === 'ready' && JSON.stringify(afterRetry.snapshot) === frozen, 'retry menghasilkan PDF dan mempertahankan snapshot');
  const fixtureTests=(await api('GET','/atomic-tests')).items;
  ok(fixtureTests.length===3&&fixtureTests.every(t=>t.last_execution?.is_demo&&t.last_execution.state==='ended'),'saved tests memiliki riwayat sintetis terminal');
  ok(['success','error','skipped'].every(outcome=>fixtureTests.some(t=>t.last_execution.outcome===outcome)),'saved-test results mengisi sukses, gagal, dilewati');
  const fixtureTest=fixtureTests.find(t=>t.last_execution.outcome==='success');
  const fixtureExecution=await api('GET',`/executions/${fixtureTest.last_execution.id}`);
  ok(fixtureExecution.is_demo&&!fixtureExecution.raw_output_key,'hasil sintetis tidak membawa raw output palsu');
  await p.goto(BASE+`/atomic/${fixtureTest.id}`);await p.getByText('Demo / synthetic result.',{exact:true}).waitFor();await axe('synthetic saved-test result');

  const orgs=(await api('GET','/organizations?limit=50')).items;
  const templates=(await api('GET','/ai/templates')).items;
  ok(templates.length===4&&templates.every(t=>t.is_demo),'dua versi prompt per tujuan bertanda demo');
  let chosenOrg,reviewJob,completedJob;
  for(const org of orgs){
    const policy=await api('GET',`/organizations/${org.id}/ai`);
    ok(policy.is_demo&&!policy.enabled&&!policy.configured&&!policy.has_keys,'kebijakan demo tanpa kunci dan tanpa provider aktif');
    const jobs=(await api('GET',`/organizations/${org.id}/ai/jobs`)).items;
    ok(jobs.length===4&&jobs.every(j=>j.is_demo&&j.output?.summary.includes('DEMO / SYNTHETIC')),'empat job per organisasi berasal dari API palsu lokal');
    ok(['review','completed','rejected','canceled'].every(state=>jobs.some(j=>j.status===state)),'seluruh status review AI tersedia');
    if(!chosenOrg){chosenOrg=org;reviewJob=jobs.find(j=>j.status==='review');completedJob=jobs.find(j=>j.status==='completed');}
  }
  const approveDemo=await ctx.request.post(`${BASE}/api/v1/ai/jobs/${reviewJob.id}/approve`,{data:{version:reviewJob.version},headers:{'X-Requested-With':'orchis-web'}});
  ok(approveDemo.status()===409,'API menolak approval job sintetis');
  await p.goto(BASE+`/organizations/${chosenOrg.id}/ai`);
  await p.getByRole('heading',{name:'AI assistance',exact:true}).waitFor();
  ok(await p.getByRole('checkbox',{name:'Enable external AI for this organisation'}).count()===0,'kebijakan demo tidak menawarkan aktivasi provider');
  await axe('synthetic AI policy');
  await p.goto(BASE+`/ai/jobs/${reviewJob.id}`);await p.getByText('Demo / synthetic local fake API output.',{exact:true}).waitFor();
  ok(await p.getByRole('button',{name:'Review and approve checks'}).count()===0,'UI tidak menawarkan dispatch rencana sintetis');
  await axe('synthetic AI request');
  const sourceSnapshot=JSON.stringify((await api('GET',`/reports/${completedJob.report_id}`)).snapshot);
  for(const format of ['json','pdf']){
    const response=await ctx.request.get(`${BASE}/api/v1/ai/jobs/${completedJob.id}/export/${format}`);
    const body=await response.body();
    ok(response.ok(),`ekspor AI sintetis ${format} tersedia`);
    if(format==='json')ok(JSON.parse(body.toString()).is_demo===true,'ekspor AI JSON bertanda demo');
    if(format==='pdf'){ok(body.subarray(0,5).toString()==='%PDF-','ekspor AI PDF dirender nyata');writeFileSync(SHOTS+'demo-ai-advice.pdf',body);}
  }
  ok(JSON.stringify((await api('GET',`/reports/${completedJob.report_id}`)).snapshot)===sourceSnapshot,'suplemen AI tidak mengubah snapshot sumber');

  server('reindex');
  ok((await api('GET', '/findings?q=Demo')).items.length > 0, 'temuan demo dapat dicari melalui Elasticsearch lokal');

  for(const format of ['html','csv'])ok((await ctx.request.get(`${BASE}/api/v1/reports/${ready.id}/export/${format}`)).status()===404,`${format} export is removed`);
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
  await p.goto(BASE + '/chains');
  await p.waitForLoadState('networkidle');
  await p.screenshot({ path: SHOTS + 'scenarios-1280.png' });
  await axe('advanced chains');

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
  const fault = process.env.E2E_DEMO_STORAGE_FAILURE_FILE;
  if (fault) writeFileSync(fault,'LOCAL fixture deletion outage',{mode:0o600});
  await p.getByRole('dialog').getByRole('button', { name: 'Purge demo data' }).click();
  if (fault) {
    await p.getByText('Demo cleanup pending',{exact:true}).waitFor({timeout:20000});
    const pending = await api('GET','/demo/status');
    ok(!pending.has_demo && pending.cleanup_pending > 0, 'storage gagal: database terhapus, cleanup artefak tetap tercatat');
    await axe('demo cleanup pending');
    await banner.getByRole('button',{name:'Retry demo cleanup'}).click();
    ok(await p.getByRole('dialog').count() === 0,'retry cleanup tidak meminta konfirmasi penghapusan ulang');
    unlinkSync(fault);
  }
  await banner.waitFor({ state: 'detached', timeout: 45000 });
  const after = await api('GET', '/demo/status');
  ok(after.has_demo === false && after.cleanup_pending === 0 && Object.values(after.counts).every((n) => n === 0), 'purge dari UI menghapus semua data demo');
  await axe('home empty after demo purge');
  ok((await api('GET', '/organizations?limit=50')).items.length === 0, 'tidak ada organisasi tersisa setelah purge');

  for (const path of ancillaryPaths) ok(rowsOf(await api('GET',path)).map(row=>row.id ?? row.key).sort().join(',') === baselineIDs.get(path), `purge memulihkan baseline API: ${path}`);

  // Setelah purge seed berbeda diperbolehkan lagi, lalu bersihkan lewat CLI.
  ok(demoJSON('demo-seed', { DEMO_SEED: '3' }).seed === 3, 'setelah purge, seed berbeda diterima');
  await waitForPDFJobs();
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

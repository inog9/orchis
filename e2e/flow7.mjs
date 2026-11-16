// E2E Fase 8: tampilan responsif (1280 dan 834 px) dan aksesibilitas otomatis (axe-core, WCAG 2.2 AA) di SEMUA halaman dan dialog utama.
import { chromium } from 'playwright';
import AxeBuilder from '@axe-core/playwright';
import { execSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { mkdirSync } from 'node:fs';
const SHOTS = fileURLToPath(new URL('./shots/', import.meta.url));
mkdirSync(SHOTS, { recursive: true });
const BASE = process.env.E2E_BASE ?? 'http://127.0.0.1:8090';
const VIEWPORTS = [{ name: '1280', width: 1280, height: 800 }, { name: '834', width: 834, height: 1112 }];
const TAGS = ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa', 'best-practice'];
let pass = 0, fail = 0;
const ok = (c, m) => { (c ? pass++ : fail++); console.log((c ? 'LULUS: ' : 'GAGAL: ') + m); };
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const b = await chromium.launch();
const ctx = await b.newContext({ viewport: VIEWPORTS[0] });
const p = await ctx.newPage();
p.setDefaultTimeout(8000);
const errors = [];
const expected = /Failed to load resource: the server responded with a status of (400|401|403|404|409|422)/;
p.on('pageerror', (e) => errors.push('pageerror: ' + e.message));
p.on('console', (m) => m.type() === 'error' && !expected.test(m.text()) && errors.push('console: ' + m.text()));
const api = async (method, path, data) => { const r = await ctx.request.fetch(BASE + path, { method, data, headers: { 'X-Requested-With': 'orchis-web' } }); return r; };
const J = async (method, path, data) => { const r = await api(method, path, data); if (r.status() >= 300 && r.status() !== 204) throw new Error(`${method} ${path} -> ${r.status()} ${await r.text()}`); return r.status() === 204 ? null : r.json(); };
const inj = (token, path, body) => fetch(BASE + '/api/v1/injector/' + path, { method: 'POST', headers: { Authorization: 'Bearer ' + token, 'Content-Type': 'application/json' }, body: JSON.stringify(body ?? {}) });

// ---------- data contoh yang realistis ----------
await p.goto(BASE + '/login');
await p.getByLabel('Email').fill('admin@orchis.local');
await p.getByLabel('Password', { exact: true }).fill('Orchis-Dev-Pass-1');
await p.getByRole('button', { name: /sign in/i }).click();
await p.waitForURL(BASE+'/overview');await p.goto(BASE+'/organizations');

const tag = await J('POST', '/api/v1/tags', { name: 'crown-jewel', color: '#E8A557' });
const web = await J('POST', '/api/v1/assets', { category: 'host', name: 'srv-web-01', platform: 'linux', arch: 'x86_64', criticality: 'critical', is_internet_facing: true, testing_authorized: true, tag_ids: [tag.id], addresses: [{ kind: 'ip', value: '127.0.0.1' }, { kind: 'url', value: 'http://127.0.0.1:18081' }] });
await J('POST', '/api/v1/assets', { category: 'web_application', name: 'portal.corp.example', criticality: 'high', addresses: [{ kind: 'url', value: 'https://portal.corp.example' }] });
await J('POST', '/api/v1/assets', { category: 'host', name: 'WINTERFELL-with-a-very-long-hostname-to-test-wrapping', platform: 'windows', arch: 'x86_64', criticality: 'medium', addresses: [{ kind: 'hostname', value: 'winterfell.corp.example' }] });
const group = await J('POST', '/api/v1/asset-groups', { name: 'Linux servers', rule: { mode: 'and', filters: [{ field: 'platform', operator: 'in', values: ['linux'] }] } });
const person = await J('POST', '/api/v1/people', { first_name: 'Ada', last_name: 'Lovelace', email: 'ada@corp.example', organization: 'Corp' });
const team = await J('POST', '/api/v1/teams', { name: 'Blue team', kind: 'internal' });
await J('PUT', `/api/v1/teams/${team.id}/members`, { version: team.version, person_ids: [person.id] });
await J('POST', '/api/v1/users', { email: 'eve@orchis.test', display_name: 'Eve Analyst', role: 'analyst' });

const integs = (await J('GET', '/api/v1/integrations')).items;
const nmapTok = (await J('POST', `/api/v1/integrations/${integs.find((i) => i.key === 'nmap').id}/token`)).token;
const nucTok = (await J('POST', `/api/v1/integrations/${integs.find((i) => i.key === 'nuclei').id}/token`)).token;
await inj(nmapTok, 'heartbeat', { version: '1.0.0' });
await inj(nucTok, 'heartbeat', { version: '1.0.0' });

const at = await J('POST', '/api/v1/atomic-tests', { name: 'Open ports of srv-web-01', contract_key: 'nmap.tcp_connect', params: { profile: 'top100', service_detection: true }, target_asset_id: web.id });
const run = await J('POST', `/api/v1/atomic-tests/${at.id}/run`);
await inj(nmapTok, `executions/${run.id}/claim`);
await inj(nmapTok, `executions/${run.id}/result`, { exit_code: 0, logs: [{ level: 'info', message: 'nmap finished' }], findings: [
  { type: 'open_port', value: 'tcp/22', title: 'Open port 22/tcp (ssh)', severity: 'info', details: { port: 22 } },
  { type: 'open_port', value: 'tcp/80', title: 'Open port 80/tcp (http)', severity: 'info', details: { port: 80 } }] });

const sc = await J('POST', '/api/v1/scenarios', { name: 'Web server kill chain', severity: 'high', description: 'Scan, then check web exposure.' });
await J('PUT', `/api/v1/scenarios/${sc.id}/targets`, { version: sc.version, asset_ids: [web.id], group_ids: [] });
const sc2 = await J('GET', `/api/v1/scenarios/${sc.id}`);
await J('PUT', `/api/v1/scenarios/${sc.id}/graph`, { version: sc2.version, edges: [{ parent: 'scan', child: 'web' }, { parent: 'web', child: 'report' }], steps: [
  { key: 'scan', title: 'Port scan', contract_key: 'nmap.tcp_connect', trigger_mode: 'after_parents', is_enabled: true, delay_seconds: 0, params: { profile: 'top100' } },
  { key: 'web', title: 'Web exposure', contract_key: 'nuclei.scan', trigger_mode: 'on_finding', trigger: { type: 'open_port', values: ['tcp/80'] }, is_enabled: true, delay_seconds: 0, params: {} },
  { key: 'report', title: 'Follow-up scan', contract_key: 'nmap.tcp_connect', trigger_mode: 'after_parents', is_enabled: true, delay_seconds: 0, params: {} }] });
const dbName = new URL(process.env.ORCHIS_MIGRATE_URL).pathname.slice(1);
const sql = (q) => execSync(`docker compose -f deploy/compose.yml exec -T pg psql -U orchis -d ${dbName} -tAc "${q}"`, { encoding: 'utf8' }).trim();
const stepExec = (simId, key) => sql(`SELECT e.id FROM executions e JOIN simulation_steps s ON s.id=e.simulation_step_id WHERE e.simulation_id='${simId}' AND s.key='${key}' LIMIT 1`);
const finish = async (token, id, result) => { await inj(token, `executions/${id}/claim`); const r = await inj(token, `executions/${id}/result`, result); if (r.status !== 204) throw new Error('result ' + r.status); };

const sim = await J('POST', `/api/v1/scenarios/${sc.id}/launch`, {});
await finish(nmapTok, stepExec(sim.id, 'scan'), { exit_code: 0, findings: [{ type: 'open_port', value: 'tcp/80', title: 'Open port 80/tcp (http)', severity: 'info' }, { type: 'open_port', value: 'tcp/22', title: 'Open port 22/tcp (ssh)', severity: 'info' }] });
await finish(nucTok, stepExec(sim.id, 'web'), { exit_code: 0, findings: [{ type: 'vulnerability', value: 'exposed-env@http://127.0.0.1:18081/.env', title: 'Exposed .env file', severity: 'critical', details: { template_id: 'exposed-env' } }, { type: 'vulnerability', value: 'missing-hsts@x', title: 'Missing HSTS header', severity: 'medium' }] });
await finish(nmapTok, stepExec(sim.id, 'report'), { exit_code: 0, findings: [] });
const sim2 = await J('POST', `/api/v1/scenarios/${sc.id}/launch`, { name: 'Second run (still running)' });
await J('POST', '/api/v1/mitre/import').catch(() => {});

// ---------- pemeriksaan ----------
const routes = [
  ['Home', '/overview'], ['Assets', '/assets'], ['Asset detail', `/assets/${web.id}`], ['Asset groups', '/assets/groups'], ['Group detail', `/assets/groups/${group.id}`], ['Agents', '/assets/agents'],
  ['Findings', '/findings'], ['Scenario chains', '/chains'], ['Chain workspace', `/chains/${sc.id}`], ['Advanced chain setup', `/chains/${sc.id}/setup`],
  ['Simulations', '/simulations'], ['Simulation overview', `/simulations/legacy/${sim.id}`], ['Simulation findings', `/simulations/legacy/${sim.id}?tab=findings`], ['Simulation attack path', `/simulations/legacy/${sim.id}?tab=path`],
  ['Simulation running', `/simulations/legacy/${sim2.id}`], ['Single tests', '/atomic'], ['Atomic detail', `/atomic/${at.id}`], ['MITRE', '/mitre'], ['People', '/people'], ['Teams', '/people/teams'],
  ['Notifications', '/notifications'], ['Profile', '/profile'], ['Settings platform', '/settings'], ['Settings users', '/settings/users'], ['Settings roles', '/settings/roles'],
  ['Settings integrations', '/settings/integrations'], ['Settings tags', '/settings/tags'], ['Not found', '/no-such-page'],
];

let dialogCount=0;
const problems = [];
const note = (vp, label, what) => { problems.push(`[${vp}] ${label}: ${what}`); };

async function layout(vp, label) {
  const r = await p.evaluate(() => {
    const de = document.documentElement;
    const dock = document.querySelector('.topbar__actions');
    const dockTop = dock && getComputedStyle(dock).position==='fixed' ? dock.getBoundingClientRect().top : Infinity;
    // elemen terjauh di bawah dalam .content (tanpa elemen fixed) setelah scroll ke dasar
    window.scrollTo(0, document.body.scrollHeight);
    let maxBottom = 0;
    const content = document.querySelector('.content');
    if (content) for (const el of content.querySelectorAll('*')) {
      const cs = getComputedStyle(el);
      if (cs.position === 'fixed' || cs.visibility === 'hidden' || cs.display === 'none') continue;
      const rc = el.getBoundingClientRect();
      if (rc.width === 0 || rc.height === 0) continue;
      maxBottom = Math.max(maxBottom, rc.bottom);
    }
    const wide = [...document.querySelectorAll('.content *')].filter((el) => { const rc = el.getBoundingClientRect(); return rc.right > window.innerWidth + 1 && !el.closest('.list, .canvas, .pathwrap, .matrix, table, pre, .cmd, .stepcard'); }).slice(0, 3).map((el) => el.tagName.toLowerCase() + '.' + (el.className || '').toString().split(' ')[0]);
    const h1 = document.querySelector('.page__title');
    const titleLines = h1 ? h1.getBoundingClientRect().height / (parseFloat(getComputedStyle(h1).lineHeight) || parseFloat(getComputedStyle(h1).fontSize) * 1.2) : 0;
    const dockH = dock ? dock.getBoundingClientRect().height : 0;
    return { titleLines, dockH, overflowX: de.scrollWidth - de.clientWidth, dockTop, maxBottom, contentBottomGap: dockTop - maxBottom, scrollable: document.body.scrollHeight > window.innerHeight, wide };
  });
  if (r.overflowX > 1) note(vp, label, `halaman bergulir mendatar ${r.overflowX}px`);
  if (r.titleLines > 2.3) note(vp, label, `judul halaman terbungkus ${r.titleLines.toFixed(1)} baris (header terlalu sempit)`);
  if (r.dockH > 64) note(vp, label, `dock aksi membengkak jadi ${Math.round(r.dockH)}px (teks terbungkus)`);
  if (r.wide.length) note(vp, label, `elemen melewati lebar layar di luar area gulir: ${r.wide.join(', ')}`);
  if (r.scrollable && r.contentBottomGap < 4 && Number.isFinite(r.dockTop)) note(vp, label, `konten paling bawah tertutup dock aksi (celah ${Math.round(r.contentBottomGap)}px)`);
  await p.evaluate(() => window.scrollTo(0, 0));
}

async function axe(vp, label) {
  const res = await new AxeBuilder({ page: p }).withTags(TAGS).analyze();
  for (const v of res.violations) {
    const n = v.nodes.length;
    note(vp, label, `axe ${v.id} (${v.impact}, ${n}×): ${v.help} — contoh: ${v.nodes[0].target.join(' ')}`);
  }
}

async function visit(vp, label, path) {
  await p.goto(BASE + path);
  await p.waitForLoadState('networkidle').catch(() => {});
  await p.locator('.skeleton').first().waitFor({ state: 'detached', timeout: 8000 }).catch(() => {});
  await sleep(250);
  if (path === '/notifications') {
    const rows=await p.locator('.alert-row').evaluateAll(rows=>rows.map(row=>({available:row.querySelector('.alert-item__btn').getBoundingClientRect().width-40,button:row.querySelector('.btn').getBoundingClientRect().width})));
    if(rows.some(row=>row.button>150||row.available<200))note(vp.name,label,'tombol Dismiss menyempitkan teks notifikasi');
  }
  await layout(vp.name, label);
  await axe(vp.name, label);
  await p.screenshot({path:SHOTS+`ux-filled-${vp.name}-${label.toLowerCase().replace(/[^a-z0-9]+/g,'-')}.png`,fullPage:true});
}

for (const vp of VIEWPORTS) {
  await p.setViewportSize({ width: vp.width, height: vp.height });
  for (const [label, path] of routes) await visit(vp, label, path);
  await p.goto(BASE + '/overview');
  await p.locator('.skeleton').first().waitFor({ state: 'detached', timeout: 8000 }).catch(() => {});
  await p.screenshot({ path: SHOTS + `responsive-home-${vp.name}.png` });
  await p.goto(BASE + `/chains/${sc.id}`);
  await p.locator('.stepcard').first().waitFor();
  await p.screenshot({ path: SHOTS + `responsive-chain-${vp.name}.png` });
  await p.goto(BASE + `/simulations/legacy/${sim.id}?tab=path`);
  await p.getByRole('group', { name: /Attack path/ }).waitFor();
  await p.screenshot({ path: SHOTS + `responsive-path-${vp.name}.png` });

  // dialog dan menu: harus muat di layar, bisa digulir, dan lolos axe
  const dialogs = [
    ['Add asset dialog', '/assets', async () => { await p.getByRole('button', { name: /add asset/i }).first().click(); await p.getByRole('dialog').waitFor(); }],
    ['New single test dialog', '/atomic', async () => { await p.getByRole('button', { name: /new single test/i }).click(); await p.getByRole('dialog').waitFor(); }],
    ['Launch dialog', `/chains/${sc.id}/setup`, async () => { await p.getByRole('button', { name: /launch simulation/i }).click(); await p.getByRole('dialog').waitFor(); }],
    ['Edit targets dialog', `/chains/${sc.id}/setup`, async () => { await p.getByRole('button', { name: 'Edit targets' }).click(); await p.getByRole('dialog').waitFor(); }],
    ['New user dialog', '/settings/users', async () => { await p.getByRole('button', { name: /new user/i }).click(); await p.getByRole('dialog').waitFor(); }],
    ['Add person dialog', '/people', async () => { await p.getByRole('button', { name: /add person/i }).click(); await p.getByRole('dialog').waitFor(); }],
    ['New tag dialog', '/settings/tags', async () => { await p.getByRole('button', { name: /new tag/i }).click(); await p.getByRole('dialog').waitFor(); }],
    ['Group dialog', '/assets/groups', async () => { await p.getByRole('button', { name: /new group/i }).click(); await p.getByRole('dialog').waitFor(); }],
    ['Edit asset dialog', `/assets/${web.id}`, async()=>{await p.getByRole('button',{name:'Edit',exact:true}).click();await p.getByRole('dialog').waitFor();}],
    ['Import CSV dialog', '/assets', async()=>{await p.getByRole('button',{name:'Import CSV',exact:true}).click();await p.getByRole('dialog').waitFor();}],
    ['Edit group dialog', `/assets/groups/${group.id}`, async()=>{await p.getByRole('button',{name:'Edit',exact:true}).click();await p.getByRole('dialog').waitFor();}],
    ['New team dialog', '/people/teams', async()=>{await p.getByRole('button',{name:'New team',exact:true}).click();await p.getByRole('dialog').waitFor();}],
    ['Team members dialog', '/people/teams', async()=>{await p.getByRole('button',{name:'Members',exact:true}).first().click();await p.getByRole('dialog').waitFor();}],
    ['Edit person dialog', '/people', async()=>{await p.getByRole('button',{name:'Edit Ada Lovelace',exact:true}).click();await p.getByRole('dialog').waitFor();}],
    ['Edit account dialog', '/settings/users', async()=>{await p.getByRole('button',{name:'Edit Eve Analyst',exact:true}).click();await p.getByRole('dialog').waitFor();}],
    ['Password setup link dialog', '/settings/users', async()=>{await p.getByRole('button',{name:'Reset link for Eve Analyst',exact:true}).click();await p.getByRole('heading',{name:'Link for Eve Analyst',exact:true}).waitFor();}],
    ['Personal token form', '/profile', async()=>{await p.getByRole('button',{name:'New token',exact:true}).click();await p.getByRole('dialog').waitFor();}],
    ['Personal token secret dialog', '/profile', async()=>{await p.getByRole('button',{name:'New token',exact:true}).click();const d=p.getByRole('dialog');await d.getByLabel('Name',{exact:true}).fill('Responsive test '+vp.name);await d.getByRole('button',{name:'Create token',exact:true}).click();await p.getByRole('heading',{name:'Copy your token now',exact:true}).waitFor();}],
    ['Remove asset confirmation', `/assets/${web.id}`, async()=>{await p.getByRole('button',{name:'Remove asset',exact:true}).click();await p.getByRole('dialog').waitFor();}],
    ['Worker disable confirmation', '/settings/integrations', async()=>{await p.getByRole('rowgroup').filter({has:p.getByText('Nmap',{exact:true})}).getByRole('button',{name:'Disable',exact:true}).click();await p.getByRole('dialog').waitFor();}],
    ['Worker token secret dialog', '/settings/integrations', async()=>{await p.getByRole('rowgroup').filter({has:p.getByText('Nmap',{exact:true})}).getByRole('button',{name:'Rotate token',exact:true}).click();await p.getByRole('dialog').getByRole('button',{name:'Rotate token',exact:true}).click();await p.getByRole('heading',{name:'New token for Nmap',exact:true}).waitFor();}],
    ['Command palette', '/overview', async () => { await p.keyboard.press('Control+k'); await p.getByRole('combobox', {name: 'Search or jump to'}).waitFor(); }],
  ];
  dialogCount=dialogs.length;
  for (const [label, path, open] of dialogs) {
    await p.goto(BASE + path);
    await p.waitForLoadState('networkidle').catch(() => {});
    await p.locator('.skeleton').first().waitFor({ state: 'detached', timeout: 8000 }).catch(() => {});
    try { await open(); } catch (e) { note(vp.name, label, 'tidak bisa dibuka: ' + e.message.split('\n')[0]); continue; }
    await sleep(200);
    const fits = await p.evaluate(() => { const d = document.querySelector('[role=dialog]'); if (!d) return null; const r = d.getBoundingClientRect(); return { top: r.top, left: r.left, right: r.right, bottom: r.bottom, vw: innerWidth, vh: innerHeight, scroll: d.scrollHeight > d.clientHeight }; });
    if (fits && (fits.left < -1 || fits.right > fits.vw + 1)) note(vp.name, label, `dialog melewati lebar layar (${Math.round(fits.left)}..${Math.round(fits.right)} dari ${fits.vw})`);
    if (fits && fits.top < -1) note(vp.name, label, 'bagian atas dialog terpotong');
    if (fits && fits.bottom > fits.vh + 1 && !fits.scroll) note(vp.name, label, 'dialog lebih tinggi dari layar dan tidak bisa digulir');
    await axe(vp.name, label);
    await p.screenshot({path:SHOTS+`ux-dialog-${vp.name}-${label.toLowerCase().replace(/[^a-z0-9]+/g,'-')}.png`,fullPage:true,mask:[p.locator('.secret')]});
    await p.keyboard.press('Escape');
  }
  // menu dan popover
  await p.goto(BASE + '/overview');
  await p.getByRole('button', { name: /Notifications/ }).first().click();
  await sleep(300);
  await axe(vp.name, 'Notifications popover');
  await p.screenshot({path:SHOTS+`ux-menu-${vp.name}-notifications.png`,fullPage:true});
  await p.keyboard.press('Escape');
  await p.getByRole('button', { name: /^New/ }).click();
  await sleep(200);
  await axe(vp.name, 'New menu');
  await p.screenshot({path:SHOTS+`ux-menu-${vp.name}-new.png`,fullPage:true});
  await p.keyboard.press('Escape');
  await p.getByRole('button', { name: /Platform default/ }).click();
  await sleep(200);
  await axe(vp.name, 'Workspace menu');
  await p.screenshot({path:SHOTS+`ux-menu-${vp.name}-workspace.png`,fullPage:true});
  await p.keyboard.press('Escape');
}

// halaman publik (tanpa sesi)
const anon = await b.newContext();
const a = await anon.newPage();
for (const vp of VIEWPORTS) {
  await a.setViewportSize({ width: vp.width, height: vp.height });
  for (const [label, path] of [['Login', '/login'], ['Forgot password', '/forgot'], ['Reset (invalid link)', '/reset?token=orl_nope']]) {
    await a.goto(BASE + path);
    await a.waitForLoadState('networkidle').catch(() => {});
    await sleep(300);
    const over = await a.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    if (over > 1) note(vp.name, label, `halaman bergulir mendatar ${over}px`);
    const res = await new AxeBuilder({ page: a }).withTags(TAGS).analyze();
    for (const v of res.violations) note(vp.name, label, `axe ${v.id} (${v.impact}, ${v.nodes.length}×): ${v.help} — contoh: ${v.nodes[0].target.join(' ')}`);
    await a.screenshot({path:SHOTS+`ux-public-${vp.name}-${label.toLowerCase().replace(/[^a-z0-9]+/g,'-')}.png`,fullPage:true});
  }
}

// ---------- papan ketik: fokus terlihat, tautan lompat ----------
await p.setViewportSize({ width: 1280, height: 800 });
const focusRoutes = [['Home', '/overview'], ['Assets', '/assets'], ['Single tests', '/atomic'], ['Settings users', '/settings/users'], ['Chain workspace', `/chains/${sc.id}`], ['Simulation detail', `/simulations/legacy/${sim.id}`]];
for (const [label, path] of focusRoutes) {
  await p.goto(BASE + path);
  await p.waitForLoadState('networkidle').catch(() => {});
  await p.locator('.skeleton').first().waitFor({ state: 'detached', timeout: 8000 }).catch(() => {});
  await p.locator('body').click({ position: { x: 3, y: 3 } });
  const stops = [];
  for (let i = 0; i < 30; i++) {
    await p.keyboard.press('Tab');
    stops.push(await p.evaluate(() => {
      const el = document.activeElement;
      if (!el || el === document.body) return null;
      const cs = getComputedStyle(el);
      const outline = cs.outlineStyle !== 'none' && parseFloat(cs.outlineWidth) > 0;
      const shadow = cs.boxShadow && cs.boxShadow !== 'none';
      // kotak centang kustom: indikator fokus ada pada kotak pengganti di sebelahnya
      const sib = el.nextElementSibling ? getComputedStyle(el.nextElementSibling) : null;
      const sibOutline = !!sib && sib.outlineStyle !== 'none' && parseFloat(sib.outlineWidth) > 0;
      const wrapper = el.closest('.field__input, .search-input');
      const wrapOutline = !!wrapper && getComputedStyle(wrapper).outlineStyle !== 'none' && parseFloat(getComputedStyle(wrapper).outlineWidth) > 0;
      const r = el.getBoundingClientRect();
      return { name: (el.getAttribute('aria-label') || el.textContent || el.tagName).trim().slice(0, 30), visible: outline || shadow || sibOutline || wrapOutline, inView: r.bottom > 0 && r.top < innerHeight };
    }));
  }
  const seen = stops.filter(Boolean);
  const bad = seen.filter((s) => !s.visible);
  ok(seen.length >= 8 && bad.length === 0, `${label}: ${seen.length} elemen terjangkau Tab, semuanya punya indikator fokus` + (bad.length ? ` (tanpa indikator: ${bad.map((b) => b.name).join(', ')})` : ''));
}
await p.goto(BASE + '/assets');
// Wait for the authenticated page frame; the initial session check has no asset skeleton.
await p.getByRole('navigation', { name: 'Primary', exact: true }).waitFor();
await p.locator('.skeleton').first().waitFor({ state: 'detached', timeout: 8000 }).catch(() => {});
await p.keyboard.press('Tab'); // tanpa klik lebih dulu: Tab pertama pada halaman baru
const skip = p.getByRole('link', { name: 'Skip to content' });
ok(await skip.evaluate((el) => el === document.activeElement && el.getBoundingClientRect().top >= 0), 'Tab pertama: tautan "Skip to content" muncul dan terfokus');
await p.keyboard.press('Enter');
ok(await p.evaluate(() => document.activeElement?.id === 'main' && location.hash === '#main'), 'Enter pada tautan lompat memindahkan fokus ke konten utama');

// ---------- laporan ----------
const total = routes.length * VIEWPORTS.length + dialogCount * VIEWPORTS.length + 6 * VIEWPORTS.length;
console.log(`\nDiperiksa: ${routes.length} halaman + ${dialogCount} dialog + 3 menu + 3 halaman publik, di ${VIEWPORTS.map((v) => v.width).join(' dan ')} px.`);
for (const line of problems) console.log('  TEMUAN ' + line);
ok(problems.length === 0, `tidak ada temuan responsif/aksesibilitas (${problems.length} temuan)`);
ok(errors.length === 0, 'tidak ada error konsol/halaman tak terduga' + (errors.length ? ': ' + errors.join(' | ') : ''));
await b.close();
console.log(`\n${pass} lulus, ${fail} gagal (${total} pemeriksaan halaman/dialog)`);
process.exit(fail ? 1 : 0);

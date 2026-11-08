// Panen audit UI: screenshot semua halaman x tema x bahasa x lebar + pemeriksaan otomatis.
// Server lokal terisolasi saja (127.0.0.1), tidak pernah produksi.
import { chromium } from 'playwright';
import { mkdir, writeFile } from 'node:fs/promises';

const POP = process.env.AUDIT_POP || 'http://127.0.0.1:18190';
const EMPTY = process.env.AUDIT_EMPTY || 'http://127.0.0.1:18191';
for (const u of [POP, EMPTY]) { const p = new URL(u); if (p.hostname !== '127.0.0.1' || p.protocol !== 'http:') throw new Error('hanya 127.0.0.1 http'); }
const OUT = process.env.AUDIT_OUT || 'docs/ui-audit/shots/before';
const REPORT = process.env.AUDIT_REPORT || 'docs/ui-audit/harvest.json';
await mkdir(OUT, { recursive: true });

const ADMIN = { email: 'admin@orchis.local', password: 'Orchis-Dev-Pass-1' };
const browser = await chromium.launch();
const report = { shots: [], inventory: {}, notes: [] };

async function login(ctx, base) {
  const res = await ctx.request.post(base + '/api/v1/auth/login', { headers: { 'X-Requested-With': 'orchis-web' }, data: ADMIN });
  if (!res.ok()) throw new Error('login gagal: ' + res.status());
}
async function setLang(ctx, base, lang) {
  await ctx.request.patch(base + '/api/v1/auth/preferences', { headers: { 'X-Requested-With': 'orchis-web' }, data: { ui_language: lang } });
}

const errors = [];
function wire(page, tag) {
  page.on('console', m => { if (m.type() === 'error') errors.push({ tag, text: m.text().slice(0, 300) }); });
  page.on('pageerror', e => errors.push({ tag, text: String(e).slice(0, 300) }));
}

async function capture(page, ctx, base, name, path, opts = {}) {
  const { themes = ['light', 'dark'], langs = ['id', 'en'], widths = [1440], matrix = false } = opts;
  const combos = matrix
    ? themes.flatMap(t => langs.map(l => ({ t, l, w: 1440 }))).concat([{ t: 'light', l: 'id', w: 820 }, { t: 'light', l: 'id', w: 390 }])
    : (opts.combos ?? [{ t: 'light', l: 'id', w: 1440 }]);
  for (const { t, l, w } of combos) {
    await ctx.request.patch(base + '/api/v1/auth/preferences', { headers: { 'X-Requested-With': 'orchis-web' }, data: { ui_language: l } }).catch(() => {});
    await page.addInitScript(theme => { try { localStorage.setItem('orchis.theme', theme); } catch {} }, t);
    await page.goto(base + path, { waitUntil: 'networkidle', timeout: 20000 }).catch(e => errors.push({ tag: name, text: 'goto: ' + String(e).slice(0, 160) }));
    await page.setViewportSize({ width: w, height: 900 }).catch(() => {});
    await page.waitForTimeout(350);
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth + 1);
    const file = `${OUT}/${name}--${t}--${l}--${w}.jpg`;
    await page.screenshot({ path: file, type: 'jpeg', quality: 75, fullPage: false });
    report.shots.push({ name, path, theme: t, lang: l, width: w, overflow, file: file.replace(/^docs\//, '') });
    await page.evaluate(() => localStorage.removeItem('orchis.theme'));
  }
}

async function inventory(page, base, path, name) {
  await page.goto(base + path, { waitUntil: 'networkidle', timeout: 20000 }).catch(() => {});
  const data = await page.evaluate(() => {
    const named = el => (el.getAttribute('aria-label') || el.textContent || el.value || '').trim().replace(/\s+/g, ' ').slice(0, 60);
    const grab = sel => [...document.querySelectorAll(sel)].map(el => ({ n: named(el), tag: el.tagName.toLowerCase(), disabled: el.disabled || el.getAttribute('aria-disabled') === 'true', visible: !!(el.offsetWidth || el.offsetHeight) }));
    return {
      buttons: grab('button, [role="button"]'),
      links: grab('a[href]'),
      tabs: grab('[role="tab"]'),
      selects: grab('select'),
      inputs: [...document.querySelectorAll('input, textarea')].map(el => ({ n: named(el), tag: el.tagName.toLowerCase(), type: el.getAttribute('type'), placeholder: el.getAttribute('placeholder'), disabled: el.disabled, visible: !!(el.offsetWidth || el.offsetHeight) })),
      headings: [...document.querySelectorAll('h1,h2,h3')].map(h => h.textContent.trim().slice(0, 60)),
      radii: (() => { const bad = new Set(); for (const el of document.querySelectorAll('button,a,select,input,[role="tab"],[class]')) { const r = getComputedStyle(el).borderRadius; if (r && r !== '0px') bad.add(`${el.tagName.toLowerCase()}.${String(el.className).split(' ')[0]}=${r}`); } return [...bad].slice(0, 12); })(),
    };
  });
  report.inventory[name] = data;
}

// ===== Server terisi =====
{
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  await login(ctx, POP);
  const api = async (p) => (await ctx.request.get(POP + '/api/v1' + p, { headers: { 'X-Requested-With': 'orchis-web' } })).json();
  const orgs = (await api('/organizations')).items ?? (await api('/organizations'));
  const org = Array.isArray(orgs) ? orgs[0] : orgs.items?.[0];
  const orgId = org?.id;
  const assets = (await api('/assets?limit=1')).items ?? [];
  const assetId = assets[0]?.id;
  const scans = (await api('/scans')).items ?? [];
  const scanId = scans[0]?.id;
  const reports = (await api('/reports')).items ?? [];
  const reportId = reports[0]?.id;
  const groups = (await api('/asset-groups')).items ?? [];
  const groupId = groups[0]?.id;
  report.ids = { orgId, assetId, scanId, reportId, groupId };

  const page = await ctx.newPage();
  wire(page, 'populated');

  const routes = [
    ['home', '/'],
    ['organizations', '/organizations'],
    ['org-detail', `/organizations/${orgId}`],
    ['assets', '/assets'],
    ['asset-detail', `/assets/${assetId}`],
    ['agents', '/assets/agents'],
    ['groups', '/assets/groups'],
    ['group-detail', `/assets/groups/${groupId}`],
    ['findings', '/findings'],
    ['simulations', '/simulations'],
    ['new-scan', `/simulations/new?org=${orgId}`],
    ['scan-detail', `/simulations/${scanId}`],
    ['reports', '/reports'],
    ['report-detail', `/reports/${reportId}`],
    ['settings', '/settings'],
    ['settings-users', '/settings/users'],
    ['settings-roles', '/settings/roles'],
    ['settings-integrations', '/settings/integrations'],
    ['settings-tags', '/settings/tags'],
    ['settings-audit', '/settings/audit'],
    ['notifications', '/notifications'],
    ['profile', '/profile'],
    ['people', '/people'],
    ['notfound', '/halaman-tidak-ada'],
  ];
  for (const [name, path] of routes) {
    await capture(page, ctx, POP, name, path, { matrix: true });
    if (['organizations', 'org-detail', 'assets', 'asset-detail', 'findings', 'simulations', 'new-scan', 'scan-detail', 'reports', 'report-detail', 'settings'].includes(name)) {
      await inventory(page, POP, path, name);
    }
  }

  // Halaman login/forgot/reset (guest) - konteks tanpa sesi.
  const gctx = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  const gpage = await gctx.newPage(); wire(gpage, 'guest');
  for (const [name, path] of [['login', '/login'], ['forgot', '/forgot'], ['reset', '/reset?token=bogus-token']]) {
    await capture(gpage, gctx, EMPTY, name, path, { matrix: true });
  }
  await gctx.close();

  // State galat: API 500.
  await ctx.route('**/api/v1/**', r => r.fulfill({ status: 500, contentType: 'application/json', body: '{"error":"galat terkendali"}' }));
  for (const [name, path] of [['err-organizations', '/organizations'], ['err-findings', '/findings'], ['err-asset-detail', `/assets/${assetId}`], ['err-reports', '/reports']]) {
    await capture(page, ctx, POP, name, path, {});
  }
  await ctx.unroute('**/api/v1/**');

  // State memuat: tunda respons 2.5 detik.
  await ctx.route('**/api/v1/**', async r => { await new Promise(res => setTimeout(res, 2500)); r.continue(); });
  for (const [name, path] of [['load-organizations', '/organizations'], ['load-findings', '/findings'], ['load-assets', '/assets']]) {
    await page.goto(POP + path, { waitUntil: 'domcontentloaded' }).catch(() => {});
    await page.waitForTimeout(900);
    await page.screenshot({ path: `${OUT}/${name}--1440.jpg`, type: 'jpeg', quality: 75 });
    report.shots.push({ name, theme: 'light', lang: 'id', width: 1440, file: `${OUT}/${name}--1440.jpg`.replace(/^docs\//, '') });
  }
  await ctx.unroute('**/api/v1/**');
  await ctx.close();
}

// ===== Server kosong =====
{
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  await login(ctx, EMPTY);
  const page = await ctx.newPage(); wire(page, 'empty');
  for (const [name, path] of [
    ['empty-home', '/'], ['empty-organizations', '/organizations'], ['empty-assets', '/assets'],
    ['empty-agents', '/assets/agents'], ['empty-groups', '/assets/groups'], ['empty-findings', '/findings'],
    ['empty-simulations', '/simulations'], ['empty-new-scan', '/simulations/new'], ['empty-reports', '/reports'],
    ['empty-settings', '/settings'], ['empty-notifications', '/notifications'],
  ]) {
    await capture(page, ctx, EMPTY, name, path, { matrix: false });
  }
  await ctx.close();
}

report.consoleErrors = errors;
await writeFile(REPORT, JSON.stringify(report, null, 1));
const ovf = report.shots.filter(s => s.overflow).map(s => s.name + '/' + s.width);
console.log(JSON.stringify({ shots: report.shots.length, inventory: Object.keys(report.inventory).length, consoleErrors: errors.length, overflow: ovf }, null, 1));
await browser.close();

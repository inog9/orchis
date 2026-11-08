// Ambil ulang shot untuk scan-detail, org nyata, dan aset web (dibuat lewat API).
import { chromium } from 'playwright';
import { mkdir, writeFile } from 'node:fs/promises';
const POP = 'http://127.0.0.1:18190';
const OUT = '/root/work/orchis-ui-v3/docs/ui-audit/shots/before';
await mkdir(OUT, { recursive: true });
const browser = await chromium.launch();
const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } });
await ctx.request.post(POP + '/api/v1/auth/login', { headers: { 'X-Requested-With': 'orchis-web' }, data: { email: 'admin@orchis.local', password: 'Orchis-Dev-Pass-1' } });
const api = async p => (await ctx.request.get(POP + '/api/v1' + p, { headers: { 'X-Requested-With': 'orchis-web' } })).json();
const scan = (await api('/scans')).items[0];
const orgs = (await api('/organizations')).items ?? [];
const realOrg = orgs.find(o => o.name === 'PT Audit Contoh') ?? orgs[0];
const assets = (await api(`/assets?organization=${realOrg.id}`)).items ?? [];
const webAsset = assets.find(a => a.category === 'web_application') ?? assets[0];
const page = await ctx.newPage();
const errors = [];
page.on('console', m => { if (m.type() === 'error') errors.push(m.text().slice(0, 200)); });
page.on('pageerror', e => errors.push(String(e).slice(0, 200)));

const targets = [
  ['scan-detail', `/simulations/${scan.id}`],
  ['org-detail-real', `/organizations/${realOrg.id}`],
  ['asset-detail-web', `/assets/${webAsset.id}`],
];
const report = [];
for (const [name, path] of targets) {
  for (const [t, l, w] of [['light','id',1440],['dark','id',1440],['light','en',1440],['light','id',820],['light','id',390]]) {
    await ctx.request.patch(POP + '/api/v1/auth/preferences', { headers: { 'X-Requested-With': 'orchis-web' }, data: { ui_language: l } });
    await page.addInitScript(theme => { try { localStorage.setItem('orchis.theme', theme); } catch {} }, t);
    await page.goto(POP + path, { waitUntil: 'networkidle', timeout: 20000 }).catch(() => {});
    await page.setViewportSize({ width: w, height: 900 });
    await page.waitForTimeout(350);
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth + 1);
    const file = `${OUT}/${name}--${t}--${l}--${w}.jpg`;
    await page.screenshot({ path: file, type: 'jpeg', quality: 75 });
    report.push({ name, theme: t, lang: l, width: w, overflow, file: file.replace(/^docs\//, '') });
    await page.evaluate(() => localStorage.removeItem('orchis.theme'));
  }
}
await writeFile('/root/work/orchis-ui-v3/docs/ui-audit/harvest-retake.json', JSON.stringify({ report, ids: { scan: scan.id, org: realOrg.id, webAsset: webAsset.id }, errors }, null, 1));
console.log(JSON.stringify({ done: report.length, overflow: report.filter(s => s.overflow).map(s => s.name + ':' + s.width), scan: scan.status, errors: errors.slice(0, 5) }));
await browser.close();

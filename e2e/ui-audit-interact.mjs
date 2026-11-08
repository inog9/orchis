// Pass interaktif: klik wizard, tab, drawer, dialog, menu; tangkap state seleksi & alur.
import { chromium } from 'playwright';
import { mkdir, writeFile } from 'node:fs/promises';
const POP = 'http://127.0.0.1:18190';
const OUT = process.env.AUDIT_OUT || '/root/work/orchis-ui-v3/docs/ui-audit/shots/before';
await mkdir(OUT, { recursive: true });
const browser = await chromium.launch();
const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } });
await ctx.request.post(POP + '/api/v1/auth/login', { headers: { 'X-Requested-With': 'orchis-web' }, data: { email: 'admin@orchis.local', password: 'Orchis-Dev-Pass-1' } });
const api = async p => (await ctx.request.get(POP + '/api/v1' + p, { headers: { 'X-Requested-With': 'orchis-web' } })).json();
const orgs = (await api('/organizations')).items ?? [];
const realOrg = orgs.find(o => o.name === 'PT Audit Contoh') ?? orgs[0];
const demoOrg = orgs.find(o => o.id !== realOrg.id);
const page = await ctx.newPage();
const errors = [];
page.on('console', m => { if (m.type() === 'error') errors.push(m.text().slice(0, 200)); });
page.on('pageerror', e => errors.push(String(e).slice(0, 200)));
const shot = (n, full = false) => page.screenshot({ path: `${OUT}/int-${n}.jpg`, type: 'jpeg', quality: 75, fullPage: full });
const log = [];
const step = (id, msg) => { log.push({ id, msg }); console.log(id, '::', msg); };

// ===== Wizard baru =====
await page.goto(`${POP}/simulations/new?org=${realOrg.id}`, { waitUntil: 'networkidle' });
await shot('wizard-step1');
step('W1', 'langkah 1 dirender');
await page.getByRole('button', { name: /lanjut|next/i }).click().catch(e => step('W2', 'lanjut gagal: ' + String(e).slice(0, 80)));
await page.waitForTimeout(600); await shot('wizard-step2');
step('W3', 'langkah 2 (yang dijalankan)');
// probe state terpilih pada pilihan engine
const engineProbe = await page.evaluate(() => {
  const out = [];
  for (const el of document.querySelectorAll('[role="radio"],[role="checkbox"],label,[class*="option"],[class*="engine"]')) {
    const cs = getComputedStyle(el); const t = (el.textContent || '').trim().slice(0, 30);
    if (t && /hackbot|defensebot|penilaian|penuh|web/i.test(t)) out.push({ text: t, cls: String(el.className).split(' ').slice(0, 2).join('.'), bg: cs.backgroundColor, border: cs.borderColor, aria: el.getAttribute('aria-checked') });
  }
  return out.slice(0, 10);
});
step('W4', 'probe engine: ' + JSON.stringify(engineProbe));
try { await page.getByRole('button', { name: /lanjut|next/i }).click(); await page.waitForTimeout(600); await shot('wizard-step3'); step('W5', 'langkah 3 (sasaran)'); } catch (e) { step('W5', 'tidak bisa lanjut: ' + String(e).slice(0, 100)); }
const targetProbe = await page.evaluate(() => {
  const rows = [...document.querySelectorAll('table tbody tr, [role="row"], label')].slice(0, 6);
  return rows.map(r => { const cs = getComputedStyle(r); return { text: (r.textContent || '').trim().slice(0, 50), bg: cs.backgroundColor, checkbox: !!r.querySelector('input[type="checkbox"]'), disabled: r.querySelector('input[disabled]') !== null }; });
});
step('W6', 'probe sasaran: ' + JSON.stringify(targetProbe).slice(0, 600));
try { const cb = page.locator('input[type="checkbox"]').first(); await cb.check({ timeout: 3000 }); await page.waitForTimeout(300); await shot('wizard-step3-selected'); step('W7', 'checkbox pertama dicentang'); } catch (e) { step('W7', 'checkbox tidak bisa dicentang: ' + String(e).slice(0, 80)); }
try { await page.getByRole('button', { name: /lanjut|next|tinjau/i }).click(); await page.waitForTimeout(600); await shot('wizard-step4'); step('W8', 'langkah 4 (tinjauan)'); } catch (e) { step('W8', 'tidak bisa ke tinjauan: ' + String(e).slice(0, 100)); }

// ===== Tab detail organisasi =====
await page.goto(`${POP}/organizations/${realOrg.id}`, { waitUntil: 'networkidle' });
await shot('org-tab-overview');
for (const tab of await page.getByRole('tab').allTextContents()) {
  const key = 'org-tab-' + tab.trim().toLowerCase().replace(/\s+/g, '-').replace(/[^\w-]/g, '').slice(0, 22);
  await page.getByRole('tab', { name: tab.trim() }).click().catch(e => step('T', 'tab gagal ' + tab + ': ' + String(e).slice(0, 60)));
  await page.waitForTimeout(700); await shot(key);
  step('T', 'tab: ' + tab.trim());
}

// ===== Temuan: buka drawer baris pertama =====
await page.goto(`${POP}/findings`, { waitUntil: 'networkidle' });
await page.locator('table tbody tr, [class*="row"]').filter({ hasText: /kritis|tinggi|sedang|rendah|info/i }).first().click({ timeout: 5000 }).catch(async () => { await page.getByText(/kritis|tinggi/i).first().click().catch(() => {}); });
await page.waitForTimeout(800); await shot('findings-drawer');
step('F1', 'drawer temuan dibuka (atau gagal klik)');
await page.keyboard.press('Escape').catch(() => {});

// ===== Grup aset =====
await page.goto(`${POP}/assets/groups`, { waitUntil: 'networkidle' }); await shot('groups');

// ===== Pengaturan: pengguna & integrasi & audit =====
for (const [name, path] of [['settings-users', '/settings/users'], ['settings-integrations', '/settings/integrations'], ['settings-audit', '/settings/audit']]) {
  await page.goto(POP + path, { waitUntil: 'networkidle' }); await shot(name);
}

// ===== Dialog tambah aset =====
await page.goto(`${POP}/assets`, { waitUntil: 'networkidle' });
await page.getByRole('button', { name: /tambah|baru|asset/i }).first().click({ timeout: 4000 }).catch(() => {});
await page.waitForTimeout(500); await shot('dialog-add-asset');
step('A1', 'dialog tambah aset');
await page.keyboard.press('Escape');

// ===== Palet perintah =====
await page.keyboard.press('ControlOrMeta+k').catch(() => {});
await page.waitForTimeout(400); await shot('command-palette');
await page.keyboard.press('Escape');

// ===== Menu akun & notifikasi =====
await page.locator('[class*="account"], [class*="avatar"], [class*="profile"]').first().click({ timeout: 3000 }).catch(() => {});
await page.waitForTimeout(400); await shot('account-menu'); await page.keyboard.press('Escape');
await page.goto(`${POP}/notifications`, { waitUntil: 'networkidle' }); await shot('notifications-page');
await page.goto(`${POP}/profile`, { waitUntil: 'networkidle' }); await shot('profile-page');
await page.goto(`${POP}/people`, { waitUntil: 'networkidle' }); await shot('people-page');

// ===== Keyboard: fokus terlihat? =====
await page.goto(`${POP}/organizations`, { waitUntil: 'networkidle' });
const focusInfo = [];
for (let i = 0; i < 8; i++) { await page.keyboard.press('Tab'); const f = await page.evaluate(() => { const el = document.activeElement; const cs = getComputedStyle(el); return { tag: el.tagName, name: (el.getAttribute('aria-label') || el.textContent || '').trim().slice(0, 26), outline: cs.outlineStyle + ' ' + cs.outlineWidth, ring: cs.boxShadow !== 'none' }; }); focusInfo.push(f); }
step('K1', 'tab order: ' + JSON.stringify(focusInfo));

// ===== Login: galat kredensial =====
const gctx = await browser.newContext({ viewport: { width: 1440, height: 900 } });
const gp = await gctx.newPage();
await gp.goto(POP + '/login', { waitUntil: 'networkidle' });
await gp.getByLabel(/email/i).fill('admin@orchis.local');
await gp.getByRole("textbox", { name: "Password" }).fill('salah-total-999');
await gp.getByRole('button', { name: /sign in|masuk/i }).click();
await gp.waitForTimeout(900); await gp.screenshot({ path: `${OUT}/int-login-error.jpg`, type: 'jpeg', quality: 75 });
step('L1', 'login dengan kredensial salah');
await gp.goto(POP + '/forgot', { waitUntil: 'networkidle' }); await gp.screenshot({ path: `${OUT}/int-forgot.jpg`, type: 'jpeg', quality: 75 });
await gctx.close();

await writeFile('/root/work/orchis-ui-v3/docs/ui-audit/interact-log.json', JSON.stringify({ log, errors: [...new Set(errors)] }, null, 1));
console.log('ERRORS:', [...new Set(errors)].slice(0, 6));
await browser.close();

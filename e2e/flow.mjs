import { chromium } from 'playwright';
import { fileURLToPath } from 'node:url';
import { mkdirSync } from 'node:fs';
const SHOTS = fileURLToPath(new URL('./shots/', import.meta.url));
mkdirSync(SHOTS, { recursive: true });
const BASE = process.env.E2E_BASE ?? 'http://127.0.0.1:8090';
let pass = 0, fail = 0;
const ok = (c, m) => { (c ? pass++ : fail++); console.log((c ? 'LULUS: ' : 'GAGAL: ') + m); };
const b = await chromium.launch();
const ctx = await b.newContext({ viewport: { width: 1440, height: 900 } });
const p = await ctx.newPage();
const errors = [];
p.on('pageerror', (e) => errors.push('pageerror: ' + e.message));
// Peramban mencatat respons 4xx yang diharapkan (401 cek sesi, 401/422/429 uji gagal) sebagai galat konsol; itu bukan bug.
const expected = /Failed to load resource: the server responded with a status of (401|422|429)/;
p.on('console', (m) => m.type() === 'error' && !expected.test(m.text()) && errors.push('console: ' + m.text()));
const fill = async (email, pw) => { await p.getByLabel('Email').fill(email); await p.getByLabel('Password', { exact: true }).fill(pw); await p.getByRole('button', { name: /sign in/i }).click(); };

// 1. tanpa sesi -> diarahkan ke /login
await p.goto(BASE + '/assets');
await p.waitForURL('**/login');
ok(true, 'jalur terlindungi mengarahkan ke /login saat belum masuk');

// 2. salah password: pesan generik
await fill('admin@orchis.local', 'salah-salah');
const alert = p.getByRole('alert');
await alert.waitFor();
ok((await alert.textContent()).includes('Email or password is incorrect'), 'password salah -> pesan generik');
ok(!/attempt|tersisa/i.test(await alert.textContent()), 'pesan tidak membocorkan sisa percobaan');
await p.screenshot({ path: SHOTS + 'login-wrong.png' });

// 3. penguncian lewat email lain (agar admin tidak terkunci)
const ghost = `hantu-${Date.now()}@orchis.local`;
for (let i = 0; i < 5; i++) { await fill(ghost, 'salah-' + i); await p.waitForTimeout(150); }
await p.getByText('Too many attempts').waitFor();
ok(await p.getByRole('timer').isVisible(), 'layar akun terkunci tampil dengan hitung mundur');
const t = await p.getByRole('timer').textContent();
ok(/^1[45]:\d\d/.test(t.replace(/minutes remaining/, '').trim()), 'hitung mundur sekitar 15 menit: ' + t.trim());
await p.screenshot({ path: SHOTS + 'login-locked.png' });
await p.getByRole('button', { name: /back to sign in/i }).click();

// 4. masuk benar
await fill('admin@orchis.local', 'Orchis-Dev-Pass-1');
await p.waitForURL(BASE+'/overview');await p.goto(BASE+'/organizations');
ok(true, 'login benar -> beranda');
const cookies = await ctx.cookies();
const sc = cookies.find((c) => c.name === 'orchis_session');
ok(!!sc && sc.httpOnly && sc.sameSite === 'Lax', 'cookie sesi HttpOnly + SameSite=Lax');
await p.screenshot({ path: SHOTS + 'home.png' });

const orgResponse=await ctx.request.post(BASE+'/api/v1/organizations',{headers:{'X-Requested-With':'orchis-web'},data:{name:'Inventory laboratory',slug:'inventory-laboratory'}});
if(!orgResponse.ok())throw Error(await orgResponse.text());
const org=await orgResponse.json();

// 5. dock dan menu workspace
ok(await p.getByRole('navigation', { name: 'Primary' }).isVisible(), 'Nav Dock tampil');
await p.getByRole('button', { name: /platform default/i }).click();
await p.getByRole('menuitem', { name: /assets/i }).click();
await p.waitForURL('**/assets');
await p.getByRole('heading', { name: 'Assets', exact:true, level:1 }).waitFor();
await p.screenshot({ path: SHOTS + 'assets-empty.png' });
ok(await p.getByText('No assets yet').isVisible(), 'state kosong tampil');

// 6. tambah aset (wizard 2 langkah, 11 kategori)
await p.getByRole('button', { name: /add asset/i }).first().click();
const dlg = p.getByRole('dialog');
await dlg.waitFor();
ok((await dlg.locator('.category').count()) === 11, 'dialog menampilkan 11 kategori');
await p.screenshot({ path: SHOTS + 'add-step1.png' });
await dlg.getByRole('button', { name: /^Host/ }).click();
await dlg.getByLabel('Organisation',{exact:true}).selectOption(org.id);
await dlg.getByLabel('Addresses',{exact:true}).fill('192.0.2.10');
await dlg.getByRole('button', { name: /create asset/i }).click();
await dlg.getByText(/Name is required/).waitFor();
ok(true, 'galat validasi 422 tampil di kolom nama');
await dlg.getByLabel('Name', { exact: true }).fill('WINTERFELL');
await dlg.getByLabel('Platform', { exact: true }).selectOption('windows');
await dlg.getByLabel('Architecture').selectOption('x86_64');
await dlg.getByLabel('Criticality').selectOption('high');
await dlg.getByLabel('Addresses', { exact: true }).fill('192.0.2.10\nwinterfell.corp.local');
await dlg.getByLabel(/I confirm this asset may be tested/i).check();
await p.screenshot({ path: SHOTS + 'add-step2.png' });
await dlg.getByRole('button', { name: /create asset/i }).click();
await dlg.waitFor({ state: 'detached' });
await p.getByText('WINTERFELL').waitFor();
ok(true, 'aset dibuat dan muncul di daftar');
ok(await p.getByText('Authorized', { exact: true }).isVisible(), 'status otorisasi pengujian tampil');
// alamat yang tidak valid
await p.getByRole('button', { name: /add asset/i }).first().click();
await p.getByRole('dialog').getByRole('button', { name: /^Web application/ }).click();
await p.getByRole('dialog').getByLabel('Organisation',{exact:true}).selectOption(org.id);
await p.getByRole('dialog').getByLabel('Name', { exact: true }).fill('portal');
await p.getByRole('dialog').getByLabel('URLs', { exact: true }).fill('999.1.1.1');
await p.getByRole('dialog').getByRole('button', { name: /create asset/i }).click();
await p.getByRole('dialog').getByRole('alert').first().waitFor();
ok(true, 'alamat tidak valid ditolak server dan galatnya tampil');
await p.keyboard.press('Escape');
ok((await p.getByRole('dialog').count()) === 0, 'Esc menutup dialog');

// 7. pencarian dan filter
for (const n of ['host-a', 'web-b']) {
  await p.getByRole('button', { name: /add asset/i }).first().click();
  const d = p.getByRole('dialog');
  await d.getByRole('button', { name: n.startsWith('web') ? /^Web application/ : /^Host/ }).click();
  await d.getByLabel('Organisation',{exact:true}).selectOption(org.id);
  await d.getByLabel('Name', { exact: true }).fill(n);
  await d.getByLabel(n.startsWith('web')?'URLs':'Addresses',{exact:true}).fill(n.startsWith('web')?'http://127.0.0.1:1':'192.0.2.11');
  await d.getByRole('button', { name: /create asset/i }).click();
  await d.waitFor({ state: 'detached' });
}
await p.getByLabel('Search assets').fill('web');
await p.waitForTimeout(600);
ok((await p.locator('.asset-row').count()) === 1, 'pencarian teks menyaring daftar');
await p.getByLabel('Search assets').fill('');
await p.getByRole('combobox', { name: 'Category' }).selectOption('host');
await p.waitForTimeout(500);
ok((await p.locator('.asset-row').count()) === 2, 'filter kategori menyaring daftar');
await p.getByRole('combobox', { name: 'Category' }).selectOption('');
await p.waitForTimeout(500);
await p.screenshot({ path: SHOTS + 'assets-list.png' });

// 8. keyboard & aksesibilitas dasar
await p.getByRole('button', { name: /add asset/i }).first().focus();
const outline = await p.evaluate(() => getComputedStyle(document.activeElement).outlineStyle);
await p.keyboard.press('Tab'); await p.keyboard.press('Shift+Tab');
const o2 = await p.evaluate(() => { const s = getComputedStyle(document.activeElement); return s.outlineStyle + ' ' + s.outlineColor; });
ok(/solid/.test(o2), 'fokus keyboard terlihat (cincin perunggu): ' + o2);

// 9. 404 dan keluar
await p.goto(BASE + '/tidak-ada');
await p.getByText('We can’t find that page').waitFor();
ok(true, 'halaman 404 tampil di dalam kerangka');
await p.screenshot({ path: SHOTS + '404.png' });
await p.getByRole('button', { name: /platform default/i }).click();
await p.getByRole('menuitem', { name: /sign out/i }).click();
await p.waitForURL('**/login');
await p.goto(BASE + '/assets'); await p.waitForURL('**/login');
ok(true, 'setelah keluar, sesi tidak berlaku lagi');

ok(errors.length === 0, 'tidak ada galat JavaScript/konsol' + (errors.length ? ': ' + errors.slice(0, 3).join(' | ') : ''));
console.log(`\nRINGKASAN: lulus=${pass} gagal=${fail}`);
await b.close();
process.exit(fail ? 1 : 0);

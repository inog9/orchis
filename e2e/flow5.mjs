// E2E Fase 7: pengaturan, user + link reset, lupa password, profil + token API, integrasi, orang/tim, tag, MITRE, notifikasi, palet perintah.
import { chromium } from 'playwright';
import { fileURLToPath } from 'node:url';
import { mkdirSync } from 'node:fs';
const SHOTS = fileURLToPath(new URL('./shots/', import.meta.url));
mkdirSync(SHOTS, { recursive: true });
const BASE = process.env.E2E_BASE ?? 'http://127.0.0.1:8090';
const STRONG = 'Fresh-Strong-Pass-42';
let pass = 0, fail = 0;
const ok = (c, m) => { (c ? pass++ : fail++); console.log((c ? 'LULUS: ' : 'GAGAL: ') + m); };
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const b = await chromium.launch();
const errors = [];
const expected = /Failed to load resource: the server responded with a status of (400|401|403|404|409|422)/;
const watch = (page, who) => {
  page.on('pageerror', (e) => errors.push(`${who} pageerror: ${e.message}`));
  page.on('console', (m) => m.type() === 'error' && !expected.test(m.text()) && errors.push(`${who} console: ${m.text()}`));
};
const ctx = await b.newContext({ viewport: { width: 1440, height: 900 } });
const p = await ctx.newPage();
watch(p, 'admin');
const api = (method, path, data) => ctx.request.fetch(BASE + path, { method, data, headers: { 'X-Requested-With': 'orchis-web' } });
// "injector" yang disimulasikan lewat API (tanpa container) — memakai token yang dibuat lewat UI
const inj = (token, path, body) => fetch(BASE + '/api/v1/injector/' + path, { method: 'POST', headers: { Authorization: 'Bearer ' + token, 'Content-Type': 'application/json' }, body: JSON.stringify(body ?? {}) });
const login = async (page, email, password) => {
  await page.goto(BASE + '/login');
  await page.getByLabel('Email').fill(email);
  await page.getByLabel('Password', { exact: true }).fill(password);
  await page.getByRole('button', { name: /sign in/i }).click();
};
await login(p, 'admin@orchis.local', 'Orchis-Dev-Pass-1');
await p.waitForURL(BASE+'/overview');await p.goto(BASE+'/organizations');

// 1. Settings > Platform: ubah nama; komponen
await p.getByRole('button', { name: /Platform default/ }).click();
await p.getByRole('menuitem', { name: 'Settings' }).click();
await p.waitForURL('**/settings');
await p.getByRole('heading', { name: 'Settings', exact: true }).waitFor();
const tabs = await p.getByRole('navigation', { name: 'Settings sections' }).getByRole('link').allTextContents();
ok(['Platform', 'Users', 'Roles', 'Integrations', 'Tags'].every((t) => tabs.includes(t)), 'admin melihat semua tab Settings: ' + tabs.join(', '));
const comps = p.getByRole('region', { name: 'Components' }).or(p.locator('section[aria-labelledby=components-title]'));
await comps.getByText('PostgreSQL').waitFor();
ok(/PostgreSQL\s*Running/.test((await comps.innerText()).replace(/\n+/g, ' ')), 'kesehatan komponen: PostgreSQL Running');
ok(/RabbitMQ\s*Not configured/.test((await comps.innerText()).replace(/\n+/g, ' ')), 'komponen yang tidak dikonfigurasi ditandai jujur (bukan Running)');
await p.getByLabel('Platform name').fill('Acme Red Team');
await p.getByRole('button', { name: 'Save', exact: true }).click();
await p.getByRole('status').filter({ hasText: 'Saved.' }).waitFor();
ok(await p.getByRole('button', { name: /Acme Red Team/ }).isVisible(), 'nama platform baru tampil di pojok kanan atas');
await p.screenshot({ path: SHOTS + 'settings-platform.png' });

// 2. Users: buat akun, dapatkan link sekali pakai
await p.getByRole('link', { name: 'Users', exact: true }).click();
await p.getByRole('table', { name: 'Users' }).waitFor();
await p.getByRole('button', { name: /new user/i }).click();
let dlg = p.getByRole('dialog');
await dlg.getByLabel('Email').fill('not-an-email');
await dlg.getByLabel('Name').fill('Eve Analyst');
await dlg.getByRole('button', { name: 'Create user' }).click();
await dlg.getByText('Enter a valid email address.').waitFor();
ok(true, 'email tidak valid ditolak di kolomnya');
await dlg.getByLabel('Email').fill('eve@orchis.test');
await dlg.getByRole('button', { name: 'Create user' }).click();
const resetUrl = await p.getByTestId('reset-url').innerText();
ok(/\/reset\?token=orl_/.test(resetUrl), 'dialog menampilkan link reset sekali pakai');
await p.getByRole('button', { name: 'Done' }).click();
ok((await p.getByTestId('reset-url').count()) === 0, 'link tidak ditampilkan lagi setelah dialog ditutup');
ok(await p.getByRole('row', { name: /Eve Analyst/ }).getByText('Invited').isVisible(), 'akun baru berstatus Invited (belum punya password)');

// 3. Eve memakai link di browser lain
const eveCtx = await b.newContext({ viewport: { width: 1440, height: 900 } });
const e = await eveCtx.newPage();
watch(e, 'eve');
await e.goto(resetUrl);
await e.getByLabel('New password').fill('weak');
await e.getByLabel('Repeat the password').fill('different');
await e.getByText('The passwords don’t match.').waitFor();
ok(true, 'form reset: ketidakcocokan password ditandai');
await e.getByLabel('Repeat the password').fill('weak');
await e.getByRole('button', { name: 'Set password' }).click();
await e.getByText(/Use at least 12 characters/).waitFor();
ok(true, 'kebijakan password dari server ditampilkan');
await e.getByLabel('New password').fill(STRONG);
await e.getByLabel('Repeat the password').fill(STRONG);
await e.getByRole('button', { name: 'Set password' }).click();
await e.waitForURL('**/login');
await e.getByText('Password set').waitFor();
ok(true, 'setelah reset diarahkan ke login dengan pesan "Password set"');
await e.goto(resetUrl);
await e.getByRole('heading', { name: 'This link can’t be used' }).waitFor();
ok(true, 'link yang sudah dipakai tidak bisa dipakai lagi');
await login(e, 'eve@orchis.test', STRONG);
await e.waitForURL(BASE+'/overview');await e.goto(BASE+'/organizations');
await e.getByRole('button', { name: /Acme Red Team/ }).click();
ok(!(await e.getByRole('menuitem', { name: 'Settings' }).isVisible()), 'Eve (analyst) tidak melihat menu Settings: pengaturan hanya untuk Admin');
await e.goto(BASE + '/settings/users');
await e.getByRole('heading',{name:'Access needed'}).waitFor();
ok(await e.getByText(/Your role cannot open this page/i).isVisible(), 'membuka /settings/users langsung: ditolak server dengan pesan jelas, bukan data');

// 4. Lupa password dari layar login → alert untuk admin
const anon = await b.newContext();
const a2 = await anon.newPage();
watch(a2, 'anon');
await a2.goto(BASE + '/login');
await a2.getByRole('link', { name: /forgot/i }).click();
await a2.getByLabel('Email').fill('eve@orchis.test');
await a2.getByRole('button', { name: /ask for a reset link/i }).click();
await a2.getByRole('heading', { name: 'Ask your administrator' }).waitFor();
const a3 = await anon.newPage();
await a3.goto(BASE + '/forgot');
await a3.getByLabel('Email').fill('nobody-here@orchis.test');
await a3.getByRole('button', { name: /ask for a reset link/i }).click();
await a3.getByRole('heading', { name: 'Ask your administrator' }).waitFor();
ok(true, 'jawaban sama untuk email yang ada dan tidak ada (tidak membocorkan akun)');
await p.goto(BASE + '/overview');
await p.getByRole('button', { name: /Notifications, 1 unread/ }).waitFor({ timeout: 30000 });
ok(true, 'lonceng admin menampilkan 1 notifikasi belum dibaca');
await p.getByRole('button', { name: /Notifications, 1 unread/ }).click();
const pop = p.getByRole('dialog', { name: 'Notifications' });
ok(await pop.getByText('Eve Analyst asked for a password reset').isVisible(), 'popover memuat permintaan reset password');
await pop.getByRole('button', { name: /Eve Analyst asked/ }).click();
await p.waitForURL('**/settings/users');
await p.getByRole('button', { name: 'Notifications' }).waitFor();
ok(true, 'membuka notifikasi membawa admin ke Settings > Users dan menandainya dibaca');
await p.screenshot({ path: SHOTS + 'settings-users.png' });
const eveNotif = await e.goto(BASE + '/overview');
ok((await e.getByRole('button', { name: /^Notifications/ }).getAttribute('aria-label')) === 'Notifications', 'alert khusus admin tidak sampai ke analyst');

// 5. Admin menonaktifkan Eve: sesinya berakhir
await p.getByRole('row', { name: /Eve Analyst/ }).getByRole('button', { name: 'Edit Eve Analyst' }).click();
dlg = p.getByRole('dialog');
await dlg.getByLabel('Status').selectOption('disabled');
await dlg.getByRole('button', { name: 'Save' }).click();
await dlg.waitFor({ state: 'detached' });
await p.getByRole('row', { name: /Eve Analyst/ }).getByText('Disabled').waitFor();
// A background authenticated poll may already have ended the revoked session.
// Both immediate redirect and rejection on navigation must end at the login page.
if (!new URL(e.url()).pathname.endsWith('/login')) {
  await Promise.race([
    e.waitForURL('**/login', { timeout: 15000 }),
    e.getByRole('navigation', { name: 'Primary' }).getByRole('link', { name: 'Findings' }).click({ timeout: 3000 }),
  ]);
}
await e.waitForURL('**/login', { timeout: 15000 });
ok(true, 'akun dinonaktifkan: sesi Eve berakhir saat itu juga');
await login(e, 'eve@orchis.test', STRONG);
await e.getByText('Email or password is incorrect').waitFor();
ok(true, 'akun nonaktif tidak bisa masuk');

// 6. Profil: token API
await p.getByRole('button', { name: /Acme Red Team/ }).click();
await p.getByRole('menuitem', { name: 'Profile' }).click();
await p.locator('#profile-title').waitFor();
await p.getByRole('button', { name: /new token/i }).click();
dlg = p.getByRole('dialog');
await dlg.getByLabel('Name').fill('ci-pipeline');
await dlg.getByRole('button', { name: 'Create token' }).click();
const apiToken = await p.getByTestId('api-token').innerText();
ok(/^oat_/.test(apiToken), 'token API tampil sekali');
await p.getByRole('button', { name: 'Done' }).click();
const withToken = await fetch(BASE + '/api/v1/auth/me', { headers: { Authorization: 'Bearer ' + apiToken } });
ok(withToken.status === 200 && (await withToken.json()).email === 'admin@orchis.local', 'token API dipakai dari luar browser (tanpa cookie/CSRF)');
const listText = await p.getByRole('list', { name: 'API tokens' }).innerText();
ok(listText.includes('oat_') && !listText.includes(apiToken), 'daftar hanya memuat awalan token, bukan rahasianya');
await p.getByRole('button', { name: 'Revoke ci-pipeline' }).click();
await p.getByRole('dialog').getByRole('button', { name: 'Revoke token' }).click();
await p.getByText('You have no active tokens.').waitFor();
ok((await fetch(BASE + '/api/v1/auth/me', { headers: { Authorization: 'Bearer ' + apiToken } })).status === 401, 'token dicabut langsung tidak berlaku');
await p.getByLabel('Current password').fill('salah-salah-1');
await p.getByLabel('New password').fill(STRONG);
await p.getByRole('button', { name: 'Change password' }).click();
await p.getByText('That isn’t your current password.').waitFor();
ok(true, 'ganti password dengan password sekarang yang salah ditolak di kolomnya');

// 7. Integrations: putar token, jadikan online lewat heartbeat, nonaktifkan
await p.goto(BASE + '/settings/integrations');
await p.getByRole('list', { name: 'Nmap recent log' }).waitFor();
const nmapCard = p.getByRole('rowgroup').filter({has:p.getByText('Nmap',{exact:true})});
ok(await nmapCard.getByText('Offline').isVisible(), 'injector yang belum terhubung tampil Offline');
await nmapCard.getByRole('button', { name: 'Create token' }).click();
await p.getByRole('dialog').getByRole('button', { name: 'Create token' }).click();
const nmapToken = await p.getByTestId('injector-token').innerText();
await p.getByRole('button', { name: 'Done' }).click();
ok(/^oij_/.test(nmapToken), 'token injector tampil sekali');
ok((await inj(nmapToken, 'heartbeat', { version: '1.0.0' })).status === 200, 'injector (simulasi) check-in dengan token baru');
await p.reload();
await p.getByRole('rowgroup').filter({has:p.getByText('Nmap',{exact:true})}).getByText('Running').waitFor();
ok(/Injector connected \(version 1\.0\.0\)/.test(await p.getByRole('list', { name: 'Nmap recent log' }).innerText()), 'log menunjukkan sambungan injector');
await p.getByRole('rowgroup').filter({has:p.getByText('Nmap',{exact:true})}).getByRole('button', { name: 'Rotate token' }).click();
await p.getByRole('dialog').getByRole('button', { name: 'Rotate token' }).click();
const rotated = await p.getByTestId('injector-token').innerText();
await p.getByRole('button', { name: 'Done' }).click();
ok((await inj(nmapToken, 'heartbeat')).status === 401 && (await inj(rotated, 'heartbeat')).status === 200, 'token lama hangus setelah diputar; token baru berlaku');
await p.screenshot({ path: SHOTS + 'settings-integrations.png' });

// 8. People & Teams
await p.goto(BASE+'/overview');
await p.getByRole('heading',{name:'Overview',exact:true}).waitFor();
await p.keyboard.press('Control+k');
await p.getByRole('combobox', { name: 'Search or jump to' }).fill('peo');
await p.getByRole('option', { name: /People/ }).first().waitFor();
await p.keyboard.press('Enter');
await p.waitForURL('**/people');
ok(true, 'palet perintah (Ctrl+K): mengetik "peo" lalu Enter membuka People');
await p.getByRole('heading', { name: 'No people yet' }).waitFor();
await p.getByRole('button', { name: 'Add the first person' }).click();
dlg = p.getByRole('dialog');
await dlg.getByLabel('First name').fill('Ada');
await dlg.getByLabel('Last name').fill('Lovelace');
await dlg.getByLabel('Email (optional)').fill('ada@corp.test');
await dlg.getByRole('button', { name: 'Save' }).click();
await dlg.waitFor({ state: 'detached' });
await p.getByRole('button', { name: /add person/i }).click();
dlg = p.getByRole('dialog');
await dlg.getByLabel('First name').fill('Ada2');
await dlg.getByLabel('Email (optional)').fill('ADA@corp.test');
await dlg.getByRole('button', { name: 'Save' }).click();
await dlg.getByText('Someone with this email already exists.').waitFor();
ok(true, 'email ganda (beda huruf besar) ditolak di kolomnya');
await dlg.getByRole('button', { name: 'Cancel' }).click();
await p.getByRole('link', { name: 'Teams', exact: true }).click();
await p.getByRole('heading', { name: 'No teams yet' }).waitFor();
await p.getByRole('button', { name: /new team/i }).click();
dlg = p.getByRole('dialog');
await dlg.getByLabel('Name').fill('Blue team');
await dlg.getByRole('button', { name: 'Save' }).click();
await dlg.waitFor({ state: 'detached' });
await p.getByText('No members yet.').waitFor();
await p.getByRole('button', { name: 'Members' }).click();
dlg = p.getByRole('dialog');
await dlg.getByLabel(/Ada Lovelace/).check();
await dlg.getByRole('button', { name: 'Save members' }).click();
await dlg.waitFor({ state: 'detached' });
await p.getByText('1 member', { exact: false }).first().waitFor();
await p.getByRole('link', { name: 'People', exact: true }).click();
await p.getByRole('table', { name: 'People' }).getByText('Blue team').waitFor({ timeout: 8000 }); // daftar tersimpan lama dimuat ulang di latar belakang
ok(true, 'orang menampilkan tim yang diikutinya');
await p.getByLabel('Search people').fill('lovelace');
await p.getByRole('table', { name: 'People' }).getByText('Ada Lovelace').waitFor();
await p.getByLabel('Search people').fill('zzzz-none');
await p.getByRole('heading', { name: 'No one matches' }).waitFor();
ok(true, 'pencarian orang dan keadaan tanpa hasil');
await p.screenshot({ path: SHOTS + 'people.png' });

// 9. Tags: buat, ubah, hapus
await p.goto(BASE + '/settings/tags');
await p.getByRole('heading', { name: 'No tags yet' }).waitFor();
await p.getByRole('button', { name: /new tag/i }).click();
dlg = p.getByRole('dialog');
await dlg.getByLabel('Name').fill('crown-jewel');
await dlg.getByRole('button', { name: 'Save' }).click();
await dlg.waitFor({ state: 'detached' });
await p.getByRole('button', { name: 'Edit crown-jewel' }).click();
dlg = p.getByRole('dialog');
await dlg.getByLabel('Name').fill('crown-jewels');
await dlg.getByRole('button', { name: 'Save' }).click();
await dlg.waitFor({ state: 'detached' });
await p.getByRole('list', { name: 'Tags' }).getByText('crown-jewels').waitFor();
await p.getByRole('button', { name: 'Delete crown-jewels' }).click();
await p.getByRole('dialog').getByRole('button', { name: 'Delete tag' }).click();
await p.getByRole('heading', { name: 'No tags yet' }).waitFor();
ok(true, 'tag: buat, ganti nama, hapus');

// 10. MITRE: impor lalu matriks
await p.goto(BASE + '/settings');
await p.getByRole('heading', { name: 'MITRE ATT&CK data' }).waitFor();
const importBtn = p.getByRole('button', { name: /Import now|Importing…/ });
if (await importBtn.isEnabled()) await importBtn.click();
await p.goto(BASE + '/mitre');
await p.getByRole('region', { name: 'ATT&CK matrix' }).waitFor({ timeout: 60000 });
const cols = await p.getByRole('region', { name: 'ATT&CK matrix' }).getByRole('heading', { level: 2 }).allTextContents();
ok(cols.map((c) => c.replace(/\d+$/, '')).join('|') === 'Reconnaissance|Initial Access|Execution|Discovery', 'kolom matriks mengikuti urutan taktik ATT&CK: ' + cols.join(', '));
ok(await p.getByRole('button', { name: /T1046 .*mapped: an action can exercise it, not yet run/ }).isVisible(), 'T1046 (Nmap) dipetakan tetapi belum dijalankan');
ok(await p.getByRole('button', { name: /T1082 .*mapped: an action can exercise it/ }).isVisible(), 'T1082 dipetakan ke aksi agent.system_info');
ok(await p.getByRole('button', { name: /T1083 .*not covered/ }).isVisible(), 'teknik tanpa aksi: Not covered');
ok((await p.getByRole('button', { name: /T1000|T1001/ }).count()) === 0, 'teknik revoked/deprecated tidak tampil');
await p.getByLabel(/Only mapped techniques/).check();
ok((await p.getByRole('region', { name: 'ATT&CK matrix' }).getByRole('button').count()) === 3, 'filter hanya yang dipetakan: 3 teknik (T1046, T1595, T1082)');
await p.getByText('Only mapped techniques').click(); // klik labelnya: kotak centang kustom menutupi input

// jalankan nmap sukses lewat "injector" simulasi: T1046 menjadi Exercised
const asset = await (await api('POST', '/api/v1/assets', { category: 'host', name: 'e2e-host', testing_authorized: true, addresses: [{ kind: 'ip', value: '127.0.0.1' }] })).json();
const tst = await (await api('POST', '/api/v1/atomic-tests', { name: 'Ports', contract_key: 'nmap.tcp_connect', target_asset_id: asset.id })).json();
const run1 = await (await api('POST', `/api/v1/atomic-tests/${tst.id}/run`)).json();
await inj(rotated, `executions/${run1.id}/claim`);
ok((await inj(rotated, `executions/${run1.id}/result`, { exit_code: 0, findings: [] })).status === 204, 'scan nmap (simulasi) selesai sukses');
await p.reload();
await p.getByRole('button', { name: /T1046 .*trained: an action was run successfully/ }).waitFor();
ok(/Trained\s*1/.test((await p.getByRole('list', { name: 'Coverage' }).innerText()).replace(/\n+/g, ' ')), 'ringkasan cakupan: 1 teknik dilatih');
await p.getByRole('button', { name: /T1046/ }).click();
const panel = p.getByRole('region', { name: 'Technique details' });
await panel.getByText('Nmap TCP connect scan', { exact: false }).waitFor();
ok(await panel.getByRole('link', { name: /View on MITRE/ }).getAttribute('href') === 'https://attack.mitre.org/techniques/T1046/', 'detail teknik: aksi yang melatihnya dan tautan resmi');
await p.screenshot({ path: SHOTS + 'mitre.png' });

// 11. Notifikasi: scan gagal → alert; halaman Notifications
const run2 = await (await api('POST', `/api/v1/atomic-tests/${tst.id}/run`)).json();
await inj(rotated, `executions/${run2.id}/claim`);
await inj(rotated, `executions/${run2.id}/result`, { exit_code: 1, error: 'boom' });
await p.goto(BASE + '/overview');
await p.getByRole('button', { name: /Notifications, 1 unread/ }).waitFor({ timeout: 30000 });
await p.getByRole('button', { name: /Notifications, 1 unread/ }).click();
ok(await p.getByRole('dialog', { name: 'Notifications' }).getByText('Scan failed on e2e-host').isVisible(), 'scan gagal membuat alert');
await p.getByRole('button', { name: 'See all notifications' }).click();
await p.waitForURL('**/notifications');
await p.getByRole('list', { name: 'Notifications' }).getByText('Scan failed on e2e-host').waitFor();
await p.getByRole('button', { name: 'Mark all as read' }).click();
await p.getByRole('button', { name: 'Notifications', exact: true }).waitFor();
ok(true, 'tandai semua dibaca: lencana hilang');
await p.getByRole('button', { name: /Dismiss Scan failed on e2e-host/ }).click();
await p.getByRole('list', { name: 'Notifications' }).getByText('Scan failed on e2e-host').waitFor({ state: 'detached' });
ok(true, 'alert disingkirkan dari daftar');
await p.screenshot({ path: SHOTS + 'notifications.png' });

// 12. Pintasan g lalu huruf
await p.locator('body').click({ position: { x: 5, y: 5 } });
await p.keyboard.press('g');
await p.keyboard.press('m');
await p.waitForURL('**/simulations');
ok(true, 'pintasan "g lalu m" membuka Simulations');

ok(errors.length === 0, 'tidak ada error konsol/halaman tak terduga' + (errors.length ? ': ' + errors.join(' | ') : ''));
await b.close();
console.log(`\n${pass} lulus, ${fail} gagal`);
process.exit(fail ? 1 : 0);

// E2E Fase 2: detail aset, ubah/hapus, tag, grup dinamis, impor CSV, fokus dialog.
import { chromium } from 'playwright';
import { fileURLToPath } from 'node:url';
import { mkdirSync, mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
const SHOTS = fileURLToPath(new URL('./shots/', import.meta.url));
mkdirSync(SHOTS, { recursive: true });
const BASE = process.env.E2E_BASE ?? 'http://127.0.0.1:8090';
let pass = 0, fail = 0;
const ok = (c, m) => { (c ? pass++ : fail++); console.log((c ? 'LULUS: ' : 'GAGAL: ') + m); };
const b = await chromium.launch();
const p = await (await b.newContext({ viewport: { width: 1440, height: 900 } })).newPage();
const errors = [];
const expected = /Failed to load resource: the server responded with a status of (401|404|409|422)/;
p.on('pageerror', (e) => errors.push('pageerror: ' + e.message));
p.on('console', (m) => m.type() === 'error' && !expected.test(m.text()) && errors.push('console: ' + m.text()));
const rows = () => p.locator('.asset-row:not(.asset-row--skeleton)');

await p.goto(BASE + '/login');
await p.getByLabel('Email').fill('admin@orchis.local');
await p.getByLabel('Password', { exact: true }).fill('Orchis-Dev-Pass-1');
await p.getByRole('button', { name: /sign in/i }).click();
await p.waitForURL(BASE + '/organizations');
await p.goto(BASE + '/assets');
await p.getByRole('heading', { name: 'Assets', exact: true }).waitFor();

// 1. tab Endpoints | Asset groups; grup bawaan
ok(await p.getByRole('navigation', { name: 'Asset sections' }).isVisible(), 'tab Endpoints dan Asset groups tampil');
await p.getByRole('link', { name: 'Asset groups', exact: true }).click();
await p.waitForURL('**/assets/groups');
await p.getByRole('heading', { name: 'Asset groups', exact: true }).waitFor();
await p.locator('.card__name').first().waitFor(); // tunggu data, bukan kerangka loading (yang juga memakai .card)
const nGroups = await p.locator('.card__name').count();
ok(nGroups === 7, `7 grup bawaan tampil (All endpoints + 6 OS/arsitektur); sebenarnya ${nGroups}`);
await p.screenshot({ path: SHOTS + 'groups-list.png' });

// 2. buat aset lewat dialog: tag baru langsung dibuat, fokus kembali ke pemicu saat ditutup
await p.getByRole('link', { name: 'Endpoints', exact: true }).click();
await p.waitForURL('**/assets');
const addBtn = p.getByRole('button', { name: /add asset/i }).first();
await addBtn.click();
let dlg = p.getByRole('dialog');
await dlg.getByRole('button', { name: /^Host/ }).click();
await dlg.getByLabel('Name', { exact: true }).fill('WINTERFELL');
await dlg.getByLabel('Platform', { exact: true }).selectOption('windows');
await dlg.getByLabel('Architecture').selectOption('x86_64');
await dlg.getByLabel('Criticality').selectOption('critical');
await dlg.getByLabel('Addresses', { exact: true }).fill('74.234.220.121\nwinterfell.corp.local');
await dlg.getByLabel('New tag name').fill('crown-jewel');
await dlg.getByRole('button', { name: 'Add tag' }).click();
await dlg.getByRole('button', { name: 'crown-jewel' }).waitFor();
ok((await dlg.getByRole('button', { name: 'crown-jewel' }).getAttribute('aria-pressed')) === 'true', 'tag baru dibuat dan langsung terpilih');
await dlg.getByLabel(/I confirm this asset may be tested/i).check();
await dlg.getByRole('button', { name: /create asset/i }).click();
await dlg.waitFor({ state: 'detached' });
ok(await p.getByRole('link', { name: 'WINTERFELL' }).isVisible(), 'aset muncul di daftar sebagai tautan');
ok(await rows().first().getByText('crown-jewel').isVisible(), 'tag tampil di baris aset');

// 3. jebakan fokus + Esc mengembalikan fokus
await addBtn.click();
dlg = p.getByRole('dialog');
for (let i = 0; i < 40; i++) await p.keyboard.press('Tab');
const stillInside = await p.evaluate(() => !!document.activeElement?.closest('[role=dialog]'));
ok(stillInside, 'Tab berulang tidak keluar dari dialog (jebakan fokus)');
await p.keyboard.press('Escape');
await dlg.waitFor({ state: 'detached' });
const back = await p.evaluate(() => document.activeElement?.textContent?.trim());
ok(/Add asset/.test(back ?? ''), 'fokus kembali ke tombol pemicu setelah dialog ditutup: ' + back);

// 4. halaman detail
await p.getByRole('link', { name: 'WINTERFELL' }).click();
await p.getByRole('heading', { name: 'WINTERFELL' }).waitFor();
ok(await p.getByText('74.234.220.121').isVisible() && await p.getByText('winterfell.corp.local').isVisible(), 'detail menampilkan semua alamat');
ok(await p.getByRole('link', { name: 'All Windows x86_64' }).isVisible() && await p.getByRole('link', { name: 'All endpoints' }).isVisible(), 'detail menampilkan grup bawaan yang memuat aset ini');
ok(await p.getByText('Authorized', { exact: true }).isVisible(), 'status otorisasi tampil');
await p.screenshot({ path: SHOTS + 'asset-detail.png' });

// 5. ubah aset: platform ke Linux memindahkan grup
await p.getByRole('button', { name: /edit/i }).click();
dlg = p.getByRole('dialog');
await dlg.getByLabel('Platform', { exact: true }).selectOption('linux');
await dlg.getByLabel('Name', { exact: true }).fill('WINTERFELL-LINUX');
await dlg.getByRole('button', { name: /save changes/i }).click();
await dlg.waitFor({ state: 'detached' });
await p.getByRole('heading', { name: 'WINTERFELL-LINUX' }).waitFor();
ok(await p.getByRole('link', { name: 'All Linux x86_64' }).isVisible(), 'setelah platform diubah, aset pindah ke grup Linux');
ok((await p.getByRole('link', { name: 'All Windows x86_64' }).count()) === 0, 'dan keluar dari grup Windows');

// 6. konflik versi: dua tab mengubah aset yang sama
const p2 = await p.context().newPage();
await p2.goto(p.url());
await p2.getByRole('button', { name: /edit/i }).click();
await p2.getByRole('dialog').getByLabel('Name', { exact: true }).fill('DARI-TAB-DUA');
await p2.getByRole('dialog').getByRole('button', { name: /save changes/i }).click();
await p2.getByRole('heading', { name: 'DARI-TAB-DUA' }).waitFor();
await p.getByRole('button', { name: /edit/i }).click();           // tab pertama masih memegang versi lama
dlg = p.getByRole('dialog');
await dlg.getByLabel('Name', { exact: true }).fill('DARI-TAB-SATU');
await dlg.getByRole('button', { name: /save changes/i }).click();
await dlg.getByText('This asset changed').waitFor();
ok(true, 'perubahan di versi lama ditolak dengan pesan konflik (tidak menimpa diam-diam)');
await p.screenshot({ path: SHOTS + 'asset-conflict.png' });
await dlg.getByRole('button', { name: /reload the latest version/i }).click();
await p.getByRole('heading', { name: 'DARI-TAB-DUA' }).waitFor();
ok(true, 'memuat ulang menampilkan versi terbaru');
await p2.close();

// 7. grup kustom dengan pratinjau langsung
await p.goto(BASE + '/assets/groups');
await p.getByRole('button', { name: /new group/i }).click();
dlg = p.getByRole('dialog');
await dlg.getByLabel('Name', { exact: true }).fill('Crown jewels');
await dlg.getByRole('button', { name: /add rule/i }).click();
await dlg.getByLabel('Rule 1 field').selectOption('tag');
await dlg.getByLabel('Rule 1 value').fill('crown-jewel');
await dlg.getByText('1 of 1').waitFor();
ok(true, 'pratinjau menunjukkan jumlah aset yang cocok sebelum disimpan');
await p.screenshot({ path: SHOTS + 'group-dialog.png' });
await dlg.getByRole('button', { name: /create group/i }).click();
await p.getByRole('heading', { name: 'Crown jewels' }).waitFor();
ok(await p.getByRole('link', { name: 'DARI-TAB-DUA' }).isVisible(), 'detail grup menampilkan anggota yang cocok');
await p.screenshot({ path: SHOTS + 'group-detail.png' });

// 8. impor CSV dengan berkas sungguhan
await p.goto(BASE + '/assets');
await p.getByRole('button', { name: /import csv/i }).click();
dlg = p.getByRole('dialog');
const csv = 'hostname,os,arch,criticality,tags\n' +
  'srv-app-01,linux,x86_64,high,web;prod\n' +
  'srv-app-02,Solaris,x86_64,low,\n' +
  'srv-app-01,linux,arm64,low,\n' +
  '10.1.2.3,macos,aarch64,critical,prod\n';
const f = join(mkdtempSync(join(tmpdir(), 'orchis-e2e-')), 'e2e-assets.csv');
writeFileSync(f, csv);
await dlg.getByLabel(/choose a csv file/i).setInputFiles(f);
await dlg.getByText('Match your columns').waitFor();
ok((await dlg.getByLabel('Import column hostname as').inputValue()) === 'host', 'kolom hostname dipetakan otomatis ke "Hostname or IP"');
await dlg.getByText('2 valid').waitFor();
ok(await dlg.getByText('1 need fixing').isVisible() && await dlg.getByText('1 duplicate').isVisible(), 'ringkasan: 2 valid, 1 perlu diperbaiki, 1 duplikat');
ok(await dlg.getByText(/Row 3/).isVisible() && await dlg.getByText(/isn't supported. Use windows, linux or macos/).isVisible(), 'baris bermasalah disebut dengan nomor dan alasan');
const importBtn = dlg.getByRole('button', { name: /import 2 assets/i });
ok(await importBtn.isDisabled(), 'tombol impor nonaktif sampai pengguna memilih melewati baris bermasalah');
await p.screenshot({ path: SHOTS + 'import.png' });
await dlg.getByLabel(/skip the rows that need fixing/i).check();
await importBtn.click();
await dlg.getByText('Imported 2 assets').waitFor();
ok(await dlg.getByText(/not authorized for testing/i).isVisible(), 'hasil impor menjelaskan aset belum diotorisasi untuk diuji');
await dlg.getByRole('button', { name: 'Done' }).click();
await rows().nth(2).waitFor();
const n = await rows().count();
ok(n === 3, `daftar berisi 3 aset (1 awal + 2 impor); sebenarnya ${n}`);
await p.getByLabel('Search assets').fill('10.1');   // pencarian berdasarkan nama; IP tidak dicari
await p.getByLabel('Search assets').fill('srv-app');
await p.waitForTimeout(500);
ok((await rows().count()) === 1, 'aset hasil impor dapat dicari');
await p.getByLabel('Search assets').fill('');

// 9. hapus aset (admin) dengan konfirmasi
await p.getByRole('link', { name: 'srv-app-01' }).click();
await p.getByRole('button', { name: /remove asset/i }).click();
dlg = p.getByRole('dialog');
await dlg.getByRole('button', { name: 'Cancel' }).click();
ok(await p.getByRole('heading', { name: 'srv-app-01' }).isVisible(), 'batal pada konfirmasi tidak menghapus');
await p.getByRole('button', { name: /remove asset/i }).click();
await p.getByRole('dialog').getByRole('button', { name: /remove asset/i }).click();
await p.waitForURL('**/assets');
await p.waitForTimeout(400);
ok((await p.getByRole('link', { name: 'srv-app-01' }).count()) === 0, 'aset terhapus hilang dari daftar');
await p.goto(BASE + '/assets/groups');
await p.getByRole('link', { name: 'All endpoints' }).click();
await p.getByRole('heading', { name: 'All endpoints' }).waitFor();
ok((await p.getByRole('link', { name: 'srv-app-01' }).count()) === 0, 'aset terhapus juga keluar dari grup');
ok(await p.getByRole('button', { name: /edit/i }).count() === 0, 'grup bawaan tidak punya tombol ubah');

// 10. URL tidak dikenal di dalam kerangka
await p.goto(BASE + '/assets/00000000-0000-0000-0000-000000000000');
await p.getByText('Asset not found').waitFor();
ok(true, 'ID aset yang tidak ada menampilkan halaman "not found" yang jelas');

ok(errors.length === 0, 'tidak ada galat JavaScript/konsol' + (errors.length ? ': ' + errors.slice(0, 3).join(' | ') : ''));
console.log(`\nRINGKASAN FASE 2: lulus=${pass} gagal=${fail}`);
await b.close();
process.exit(fail ? 1 : 0);

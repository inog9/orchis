// Regresi navigasi setelah penyederhanaan; latency sesi dikendalikan, bukan sleep.
import assert from 'node:assert/strict';
import { chromium } from 'playwright';
const BASE = process.env.E2E_BASE ?? 'http://127.0.0.1:18090';
const browser = await chromium.launch();
const ctx = await browser.newContext();
const page = await ctx.newPage();
const errors = [];
page.on('pageerror', error => errors.push(error.message));
let checks = 0;
const check = (value, message) => { assert(value, message); checks++; console.log('LULUS: ' + message); };
const primary = () => page.getByRole('navigation', { name: 'Primary', exact: true });
const palette = () => page.getByRole('combobox', { name: 'Search or jump to', exact: true });
async function workspaceReady() {
  await primary().waitFor();
  await page.getByRole('heading', { name: 'Organisations', exact: true }).waitFor();
}
try {
  await page.goto(BASE + '/login');
  await page.getByLabel('Email').fill('admin@orchis.local');
  await page.getByLabel('Password', { exact: true }).fill('Orchis-Dev-Pass-1');
  await page.getByRole('button', { name: 'Sign in', exact: true }).click();
  await page.waitForURL(BASE+'/overview');await page.goto(BASE+'/organizations');
  await workspaceReady();
  const me = await (await ctx.request.get(BASE + '/api/v1/auth/me')).json();
  check(me.features.advanced === false, 'regresi berjalan dengan fitur lanjutan mati');
  const destinations = await primary().getByRole('link').evaluateAll(links => links.map(link => new URL(link.href).pathname));
  assert.deepEqual(destinations, ['/overview', '/organizations', '/assets', '/simulations', '/simulations/schedules', '/findings', '/reports', '/settings']);
  checks++; console.log('LULUS: current workspace destinations include Overview and audit schedules');

  await page.goto(BASE + '/');
  await page.locator('#public-title').waitFor();
  check(new URL(page.url()).pathname === '/', 'root retains the public product landing page');

  // Kontrol negatif: tahan jawaban /me sehingga Shell pasti belum dipasang.
  // Pola goto + langsung Ctrl+K dari flow5 lama kehilangan input pada kondisi ini.
  let releaseAuth;
  const authGate = new Promise(resolve => { releaseAuth = resolve; });
  let requestArrived;
  const authArrived = new Promise(resolve => { requestArrived = resolve; });
  await page.route('**/api/v1/auth/me', async route => { requestArrived(); await authGate; await route.continue(); });
  try {
    await page.goto(BASE + '/organizations', { waitUntil: 'domcontentloaded' });
    await authArrived;
    check(await primary().count() === 0, 'respons sesi tertahan: Shell belum tersedia');
    await page.keyboard.press('Control+k');
    check(await palette().count() === 0, 'Ctrl+K sebelum Shell siap tidak membuka palet');
  } finally {
    releaseAuth();
  }
  await workspaceReady();
  await page.unroute('**/api/v1/auth/me');
  await page.locator('body').click({ position: { x: 5, y: 5 } });
  await page.keyboard.press('Control+k');
  await palette().waitFor();
  check(await palette().isVisible(), 'setelah kesiapan eksplisit, Ctrl+K membuka palet');
  await page.keyboard.press('Escape');
  await palette().waitFor({ state: 'detached' });

  await page.locator('body').click({ position: { x: 5, y: 5 } });
  await page.keyboard.press('g');
  await page.keyboard.press('m');
  await page.waitForURL(BASE + '/simulations');
  await page.getByRole('heading', { name: 'Scans', exact: true }).first().waitFor();
  check(await primary().getByRole('link', { name: 'Scans', exact: true }).getAttribute('aria-current') === 'page', 'g lalu m membuka Simulations sesuai menu');
  check(errors.length === 0, 'tanpa galat runtime browser');
} finally {
  await ctx.close();
  await browser.close();
}
console.log(`\nNavigasi deterministik: ${checks} lulus, 0 gagal`);

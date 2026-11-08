import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { OrganizationsPage, OrganizationDetailPage } from './OrganizationsPage';
import { pageList } from '../components/orgs/Pagination';

const perms = ['organization.read', 'organization.write', 'asset.read', 'agent.read', 'report.read', 'simulation.run', 'assessment.run'];
const me = { id: 'u', email: 'a@b.c', display_name: 'Ada', role: 'admin', permissions: perms };
const json = (v: unknown, status = 200) => new Response(JSON.stringify(v), { status });
const orgs = (n: number) => Array.from({ length: n }, (_, i) => ({ id: `o${String(i).padStart(2, '0')}`, name: `Org ${String(i).padStart(2, '0')}`, slug: `org-${i}`, description: '', industry: i % 2 ? 'retail' : 'animal', contact: '', version: 1 }));

function route(n: number, opts: { fail?: boolean } = {}) {
  let failed = false;
  return vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
    const u = new URL(String(input), 'http://x');
    if (u.pathname.endsWith('/auth/me')) return json(me);
    if (u.pathname === '/api/v1/organizations') {
      if (opts.fail && !failed) { failed = true; return json({ title: 't', status: 500, error_id: 'err_1' }, 500); }
      return json({ items: orgs(n), next_cursor: null });
    }
    if (u.pathname === '/api/v1/home') return json({ organizations: orgs(n).map((o) => ({ id: o.id, name: o.name, assets: 7, authorized: 1, risk: '', comparison: '', latest_assessment: null, latest_status: '' })), assets: { total: 7 * n, authorized: 1, internet_facing: 0 }, findings: { total: 5, vulnerabilities: 5, open_ports: 0, vulnerable_assets: 1, by_severity: { critical: 1, high: 2 }, new_7d: 0 }, atomic: null, chains: null, latest_simulations: [], latest_runs: [], most_exposed: [], injectors: [], generated_at: '' });
    if (u.pathname === '/api/v1/agents') return json({ items: [{ id: 'a1', organization_id: 'o00', status: 'online', os: 'linux', arch: 'x86_64' }, { id: 'a2', organization_id: 'o00', status: 'offline', os: 'linux', arch: 'x86_64' }] });
    if (u.pathname === '/api/v1/reports') return json({ items: u.searchParams.get('organization_id') === 'o00' ? [{ id: 'r', created_at: new Date(Date.now() - 3 * 3600e3).toISOString(), snapshot: { by_severity: { high: 2, low: 1 } } }] : [] });
    return json({ items: [] });
  });
}
function mount(path = '/organizations') {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={qc}><MemoryRouter initialEntries={[path]}><OrganizationsPage /></MemoryRouter></QueryClientProvider>);
}

beforeEach(() => localStorage.clear());
afterEach(() => vi.restoreAllMocks());

describe('OrganizationsPage', () => {
  it('kartu memuat metrik nyata: aset, agent online, temuan terbuka, status dan pindai terakhir', async () => {
    route(3);
    mount();
    const card = (await screen.findByRole('link', { name: 'Org 00' })).closest('.oc') as HTMLElement;
    expect(within(card).getByText('Animal')).toBeInTheDocument();
    await waitFor(() => expect(within(card).getByText('3 open')).toBeInTheDocument());
    expect(within(card).getByText('1 online')).toBeInTheDocument();
    expect(within(card).getByText('7')).toBeInTheDocument();
    expect(within(card).getByText('Needs attention')).toBeInTheDocument();
    expect(within(card).getByRole('img', { name: /high 2, low 1/ })).toBeInTheDocument();
    expect(screen.getByRole('heading', { level: 1, name: 'Organisations' })).toBeInTheDocument();
  });

  it('paginasi tersembunyi untuk satu halaman, muncul dengan "Showing" untuk 14 organisasi', async () => {
    route(3);
    const { unmount } = mount();
    await screen.findByRole('link', { name: 'Org 00' });
    expect(screen.queryByRole('navigation', { name: /pagination/i })).toBeNull();
    unmount();
    vi.restoreAllMocks();
    route(14);
    mount();
    await screen.findByRole('link', { name: 'Org 00' });
    expect(screen.getByText('Showing 1–12 of 14')).toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'Org 13' })).toBeNull();
    await userEvent.click(screen.getByRole('button', { name: 'Next' }));
    expect(await screen.findByRole('link', { name: 'Org 13' })).toBeInTheDocument();
    expect(screen.getByText('Showing 13–14 of 14')).toBeInTheDocument();
  });

  it('pencarian menyaring dan keadaan "tidak cocok" bisa dibersihkan', async () => {
    route(4);
    mount();
    await screen.findByRole('link', { name: 'Org 00' });
    await userEvent.type(screen.getByLabelText('Search organisations'), 'zzz');
    expect(await screen.findByText('No organisations match')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Clear filters' }));
    expect(await screen.findByRole('link', { name: 'Org 00' })).toBeInTheDocument();
  });

  it('pilihan tampilan daftar diingat', async () => {
    route(2);
    mount();
    await screen.findByRole('link', { name: 'Org 00' });
    await userEvent.click(screen.getByRole('button', { name: 'List view' }));
    expect(localStorage.getItem('orchis.orgs.view')).toBe('list');
    expect(document.querySelector('.orgs__list--list')).not.toBeNull();
  });

  it('keadaan kosong mengajak membuat organisasi', async () => {
    route(0);
    mount();
    expect(await screen.findByText('No organisations yet')).toBeInTheDocument();
    expect(screen.getAllByRole('button', { name: /create organisation/i }).length).toBeGreaterThan(0);
  });

  it('galat tampil sebagai banner dan bisa dicoba lagi', async () => {
    route(2, { fail: true });
    mount();
    expect(await screen.findByRole('alert')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: /try again/i }));
    expect(await screen.findByRole('link', { name: 'Org 00' })).toBeInTheDocument();
  });
});

describe('pageList', () => {
  it('memakai elipsis untuk banyak halaman', () => {
    expect(pageList(1, 5)).toEqual([1, 2, 3, 4, 5]);
    expect(pageList(6, 12)).toEqual([1, '…', 5, 6, 7, '…', 12]);
  });
});

// Detail organisasi (HALAMAN 5): heading, metric strip, tabel aset, kartu agen.
const DETAIL_PERMS = ['organization.read', 'organization.write', 'asset.read', 'asset.write', 'asset.authorize', 'agent.read', 'agent.manage', 'report.read', 'finding.read', 'assessment.read', 'assessment.run', 'simulation.run'];
const detailMe = { id: 'u', email: 'a@b.c', display_name: 'Ada', role: 'admin', permissions: DETAIL_PERMS };
const detailOrg = { id: 'o1', name: 'Juice Shop', slug: 'juice-shop', description: 'Demo shop', industry: 'retail', contact: 'sec@juice.sh', version: 1 };
const detailReadiness = {
  total: 3, authorized: 2, ready: 1, scan_window_open: true, emergency_stop: false,
  items: [
    { id: 'a1', name: 'Guntur Mac', authorized: true, reason: '', web_reason: '', defense_reason: '', os: 'macos', arch: 'arm64', agent_status: 'online', url: 'https://guntur.local', last_scan_at: new Date(Date.now() - 3600e3).toISOString(), last_scan_status: 'finished' },
    { id: 'a2', name: 'Victus', authorized: false, reason: 'Not authorized for testing', web_reason: 'Not authorized for testing', defense_reason: 'No agent is installed on this asset.', agent_status: 'none' },
  ],
};
const detailAgents = {
  items: [
    { id: 'ag1', organization_id: 'o1', asset_id: 'a1', asset_name: 'Guntur Mac', hostname: 'Gunturs-MacBook-Pro.local', os: 'macos', arch: 'arm64', mode: 'service', version: '1.4.0', status: 'online', last_heartbeat_at: new Date(Date.now() - 60e3).toISOString(), enrolled_at: new Date().toISOString(), update_available: false },
    { id: 'ag2', organization_id: 'o1', asset_id: 'a2', asset_name: 'Victus', hostname: 'inoginn-Victus', os: 'linux', arch: 'x86_64', mode: 'service', version: '1.4.0', status: 'offline', last_heartbeat_at: new Date(Date.now() - 3600e3).toISOString(), enrolled_at: new Date().toISOString(), update_available: false },
    { id: 'ag3', organization_id: 'o2', asset_id: 'a3', asset_name: 'Other', hostname: 'other-host', os: 'linux', arch: 'x86_64', mode: 'service', version: '1.4.0', status: 'online', last_heartbeat_at: new Date().toISOString(), enrolled_at: new Date().toISOString(), update_available: false },
  ],
};
function detailRoute(opts: { paused?: boolean; windowOpen?: boolean; empty?: boolean; failReadiness?: boolean } = {}) {
  return vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
    const u = new URL(String(input), 'http://x');
    if (u.pathname.endsWith('/auth/me')) return json(detailMe);
    if (u.pathname === '/api/v1/organizations/o1') return json(detailOrg);
    if (u.pathname === '/api/v1/organizations/o1/readiness') {
      if (opts.failReadiness) return json({ title: 't', status: 500, error_id: 'err_1' }, 500);
      if (opts.empty) return json({ total: 0, authorized: 0, ready: 0, scan_window_open: true, emergency_stop: false, items: [] });
      return json({ ...detailReadiness, scan_window_open: opts.windowOpen ?? true });
    }
    if (u.pathname === '/api/v1/organizations/o1/scan-control') return json({ paused: opts.paused ?? false, global_paused: false, window: {}, version: 0, allowed_now: !(opts.paused ?? false) });
    if (u.pathname === '/api/v1/agents') return json(detailAgents);
    return json({ items: [], next_cursor: null });
  });
}
function mountDetail(path = '/organizations/o1') {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={qc}><MemoryRouter initialEntries={[path]}><Routes><Route path="/organizations/:id" element={<OrganizationDetailPage />}/></Routes></MemoryRouter></QueryClientProvider>);
}

describe('OrganizationDetailPage', () => {
  it('menampilkan heading, aksi, metric strip, tabel aset, dan kartu agen dari data nyata', async () => {
    detailRoute();
    mountDetail();
    expect(await screen.findByRole('heading', { level: 1, name: 'Juice Shop' })).toBeInTheDocument();
    expect(screen.getByText('Organisation')).toBeInTheDocument();
    expect(screen.getByText('juice-shop · Demo shop')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'View reports' })).toHaveAttribute('href', '/reports?organization_id=o1');
    expect(screen.getByRole('link', { name: 'Run scan' })).toHaveAttribute('href', '/simulations/new?org=o1');
    // Tabel aset menunggu readiness selesai dimuat; nama aksesibel ada di region pembungkusnya.
    const region = await screen.findByRole('region', { name: 'Assets in this organisation' });
    const table = within(region).getByRole('table');
    // Metric strip di atas tab: kesiapan, diotorisasi, agen online, jendela pindai.
    expect(screen.getByText('Scan readiness')).toBeInTheDocument();
    expect(screen.getByText('1/3')).toBeInTheDocument();
    expect(screen.getByText('2 authorized')).toBeInTheDocument();
    expect(screen.getByText('Authorized assets')).toBeInTheDocument();
    expect(screen.getByText('of 3 assets in total')).toBeInTheDocument();
    await waitFor(() => expect(screen.getByText('of 2 installed')).toBeInTheDocument());
    expect(screen.getByText('Agents online')).toBeInTheDocument();
    expect(screen.getByText('Scan window')).toBeInTheDocument();
    expect(screen.getByText('Open')).toBeInTheDocument();
    expect(screen.getByText('not paused · emergency stop: no')).toBeInTheDocument();
    // Isi tabel aset: nama tertaut, platform, titik agent online, status pindai terakhir, waktu.
    expect(within(table).getByRole('link', { name: 'Guntur Mac' })).toHaveAttribute('href', '/assets/a1');
    expect(within(table).getAllByRole('row')).toHaveLength(3);
    expect(within(table).getByText('macos · arm64')).toBeInTheDocument();
    expect(within(table).getByText('Online')).toBeInTheDocument();
    expect(within(table).getByText('Finished')).toBeInTheDocument();
    expect(within(table).getByText('Never scanned')).toBeInTheDocument();
    // Kartu agen terpasang: hostname, tag status, versi, heartbeat.
    expect(screen.getByRole('heading', { name: 'Installed agents' })).toBeInTheDocument();
    expect(screen.getByText('Gunturs-MacBook-Pro.local')).toBeInTheDocument();
    expect(screen.getAllByText('1.4.0')).toHaveLength(2);
    expect(screen.getAllByText('Last heartbeat')).toHaveLength(2);
    // Semua tab tetap ada.
    for (const name of ['Overview', 'Agents/Endpoints', 'Assets', 'Scans', 'Findings', 'Reports']) {
      expect(screen.getByRole('button', { name })).toBeInTheDocument();
    }
  });

  it('jendela pindai tertutup dengan caption dijeda dan emergency stop dari scan-control', async () => {
    detailRoute({ paused: true, windowOpen: false });
    mountDetail();
    expect(await screen.findByText('Closed')).toBeInTheDocument();
    expect(screen.getByText('paused · emergency stop: yes')).toBeInTheDocument();
  });

  it('keadaan kosong pada bagian aset mengajak menambah aset', async () => {
    detailRoute({ empty: true });
    mountDetail();
    expect(await screen.findByText('No assets yet')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Add an asset →' })).toHaveAttribute('href', '/organizations/o1?tab=assets&new=1');
  });

  it('galat readiness tampil dengan tombol coba lagi', async () => {
    detailRoute({ failReadiness: true });
    mountDetail();
    expect((await screen.findAllByRole('alert')).length).toBeGreaterThan(0);
  });
});

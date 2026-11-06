import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { OrganizationsPage } from './OrganizationsPage';
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

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { FindingsPage } from './FindingsPage';

const me = { id: 'u', email: 'a@b.c', display_name: 'Ada', role: 'admin', permissions: ['finding.read', 'asset.read', 'organization.read', 'report.read'] };
const json = (v: unknown, status = 200) => new Response(JSON.stringify(v), { status });
const ago = (d: number) => new Date(Date.now() - d * 86_400_000).toISOString();
const finding = (o: Record<string, unknown>) => ({ id: 'f', asset_id: 'a1', asset_name: 'host-a', type: 'vulnerability', value: 'v', title: 'T', severity: 'high', details: { k: 1 }, first_seen_at: ago(30), last_seen_at: ago(1), occurrence_count: 2, organization_id: 'o1', organization_name: 'Org Uno', ...o });
const data = [
  finding({ id: '1', title: 'Judul kritis', severity: 'critical', first_seen_at: ago(1) }),
  finding({ id: '2', title: 'Judul rendah', severity: 'low', asset_id: 'a2', asset_name: 'host-b', organization_id: 'o2', organization_name: 'Org Dua' }),
  finding({ id: '3', title: 'Judul tinggi', severity: 'high' }),
];
const home = { organizations: [], assets: { total: 9, authorized: 0, internet_facing: 0 }, findings: { total: 3, vulnerabilities: 2, open_ports: 1, vulnerable_assets: 2, by_severity: { critical: 1, high: 1, low: 1 }, new_7d: 1 }, atomic: null, chains: null, latest_simulations: [], latest_runs: [], most_exposed: [], injectors: [], generated_at: '' };

function route(opts: { items?: unknown[]; fail?: boolean; urls?: string[] } = {}) {
  return vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
    const u = new URL(String(input), 'http://x');
    opts.urls?.push(u.pathname + u.search);
    if (u.pathname.endsWith('/auth/me')) return json(me);
    if (u.pathname === '/api/v1/home') return json(home);
    if (u.pathname === '/api/v1/findings') return opts.fail ? json({ title: 't', status: 500, error_id: 'e' }, 500) : json({ items: opts.items ?? data, next_cursor: null, search: 'none' });
    return json({ items: [] });
  });
}
function mount(props: Record<string, string> = {}) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}><MemoryRouter><FindingsPage {...props} /></MemoryRouter></QueryClientProvider>);
}
beforeEach(() => localStorage.clear());
afterEach(() => vi.restoreAllMocks());

describe('FindingsPage', () => {
  it('kartu statistik memakai angka dari /home dan kartu Kritis+Tinggi menerapkan filter', async () => {
    route(); mount();
    const group = await screen.findByRole('group', { name: /critical \+ high/i });
    await waitFor(() => expect(within(group).getByText('2')).toBeInTheDocument());
    await userEvent.click(group.closest('button')!);
    expect((await screen.findAllByRole('button', { name: /critical/i, pressed: true }))[0]).toBeInTheDocument();
    expect(screen.getByText('Severity: High')).toBeInTheDocument();
  });

  it('satu severity dikirim ke server; dua severity disaring di klien; Hapus semua mereset', async () => {
    const urls: string[] = []; route({ urls }); mount();
    await screen.findByText('Judul kritis');
    const chips = screen.getByRole('group', { name: /filter by severity/i });
    await userEvent.click(within(chips).getByRole('button', { name: /critical/i }));
    await waitFor(() => expect(urls.some((u) => u.includes('severity=critical'))).toBe(true));
    await userEvent.click(within(chips).getByRole('button', { name: /high/i }));
    await waitFor(() => expect(screen.queryByText('Judul rendah')).not.toBeInTheDocument());
    expect(screen.getByText('Judul tinggi')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: /clear all/i }));
    expect(await screen.findByText('Judul rendah')).toBeInTheDocument();
  });

  it('pengalih tampilan: per organisasi (bawaan), per aset, daftar datar', async () => {
    route(); mount();
    await screen.findByText('Judul kritis');
    expect(screen.getByRole('heading', { name: 'Org Uno' })).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'By asset' }));
    expect(screen.getByRole('heading', { name: 'host-b' })).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Flat list' }));
    expect(screen.queryByRole('heading', { name: 'host-b' })).not.toBeInTheDocument();
    expect(localStorage.getItem('orchis.findings.view')).toBe('"flat"');
  });

  it('kelompok bisa dilipat dan status lipat diingat', async () => {
    route(); mount();
    await screen.findByText('Judul kritis');
    await userEvent.click(screen.getByRole('button', { name: 'Org Uno' }));
    expect(screen.queryByText('Judul kritis')).not.toBeInTheDocument();
    expect(localStorage.getItem('orchis.findings.collapsed')).toContain('o-');
  });

  it('penanda Baru hanya untuk temuan 7 hari terakhir', async () => {
    route(); mount();
    const row = (await screen.findByText('Judul kritis')).closest('li')!;
    expect(within(row).getByText('New')).toBeInTheDocument();
    expect(within(screen.getByText('Judul tinggi').closest('li')!).queryByText('New')).toBeNull();
  });

  it('drawer: buka, tampilkan rincian JSON, Esc menutup, fokus kembali ke baris', async () => {
    route(); mount();
    const btn = (await screen.findByText('Judul tinggi')).closest('button')!;
    await userEvent.click(btn);
    const dlg = await screen.findByRole('dialog', { name: 'Judul tinggi' });
    expect(within(dlg).getAllByText('k').length).toBeGreaterThan(0); // rincian kini pasangan kunci-nilai, JSON utuh lewat Salin (AUDIT UI-016)
    expect(within(dlg).getByText('1', { exact: true })).toBeInTheDocument();
    await userEvent.keyboard('{Escape}');
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(btn).toHaveFocus();
  });

  it('galat: role=alert dengan tombol coba lagi; kosong tanpa filter dan dengan filter', async () => {
    route({ fail: true }); mount();
    expect(await screen.findByRole('alert')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /try again|retry/i })).toBeInTheDocument();
  });

  it('kosong dengan filter menawarkan hapus filter', async () => {
    route({ items: [] }); mount();
    await screen.findByText(/no findings yet|belum ada/i);
    await userEvent.type(screen.getByRole('searchbox'), 'zzz');
    await userEvent.click(await screen.findByRole('button', { name: 'Clear filters' }));
    expect(screen.getByRole('searchbox')).toHaveValue('');
  });

  it('di konteks organisasi: tanpa kartu statistik dan tanpa pengelompokan per organisasi', async () => {
    route(); mount({ organizationID: 'o1', organizationName: 'Org Uno' });
    await screen.findByText('Judul kritis');
    expect(screen.queryByRole('group', { name: /total findings/i })).toBeNull();
    expect(screen.queryByRole('button', { name: 'By organisation' })).toBeNull();
  });
});

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ScanDetailPage, ScanList } from './ScansPage';

const perms = ['organization.read', 'asset.read', 'finding.read', 'report.read', 'assessment.read', 'assessment.run', 'simulation.read', 'simulation.run'];
const json = (v: unknown, status = 200) => new Response(JSON.stringify(v), { status });
const scan = (o: Record<string, unknown>) => ({ id: 'aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee', organization_id: 'o1', name: 'n', status: 'finished', runner: 'injector', options: { tools: ['nmap', 'nuclei'], preset: 'quick', asset_ids: [], range_ids: [], organization_id: 'o1' }, created_at: '2026-01-01T00:00:00Z', ended_at: '2026-01-01T00:03:12Z', ...o });
const job = (o: Record<string, unknown>) => ({ id: 'j', asset_id: 'a1', range_id: null, name: 'host-x', target: 'host-x.test', tool: 'nmap', status: 'finished', error: '', simulation_id: null, findings: [], ...o });

function route(handler: (u: URL) => unknown | undefined) {
  return vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
    const u = new URL(String(input), 'http://x');
    if (u.pathname.endsWith('/auth/me')) return json({ id: 'u', permissions: perms });
    if (u.pathname === '/api/v1/organizations') return json({ items: [{ id: 'o1', name: 'Org Uno', slug: 'o', description: '', industry: '', contact: '', version: 1 }], next_cursor: null });
    if (u.pathname === '/api/v1/organizations/o1') return json({ name: 'Org Uno' });
    const r = handler(u);
    return r === undefined ? json({ items: [], next_cursor: null }) : r instanceof Response ? r : json(r);
  });
}
const mount = (ui: React.ReactNode, path: string, pattern = '*') => render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><MemoryRouter initialEntries={[path]}><Routes><Route path={pattern} element={ui} /></Routes></MemoryRouter></QueryClientProvider>);
afterEach(() => vi.restoreAllMocks());

describe('ScanList', () => {
  it('menampilkan baris: organisasi, status bertulisan, alat, preset, durasi', async () => {
    route((u) => u.pathname === '/api/v1/scans' ? { items: [scan({}), scan({ id: 'bbbbbbbb-1111-2222-3333-444444444444', status: 'running', ended_at: null })], next_cursor: null } : undefined);
    mount(<ScanList />, '/simulations');
    const table = await screen.findByRole('table', { name: 'Scans' });
    await waitFor(() => expect(within(table).getAllByText('Org Uno')).toHaveLength(2));
    expect(within(table).getByText('Finished')).toBeInTheDocument();
    expect(within(table).getByText('Running')).toBeInTheDocument();
    expect(within(table).getAllByText('Hackbot')).toHaveLength(2);
    expect(within(table).getByText('3m 12s')).toBeInTheDocument();
    expect(within(table).getAllByText('—').length).toBeGreaterThan(0);
  });
  it('kosong tanpa filter dan dengan filter organisasi (No results + tombol tampilkan semua)', async () => {
    route(() => ({ items: [], next_cursor: null }));
    mount(<ScanList />, '/simulations?org=o1');
    expect(await screen.findByText('No results')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Show all organisations' }));
    expect(await screen.findByText('No scans yet')).toBeInTheDocument();
  });
  it('galat: role=alert dan Coba lagi memuat ulang', async () => {
    let n = 0;
    route((u) => u.pathname === '/api/v1/scans' ? (++n === 1 ? json({ title: 't', status: 500, error_id: 'e' }, 500) : { items: [scan({})], next_cursor: null }) : undefined);
    mount(<ScanList />, '/simulations');
    expect(await screen.findByRole('alert')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Try again' }));
    expect(await screen.findByRole('table', { name: 'Scans' })).toBeInTheDocument();
  });
});

describe('ScanDetailPage', () => {
  it('selesai: statistik, linimasa hanya created_at/ended_at, tabel temuan terurut severity', async () => {
    const f = (severity: string, title: string) => ({ id: title, type: 'vulnerability', value: 'v-' + title, title, severity, evidence: {} });
    route((u) => u.pathname === '/api/v1/scans/s1' ? scan({ id: 's1', jobs: [job({ id: 'j1', findings: [f('low', 'Rendah')] }), job({ id: 'j2', tool: 'nuclei', findings: [f('critical', 'Kritis')] })] }) : undefined);
    mount(<ScanDetailPage />, '/simulations/s1', '/simulations/:id');
    expect(await screen.findByRole('heading', { name: 'Org Uno' })).toBeInTheDocument();
    expect(screen.getByText('2 / 2')).toBeInTheDocument();
    const table = screen.getByRole('table', { name: 'Findings produced' });
    const rows = within(table).getAllByRole('row').slice(1);
    expect(rows[0]).toHaveTextContent('Kritis');
    expect(rows[1]).toHaveTextContent('Rendah');
    expect(screen.queryByRole('button', { name: 'Cancel scan' })).toBeNull();
    expect(screen.getByRole('link', { name: 'Scan report' })).toBeInTheDocument();
  });
  it('berjalan: tombol batal memanggil endpoint, tanpa link laporan, "masih berjalan"', async () => {
    const fetch = route((u) => u.pathname === '/api/v1/scans/s1' ? scan({ id: 's1', status: 'running', ended_at: null, jobs: [job({ status: 'running' })] }) : u.pathname.endsWith('/cancel') ? {} : undefined);
    mount(<ScanDetailPage />, '/simulations/s1', '/simulations/:id');
    await userEvent.click(await screen.findByRole('button', { name: 'Cancel scan' }));
    await waitFor(() => expect(fetch.mock.calls.some(([u, i]) => String(u).endsWith('/scans/s1/cancel') && i?.method === 'POST')).toBe(true));
    expect(screen.getByText('still running')).toBeInTheDocument(); // tanpa ekspresi debug (AUDIT UI-015)
    expect(screen.queryByRole('link', { name: 'Scan report' })).toBeNull();
  });
  it('gagal: pesan job.error terlihat di kolom catatan', async () => {
    route((u) => u.pathname === '/api/v1/scans/s1' ? scan({ id: 's1', status: 'failed', jobs: [job({ status: 'failed', error: 'Endpoint unreachable' })] }) : undefined);
    mount(<ScanDetailPage />, '/simulations/s1', '/simulations/:id');
    expect(await screen.findByText('Endpoint unreachable')).toBeInTheDocument();
    expect(screen.getAllByText('Failed').length).toBeGreaterThan(0);
  });
});

describe('DefenseBot scan', () => {
  const summary = { schema: 1, disposition: 'limited', software_checked: 120, software_unmapped: 3, findings: 4, omitted: 0, by_severity: { high: 2 }, known_exploited: 1, exposed: 2, reboot_required: 0, hardening: 0, notes: [{ code: 'source_down', limiting: true }, { code: 'apps_unmapped', count: 3, limiting: true }], provenance: { engine: 'defensebot/1', queried_at: '2026-11-08T02:00:00Z' } };
  it('names the tool, summarises the run in the job row and shows counts with the coverage notes', async () => {
    route((u) => u.pathname.endsWith('/scans/aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee') ? scan({ options: { tools: ['defensebot'], preset: 'defense', asset_ids: ['a1'], range_ids: [], organization_id: 'o1' }, status: 'incomplete', jobs: [job({ tool: 'defensebot', status: 'incomplete', target: 'local', defense: summary })] }) : undefined);
    mount(<ScanDetailPage />, '/simulations/aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee', '/simulations/:id');
    expect(await screen.findByRole('heading', { name: 'DefenseBot result' })).toBeInTheDocument();
    expect(screen.getAllByText('DefenseBot').length).toBeGreaterThan(0);
    expect(screen.getByText('4 advisories · 1 known exploited')).toBeInTheDocument();
    expect(screen.getByText(/Incomplete result/)).toBeInTheDocument();
    expect(screen.getByText('The vulnerability service could not be reached; no result is shown as clean.')).toBeInTheDocument();
    expect(screen.getByText('3 applications could not be mapped to vulnerability data.')).toBeInTheDocument();
  });
});

describe('Engine names follow what actually ran', () => {
  it('lists a DefenseBot-only scan as DefenseBot and a web scan as Hackbot, never the other way round', async () => {
    route((u) => u.pathname === '/api/v1/scans' ? { items: [
      scan({ id: 'aaaaaaaa-0000-0000-0000-000000000001', options: { tools: ['defensebot'], preset: 'defense', asset_ids: ['a1'], range_ids: [], organization_id: 'o1' } }),
      scan({ id: 'aaaaaaaa-0000-0000-0000-000000000002', options: { tools: ['nmap', 'nuclei', 'hackbot'], preset: 'standard', asset_ids: ['a1', 'a2'], range_ids: [], organization_id: 'o1' } }),
      scan({ id: 'aaaaaaaa-0000-0000-0000-000000000003', options: { tools: ['nmap', 'nuclei', 'hackbot', 'defensebot'], preset: 'standard', asset_ids: ['a1'], range_ids: [], organization_id: 'o1' } }),
    ], next_cursor: null } : undefined);
    mount(<ScanList />, '/simulations');
    const table = await screen.findByRole('table', { name: 'Scans' });
    const rows = await within(table).findAllByRole('row');
    const text = rows.slice(1).map((r) => r.textContent ?? '');
    expect(text[0]).toContain('DefenseBot');
    expect(text[0]).not.toContain('Hackbot');
    expect(text[1]).toContain('Hackbot');
    expect(text[1]).not.toContain('DefenseBot');
    expect(text[2]).toContain('Hackbot + DefenseBot');
    // Internal words never leak into the list: no preset names and no tool names.
    for (const t of text) expect(t).not.toMatch(/standard|defense\b|nmap|nuclei|preset/i);
    expect(text[1]).toContain('2 assets');
  });
});

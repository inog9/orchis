import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ReportDetailPage, ReportsPage } from './ReportsPage';

const perms = ['organization.read', 'report.read', 'report.generate', 'assessment.read', 'simulation.read'];
const json = (v: unknown, status = 200) => new Response(JSON.stringify(v), { status });
const report = (o: Record<string, unknown> = {}) => ({ id: 'aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee', organization_id: 'o1', assessment_id: 'a', status: 'ready', error: '', version: 2, created_at: '2026-01-01T00:00:00Z', snapshot: { organization: 'Org Uno', profile: 'standard', risk: 'Low exposure', scan_id: 's1', by_severity: {}, comparison: 'c', remediation: 'r', assets: [] }, ...o });
function route(handler: (u: URL, init?: RequestInit) => unknown | undefined) {
  return vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
    const u = new URL(String(input), 'http://x');
    if (u.pathname.endsWith('/auth/me')) return json({ id: 'u', permissions: perms });
    if (u.pathname === '/api/v1/organizations') return json({ items: [], next_cursor: null });
    const r = handler(u, init);
    return r === undefined ? json({ items: [], next_cursor: null }) : r instanceof Response ? r : json(r);
  });
}
const mount = (ui: React.ReactNode, path: string, pattern = '*') => render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><MemoryRouter initialEntries={[path]}><Routes><Route path={pattern} element={ui} /></Routes></MemoryRouter></QueryClientProvider>);
afterEach(() => vi.restoreAllMocks());

describe('ReportsPage', () => {
  it('tabel: status PDF bertulisan, aksi PDF hanya saat siap', async () => {
    route((u) => u.pathname === '/api/v1/reports' ? { items: [report(), report({ id: 'bbbbbbbb-bbbb-cccc-dddd-eeeeeeeeeeee', status: 'failed' }), report({ id: 'cccccccc-bbbb-cccc-dddd-eeeeeeeeeeee', status: 'generating' })], next_cursor: null } : undefined);
    mount(<ReportsPage />, '/reports');
    const table = await screen.findByRole('table', { name: 'Reports' });
    expect(within(table).getByText('Ready')).toBeInTheDocument();
    expect(within(table).getByText('Failed')).toBeInTheDocument();
    expect(within(table).getByText('Generating')).toBeInTheDocument();
    expect(within(table).getAllByRole('link', { name: 'PDF' })).toHaveLength(1);
    expect(within(table).getAllByText('Low exposure')).toHaveLength(3);
  });
  it('kosong, tanpa hasil dengan filter, dan galat dengan coba lagi', async () => {
    route(() => ({ items: [], next_cursor: null }));
    const { unmount } = mount(<ReportsPage />, '/reports');
    expect(await screen.findByText('No reports yet')).toBeInTheDocument();
    unmount();
    mount(<ReportsPage />, '/reports?org=o1');
    expect(await screen.findByText('No results')).toBeInTheDocument();
  });
  it('galat memuat memberi role=alert dan tombol Try again', async () => {
    let n = 0;
    route((u) => u.pathname === '/api/v1/reports' ? (++n === 1 ? json({ title: 't', status: 500, error_id: 'e' }, 500) : { items: [report()], next_cursor: null }) : undefined);
    mount(<ReportsPage />, '/reports');
    expect(await screen.findByRole('alert')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Try again' }));
    expect(await screen.findByRole('table', { name: 'Reports' })).toBeInTheDocument();
  });
});

describe('ReportDetailPage status', () => {
  it('counts observations shown in the snapshot even when legacy counters omit ports', async () => {
    const r = report({id:'rc'});
    r.snapshot.assets = [{id:'a',name:'Host',status:'incomplete',skip_reason:'',methods:[],findings:[{id:'f',type:'open_port',title:'Open 443',value:'tcp/443',severity:'info'}]}] as never;
    route(u => u.pathname === '/api/v1/reports/rc' ? r : undefined);
    mount(<ReportDetailPage />, '/reports/rc', '/reports/:id');
    expect(await screen.findByText('1 findings recorded')).toBeInTheDocument();
    expect(screen.getByText('Complete assessment')).toBeInTheDocument();
    expect(screen.getByText('Open 443')).toBeInTheDocument();
  });
  it('incomplete snapshot prioritises completing checks instead of claiming a clean result', async () => {
    const r = report({id:'r0'});
    r.snapshot.assets = [{id:'a',name:'Host',status:'failed',skip_reason:'',methods:[],findings:[]}] as never;
    route(u => u.pathname === '/api/v1/reports/r0' ? r : undefined);
    mount(<ReportDetailPage />, '/reports/r0', '/reports/:id');
    expect(await screen.findByText('Not assessed')).toBeInTheDocument();
    expect(screen.getByText('Complete assessment')).toBeInTheDocument();
    expect(screen.getByText(/does not establish that the assets are safe/)).toBeInTheDocument();
  });

  it('failed: alert, tombol retry memanggil endpoint, PDF dinonaktifkan, tanpa pratinjau', async () => {
    const fetch = route((u) => u.pathname === '/api/v1/reports/r1' ? report({ id: 'r1', status: 'failed', error: 'renderer down' }) : u.pathname.endsWith('/retry') ? {} : undefined);
    mount(<ReportDetailPage />, '/reports/r1', '/reports/:id');
    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('renderer down');
    expect(screen.getByRole('button', { name: 'Download PDF' })).toBeDisabled(); // PDF gagal: tombol disabled dengan alasan, bukan tautan mati
    expect(screen.queryByRole('link', { name: 'Open PDF preview' })).toBeNull();
    await userEvent.click(screen.getByRole('button', { name: /retry/i }));
    await waitFor(() => expect(fetch.mock.calls.some(([u, i]) => String(u).endsWith('/reports/r1/retry') && i?.method === 'POST')).toBe(true));
  });
  it('generating status and ready PDF preview opens below the report', async () => {
    route((u) => u.pathname === '/api/v1/reports/r1' ? report({ id: 'r1', status: 'generating' }) : undefined);
    const { unmount } = mount(<ReportDetailPage />, '/reports/r1', '/reports/:id');
    expect((await screen.findAllByText('Generating')).length).toBeGreaterThan(0);
    expect(screen.queryByRole('button', { name: /retry/i })).toBeNull();
    unmount();
    vi.restoreAllMocks();
    route((u) => u.pathname === '/api/v1/reports/r2' ? report({ id: 'r2' }) : undefined);
    mount(<ReportDetailPage />, '/reports/r2', '/reports/:id');
    const preview = await screen.findByRole('button', { name: 'Open PDF preview' });
    expect(screen.queryByTitle('Pentest report')).toBeNull();
    await userEvent.click(preview);
    expect(preview).toHaveAttribute('aria-expanded', 'true');
    expect(document.querySelector('iframe')).toHaveAttribute('src', '/api/v1/reports/r2/export/pdf?preview=1');
    await userEvent.click(screen.getByRole('button', {name: 'Hide PDF preview'}));
    expect(document.querySelector('iframe')).toBeNull();
  });
});

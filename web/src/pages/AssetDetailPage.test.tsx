import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { AssetDetailPage } from './AssetDetailPage';

// Fixture permissions lengkap agar semua section halaman ikut ter-render.
const me = {
  id: 'u', email: 'a@b.c', display_name: 'Ada', role: 'admin',
  permissions: ['asset.read', 'asset.write', 'asset.delete', 'finding.read', 'assessment.run', 'simulation.read', 'simulation.run', 'agent.read', 'settings.manage', 'organization.read'],
};
const asset = {
  organization_id: 'o1', organization_name: 'Acme', id: 'a1', category: 'host', name: 'srv-01',
  description: 'Primary build host', platform: 'linux', arch: 'x86_64', criticality: 'high',
  is_internet_facing: true, testing_authorized: true, source: 'agent', agent_state: 'online',
  attributes: { os_version: 'Ubuntu 25.10', role: 'CI runner' },
  addresses: [{ kind: 'ip', value: '10.0.0.5', is_primary: true }],
  tags: [{ id: 't1', name: 'prod', color: '#0a7' }],
  groups: [{ id: 'g1', name: 'All endpoints', is_system: true }],
  created_at: '2026-11-01T10:00:00Z', updated_at: '2026-11-08T10:00:00Z', version: 3,
};
const endpointItem = (over: object) => ({
  key: 'k', title: 'T', status: 'never', reason: '', collected_at: null, last_attempt_at: null,
  osquery_version: '', attributes: {}, rows: [], truncated: false, enabled: true, synthetic: false, ...over,
});
const endpointData = {
  asset_id: 'a1', version: 1, read_only: false, organization_id: 'o1', has_agent: true,
  items: [
    endpointItem({ key: 'osquery.system_info', title: 'System inventory', status: 'ok', collected_at: '2026-11-08T17:45:48Z' }),
    endpointItem({ key: 'osquery.ports', title: 'Listening ports' }),
  ],
  findings: [],
};
const finding = (n: number, severity = 'low') => ({
  id: `f${n}`, asset_id: 'a1', asset_name: 'srv-01', type: 'vulnerability', value: `cve-2026-${1000 + n}`,
  title: `Finding number ${n}`, severity, details: { query: 'osquery.hardening' },
  first_seen_at: '2026-11-08T10:00:00Z', last_seen_at: '2026-11-08T10:00:00Z', occurrence_count: 1,
});
const defaultFindings = [finding(1, 'medium'), finding(2, 'info')];

const json = (v: unknown, status = 200) => new Response(JSON.stringify(v), { status });

// Handler boleh mengoverride URL tertentu; sisanya memakai jawaban bawaan fixture.
function route(handler: (url: string) => Response | undefined = () => undefined, user = me) {
  return vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
    const url = String(input);
    const custom = handler(url);
    if (custom) return custom;
    if (url.includes('/auth/me')) return json(user);
    if (url.includes('/assets/a1/endpoint-data')) return json(endpointData);
    if (url.includes('/assets/a1')) return json(asset);
    if (url.includes('/findings')) return json({ items: defaultFindings, next_cursor: null });
    if (url.includes('/agents')) return json({ items: [{ id: 'ag1', asset_id: 'a1', last_heartbeat_at: new Date().toISOString() }] });
    if (url.includes('/scans')) return json({ items: [], next_cursor: null });
    if (url.includes('/readiness')) return json({ total: 1, authorized: 1, ready: 1, scan_window_open: true, emergency_stop: false, items: [{ id: 'a1', name: 'srv-01', authorized: true, reason: '', web_reason: '', defense_reason: '', agent_status: 'online' }] });
    return json({});
  });
}

function mount() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={['/assets/a1']}>
        <Routes>
          <Route path="/assets/:id" element={<AssetDetailPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

afterEach(() => vi.restoreAllMocks());

describe('AssetDetailPage', () => {
  it('menampilkan heading, strip metrik, dan panel detail dengan data nyata', async () => {
    route();
    mount();
    expect(await screen.findByRole('heading', { level: 1, name: 'srv-01' })).toBeInTheDocument();
    expect(screen.getByText('Asset · Host')).toBeInTheDocument();
    // strip metrik: kritikalitas, agent, internet-facing, platform
    expect(screen.getByText('Criticality')).toBeInTheDocument();
    expect(screen.getByText('High')).toBeInTheDocument();
    expect(screen.getAllByText('Yes').length).toBeGreaterThan(0); // internet-facing + testing authorized
    expect(screen.getByText('Linux x86_64')).toBeInTheDocument();
    // detail: sumber, arsitektur, atribut kategori, grup, tag, alamat
    expect(screen.getByText('CI runner')).toBeInTheDocument();
    expect(screen.getByText('Primary build host')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /All endpoints/ })).toHaveAttribute('href', '/assets/groups/g1');
    expect(screen.getAllByText('prod').length).toBeGreaterThan(0); // chip tag di kepala + panel detail
    expect(screen.getByText('10.0.0.5')).toBeInTheDocument();
    // aksi header
    expect(screen.getByRole('link', { name: 'View organisation' })).toHaveAttribute('href', '/organizations/o1');
    expect(screen.getByRole('link', { name: 'Run scan' })).toHaveAttribute('href', '/simulations/new?org=o1');
  });

  it('tabel pengumpulan data endpoint menampilkan status per-check dan panel orchis-defense-agent tetap ada', async () => {
    route();
    mount();
    expect(await screen.findByText('System inventory')).toBeInTheDocument();
    expect(screen.getByText('osquery.system_info')).toBeInTheDocument();
    expect(screen.getByText('Collected')).toBeInTheDocument();
    expect(screen.getByText('Listening ports')).toBeInTheDocument();
    expect(screen.getByText('Never')).toBeInTheDocument();
    expect(screen.getByText('—')).toBeInTheDocument(); // belum pernah dikumpulkan
    // panel lama tidak hilang: orchis-defense-agent + hapus data endpoint
    expect(screen.getByRole('heading', { name: 'orchis-defense-agent' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Delete collected data' })).toBeInTheDocument();
  });

  it('tabel temuan: severity, nilai, query, dan counter untuk subset', async () => {
    const many = Array.from({ length: 12 }, (_, i) => finding(i + 1, i < 2 ? 'critical' : 'low'));
    route((url) => (url.includes('/findings') ? json({ items: many, next_cursor: null }) : undefined));
    mount();
    expect((await screen.findAllByText(/^Finding number /)).length).toBe(10);
    expect(screen.getAllByText('Critical')).toHaveLength(2);
    expect(screen.getAllByText('osquery.hardening')).toHaveLength(10);
    expect(screen.getByText('+2 more findings')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /View all findings for this asset/ })).toHaveAttribute('href', '/findings?asset_id=a1');
  });

  it('endpoint data kosong menampilkan keadaan kosong dengan penjelasan', async () => {
    route((url) => (url.includes('/endpoint-data') ? json({ ...endpointData, items: [] }) : undefined));
    mount();
    expect(await screen.findByText('No endpoint data collected yet')).toBeInTheDocument();
    expect(screen.getByText(/Checks appear here once an agent is enrolled/)).toBeInTheDocument();
  });

  it('aset tidak ditemukan menampilkan pesan 404', async () => {
    route((url) => (url.includes('/auth/me') ? undefined : json({ title: 'Not Found', status: 404 }, 404)));
    mount();
    expect(await screen.findByRole('alert')).toHaveTextContent('Asset not found');
  });

  it('tanpa izin baca temuan: section tetap tampil dengan catatan akses dan tidak memanggil /findings', async () => {
    const limited = { ...me, permissions: me.permissions.filter((p) => p !== 'finding.read') };
    const spy = route(undefined, limited);
    mount();
    expect(await screen.findByRole('heading', { level: 1, name: 'srv-01' })).toBeInTheDocument();
    expect(await screen.findByText(/Your role cannot read findings/)).toBeInTheDocument();
    expect(spy.mock.calls.some(([u]) => String(u).includes('/findings'))).toBe(false);
  });
});

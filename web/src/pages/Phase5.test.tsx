const schemas: Record<string, unknown> = {"nmap.tcp_connect":{"type":"object","additionalProperties":false,"properties":{"profile":{"type":"string","label":"Ports","enum":["top100","top1000","web","custom"],"enum_labels":["Top 100 ports","Top 1000 ports","Web ports (80, 443, 8000, 8080, 8443)","Custom list"],"default":"top100"},"ports":{"type":"string","label":"Port list","pattern":"^[0-9]{1,5}(-[0-9]{1,5})?(,[0-9]{1,5}(-[0-9]{1,5})?){0,49}$","max_length":600,"help":"Ports and ranges separated by commas.","show_when":{"field":"profile","value":"custom"}},"service_detection":{"type":"boolean","label":"Detect services and versions","default":false,"help":"Slower, and sends probes to every open port."}},"order":["profile","ports","service_detection"]},"nuclei.scan":{"type":"object","additionalProperties":false,"properties":{"severity":{"type":"array","label":"Severity to report","items_enum":["info","low","medium","high","critical"],"min_items":1,"max_items":5,"default":["medium","high","critical"]},"tags":{"type":"array","label":"Template tags (optional)","items_pattern":"^[a-z0-9][a-z0-9_-]{0,31}$","max_items":10,"default":[],"help":"Limit the scan to templates with these tags."}},"order":["severity","tags"]},"agent.listening_ports":{"type":"object","properties":{},"order":[]},"agent.system_info":{"type":"object","properties":{},"order":[]}};
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ChainWorkspacePage } from './ChainWorkspacePage';
import { ScenariosPage } from './ScenariosPage';
import { AttackPathView, SimulationDetailPage } from './SimulationDetailPage';
import { SimulationsPage } from './SimulationsPage';

const perms = ['scenario.read', 'scenario.write', 'simulation.read', 'simulation.run', 'atomic.read', 'finding.read'];
const analyst = { id: 'u', email: 'a@b.c', display_name: 'Ada', role: 'analyst', permissions: perms };
const viewer = { ...analyst, role: 'viewer', permissions: ['scenario.read', 'simulation.read', 'atomic.read', 'finding.read'] };
const json = (v: unknown, status = 200) => new Response(JSON.stringify(v), { status });

const step = (key: string, extra = {}) => ({ key, title: `Step ${key}`, description: '', contract_key: 'nmap.tcp_connect', params: { profile: 'top100', service_detection: false }, delay_seconds: 0, trigger_mode: 'after_parents', trigger: null, is_enabled: true, ...extra });
const scenario = (extra = {}) => ({ id: 's1', name: 'Kill chain', description: null, severity: 'high', category: null, status: 'ready', step_count: 2, edge_count: 1, target_count: 1, simulations: 0, created_at: '2026-10-29T10:00:00Z', updated_at: '2026-10-29T10:00:00Z', version: 3, targets: [], steps: [step('a'), step('b')], edges: [{ parent: 'a', child: 'b' }], problems: [], ...extra });
const contracts = { items: [{ key: 'agent.listening_ports', input_schema: schemas['agent.listening_ports'], name: 'Listening ports (agent)', description: '', kind: 'scan', output_types: [], timeout_seconds: 60, integration: 'agent', integration_status: 'online', last_heartbeat_at: null, needs_agent: true },
  { key: 'nmap.tcp_connect', input_schema: schemas['nmap.tcp_connect'], name: 'Nmap TCP connect scan', description: '', kind: 'scan', output_types: [], timeout_seconds: 600, integration: 'nmap', integration_status: 'online', last_heartbeat_at: null },
  { key: 'nuclei.scan', input_schema: schemas['nuclei.scan'], name: 'Nuclei vulnerability scan', description: '', kind: 'scan', output_types: [], timeout_seconds: 900, integration: 'nuclei', integration_status: 'online', last_heartbeat_at: null }] };
const sim = (extra = {}) => ({ id: 'm1', number: 1, code: 'OP-0001', name: 'Run one', status: 'running', scenario_id: 's1', scenario_name: 'Kill chain', safe_mode: true, timeout_seconds: 3600, cancel_reason: null,
  started_at: '2026-10-29T10:00:00Z', ended_at: null, created_at: '2026-10-29T10:00:00Z', target_count: 1, step_count: 2, steps_done: 1, finding_count: 3, ...extra });
const ex = (o = {}) => ({ total: 1, running: 0, queued: 0, success: 1, failed: 0, canceled: 0, ...o });
const simDetail = (extra = {}) => ({ ...sim(), steps: [
  { key: 'a', title: 'Port scan', contract_key: 'nmap.tcp_connect', contract_name: 'Nmap TCP connect scan', status: 'done', trigger_mode: 'after_parents', trigger: null, delay_seconds: 0, is_enabled: true, executions: ex(), finding_count: 2 },
  { key: 'b', title: 'Web exposure', contract_key: 'nuclei.scan', contract_name: 'Nuclei vulnerability scan', status: 'skipped', trigger_mode: 'on_finding', trigger: { type: 'open_port', values: ['tcp/80'] }, delay_seconds: 5, is_enabled: true, executions: ex({ total: 0, success: 0 }), finding_count: 0 }],
  edges: [{ parent: 'a', child: 'b' }], targets: [{ id: 'x', name: 'srv' }], ...extra });

function route(user: object, handler: (url: string, init?: RequestInit) => Response | undefined) {
  return vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
    const url = String(input);
    if (url.includes('/auth/me')) return json(user);
    if (url.endsWith('/contracts')) return json(contracts);
    return handler(url, init) ?? json({}, 404);
  });
}
function mount(ui: React.ReactNode, path: string, routePath = '*') {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}><MemoryRouter initialEntries={[path]}><Routes><Route path={routePath} element={ui} /></Routes></MemoryRouter></QueryClientProvider>);
}
afterEach(() => vi.restoreAllMocks());

describe('ScenariosPage', () => {
  it('menampilkan scenario dengan status Ready/Draft dan jumlah aksi', async () => {
    route(analyst, (u) => (u.endsWith('/scenarios') ? json({ items: [scenario(), scenario({ id: 's2', name: 'Draft one', status: 'draft', step_count: 0, target_count: 0 })] }) : undefined));
    mount(<ScenariosPage />, '/scenarios');
    expect(await screen.findByRole('link', { name: 'Kill chain' })).toHaveAttribute('href', '/scenarios/s1');
    expect(screen.getByText('Ready')).toBeInTheDocument();
    expect(screen.getByText('Draft')).toBeInTheDocument();
    expect(screen.getByText(/2 actions · 1 target/)).toBeInTheDocument();
  });
  it('keadaan kosong, dan viewer tidak melihat tombol membuat', async () => {
    route(viewer, (u) => (u.endsWith('/scenarios') ? json({ items: [] }) : undefined));
    mount(<ScenariosPage />, '/scenarios');
    expect(await screen.findByText('No scenarios yet')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /new scenario|create the first/i })).not.toBeInTheDocument();
  });
});

describe('ChainWorkspacePage', () => {
  const load = (extra = {}) => (u: string, init?: RequestInit) => {
    if (u.endsWith('/scenarios/s1') && (!init?.method || init.method === 'GET')) return json(scenario(extra));
  };
  it('menandai perubahan, mengirim graf dengan versi yang dimuat, dan mencegah siklus', async () => {
    const fetchMock = route(analyst, (u, init) => {
      if (u.endsWith('/graph') && init?.method === 'PUT') return json(scenario({ version: 4 }));
      return load()(u);
    });
    const user = userEvent.setup();
    mount(<ChainWorkspacePage />, '/chains/s1', '/chains/:id');
    await user.click(await screen.findByRole('button', { name: 'Action Step b' }));
    // a adalah induk b, jadi b tidak boleh menjadi induk a (siklus)
    await user.click(screen.getByRole('button', { name: 'Action Step a' }));
    const loop = screen.getByLabelText(/^Step b/);
    expect(loop).toBeDisabled();
    expect(screen.getByText('Would create a loop')).toBeInTheDocument();
    expect(screen.queryByText('Unsaved changes')).not.toBeInTheDocument();
    await user.clear(screen.getByLabelText('Title'));
    await user.type(screen.getByLabelText('Title'), 'Renamed');
    expect(screen.getByText('Unsaved changes')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Save chain' }));
    await waitFor(() => expect(screen.queryByText('Unsaved changes')).not.toBeInTheDocument());
    const put = fetchMock.mock.calls.find(([, i]) => i?.method === 'PUT')!;
    const body = JSON.parse(String(put[1]!.body));
    expect(body.version).toBe(3);
    expect(body.steps[0].title).toBe('Renamed');
    expect(body.edges).toEqual([{ parent: 'a', child: 'b' }]);
  });
  it('menampilkan masalah validasi dari server tanpa kehilangan suntingan', async () => {
    route(analyst, (u, init) => (u.endsWith('/graph') && init?.method === 'PUT'
      ? json({ title: 'Validation failed', status: 422, detail: 'x', fields: [{ name: 'edges', message: 'Dependencies form a loop: a → b.' }] }, 422) : load()(u)));
    const user = userEvent.setup();
    mount(<ChainWorkspacePage />, '/chains/s1', '/chains/:id');
    await user.click(await screen.findByRole('button', { name: 'Action Step a' }));
    await user.clear(screen.getByLabelText('Title'));
    await user.type(screen.getByLabelText('Title'), 'Edited');
    await user.click(screen.getByRole('button', { name: 'Save chain' }));
    expect(await screen.findByText('Fix these before saving')).toBeInTheDocument();
    expect(screen.getByText(/form a loop/)).toBeInTheDocument();
    expect(screen.getByLabelText('Title')).toHaveValue('Edited');
  });
  it('bentrok versi (409) memberi tahu bahwa orang lain mengubah chain', async () => {
    route(analyst, (u, init) => (u.endsWith('/graph') && init?.method === 'PUT' ? json({ title: 'Conflict', status: 409, detail: 'changed by someone else' }, 409) : load()(u)));
    const user = userEvent.setup();
    mount(<ChainWorkspacePage />, '/chains/s1', '/chains/:id');
    await user.click(await screen.findByRole('button', { name: 'Action Step a' }));
    await user.clear(screen.getByLabelText('Title'));
    await user.type(screen.getByLabelText('Title'), 'X');
    await user.click(screen.getByRole('button', { name: 'Save chain' }));
    expect(await screen.findByText('Someone else changed this chain')).toBeInTheDocument();
  });
  it('viewer hanya membaca: tidak ada tombol sunting dan kolom nonaktif', async () => {
    route(viewer, load());
    const user = userEvent.setup();
    mount(<ChainWorkspacePage />, '/chains/s1', '/chains/:id');
    await user.click(await screen.findByRole('button', { name: 'Action Step a' }));
    expect(screen.queryByRole('button', { name: /save chain|add action/i })).not.toBeInTheDocument();
    expect(screen.getByLabelText('Title')).toBeDisabled();
  });
  it('aksi baru memakai Nmap sebagai bawaan, bukan aksi pertama menurut abjad (aksi agent)', async () => {
    const f = route(analyst, (u, init) => (u.endsWith('/graph') && init?.method === 'PUT' ? json(scenario({ version: 4 })) : load({ steps: [], edges: [], step_count: 0 })(u, init)));
    const user = userEvent.setup();
    mount(<ChainWorkspacePage />, '/chains/s1', '/chains/:id');
    await user.click(await screen.findByRole('button', { name: 'Add the first action' }));
    expect(screen.getByLabelText('Action')).toHaveValue('nmap.tcp_connect');
    await user.click(screen.getByRole('button', { name: 'Save chain' }));
    await waitFor(() => expect(f.mock.calls.some(([, i]) => i?.method === 'PUT')).toBe(true));
    expect(JSON.parse(String(f.mock.calls.find(([, i]) => i?.method === 'PUT')![1]!.body)).steps[0].contract_key).toBe('nmap.tcp_connect');
  });

  it('chain kosong mengajak menambah aksi pertama', async () => {
    route(analyst, load({ steps: [], edges: [], step_count: 0 }));
    mount(<ChainWorkspacePage />, '/chains/s1', '/chains/:id');
    expect(await screen.findByText('This chain is empty')).toBeInTheDocument();
  });
});

describe('Simulations', () => {
  it('daftar menampilkan progres langkah dan temuan, dan filter mengirim status', async () => {
    const urls: string[] = [];
    route(analyst, (u) => { urls.push(u); if (u.includes('/simulations')) return json({ items: [sim(), sim({ id: 'm2', code: 'OP-0002', name: 'Run two', status: 'finished', steps_done: 2 })] }); });
    const user = userEvent.setup();
    mount(<SimulationsPage />, '/simulations');
    expect(await screen.findByText('Run one')).toBeInTheDocument();
    expect(screen.getByText('1 of 2 steps')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Finished' }));
    await waitFor(() => expect(urls.some((u) => u.includes('status=finished'))).toBe(true));
  });
  it('detail: status tiap langkah, aturan trigger, dan tombol sesuai status dan izin', async () => {
    route(analyst, (u) => (u.includes('/simulations/m1/attack-path') ? json({ nodes: [], edges: [], truncated: false }) : u.endsWith('/simulations/m1') ? json(simDetail()) : undefined));
    mount(<SimulationDetailPage />, '/simulations/m1', '/simulations/:id');
    expect(await screen.findByRole('heading', { name: 'Run one' })).toBeInTheDocument();
    const rows = screen.getByRole('table', { name: 'Steps' });
    expect(within(rows).getByText('Done')).toBeInTheDocument();
    expect(within(rows).getByText('Skipped')).toBeInTheDocument();
    expect(within(rows).getByText(/On open port tcp\/80 · \+5s/)).toBeInTheDocument();
    expect(screen.getByRole('progressbar', { name: 'Simulation progress' })).toHaveAttribute('aria-valuenow', '50');
    expect(screen.getByRole('button', { name: /pause/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /stop/i })).toBeInTheDocument();
  });
  it('simulasi yang dijeda menawarkan Resume; yang selesai tanpa kontrol; viewer tanpa kontrol', async () => {
    route(analyst, (u) => (u.endsWith('/simulations/m1') ? json(simDetail({ status: 'paused' })) : undefined));
    const { unmount } = mount(<SimulationDetailPage />, '/simulations/m1', '/simulations/:id');
    expect(await screen.findByRole('button', { name: /resume/i })).toBeInTheDocument();
    unmount(); vi.restoreAllMocks();
    route(analyst, (u) => (u.endsWith('/simulations/m1') ? json(simDetail({ status: 'finished', ended_at: '2026-10-29T10:05:00Z' })) : undefined));
    const second = mount(<SimulationDetailPage />, '/simulations/m1', '/simulations/:id');
    await screen.findByRole('heading', { name: 'Run one' });
    expect(screen.queryByRole('button', { name: /pause|resume|stop/i })).not.toBeInTheDocument();
    second.unmount(); vi.restoreAllMocks();
    route(viewer, (u) => (u.endsWith('/simulations/m1') ? json(simDetail()) : undefined));
    mount(<SimulationDetailPage />, '/simulations/m1', '/simulations/:id');
    await screen.findByRole('heading', { name: 'Run one' });
    expect(screen.queryByRole('button', { name: /pause|stop/i })).not.toBeInTheDocument();
  });
  it('gagal memuat menampilkan galat', async () => {
    route(analyst, (u) => (u.endsWith('/simulations/m1') ? json({ title: 'x', status: 500, error_id: 'err_1' }, 500) : undefined));
    mount(<SimulationDetailPage />, '/simulations/m1', '/simulations/:id');
    expect(await screen.findByText('Couldn’t load this simulation')).toBeInTheDocument();
  });
});

describe('AttackPathView', () => {
  it('menggambar simpul per kolom dan menyebut jumlahnya secara aksesibel', () => {
    render(<AttackPathView path={{ truncated: false, nodes: [
      { id: 'step:a', kind: 'step', label: 'Port scan', sub: 'Nmap', status: 'done', column: 0 },
      { id: 'asset:1', kind: 'asset', label: 'srv', column: 1 },
      { id: 'finding:1', kind: 'finding', label: 'Open port 80', sub: 'open_port', severity: 'info', column: 2 },
      { id: 'step:b', kind: 'step', label: 'Web exposure', sub: 'Nuclei', status: 'done', column: 3 }],
      edges: [{ from: 'step:a', to: 'asset:1', kind: 'targets' }, { from: 'asset:1', to: 'finding:1', kind: 'found' }, { from: 'finding:1', to: 'step:b', kind: 'triggers' }] }} />);
    expect(screen.getByRole('group', { name: 'Attack path with 4 nodes and 3 connections' })).toBeInTheDocument();
    for (const t of ['Port scan', 'srv', 'Open port 80', 'Web exposure']) expect(screen.getByText(t)).toBeInTheDocument();
  });
  it('graf kosong tidak error', () => {
    render(<AttackPathView path={{ nodes: [], edges: [], truncated: false }} />);
    expect(screen.getByText('Nothing to draw yet.')).toBeInTheDocument();
  });
});

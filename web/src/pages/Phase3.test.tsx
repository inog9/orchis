const schemas: Record<string, unknown> = {"nmap.tcp_connect":{"type":"object","additionalProperties":false,"properties":{"profile":{"type":"string","label":"Ports","enum":["top100","top1000","web","custom"],"enum_labels":["Top 100 ports","Top 1000 ports","Web ports (80, 443, 8000, 8080, 8443)","Custom list"],"default":"top100"},"ports":{"type":"string","label":"Port list","pattern":"^[0-9]{1,5}(-[0-9]{1,5})?(,[0-9]{1,5}(-[0-9]{1,5})?){0,49}$","max_length":600,"help":"Ports and ranges separated by commas.","show_when":{"field":"profile","value":"custom"}},"service_detection":{"type":"boolean","label":"Detect services and versions","default":false,"help":"Slower, and sends probes to every open port."}},"order":["profile","ports","service_detection"]},"nuclei.scan":{"type":"object","additionalProperties":false,"properties":{"severity":{"type":"array","label":"Severity to report","items_enum":["info","low","medium","high","critical"],"min_items":1,"max_items":5,"default":["medium","high","critical"]},"tags":{"type":"array","label":"Template tags (optional)","items_pattern":"^[a-z0-9][a-z0-9_-]{0,31}$","max_items":10,"default":[],"help":"Limit the scan to templates with these tags."}},"order":["severity","tags"]},"agent.listening_ports":{"type":"object","properties":{},"order":[]},"agent.system_info":{"type":"object","properties":{},"order":[]}};
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { AtomicTestDetailPage } from './AtomicTestDetailPage';
import { AtomicTestsPage } from './AtomicTestsPage';
import { FindingsPage } from './FindingsPage';

const analyst = { id: 'u', email: 'a@b.c', display_name: 'Ada', role: 'analyst', permissions: ['organization.read', 'atomic.read', 'atomic.write', 'atomic.run', 'finding.read', 'asset.read'] };
const viewer = { ...analyst, role: 'viewer', permissions: ['atomic.read', 'finding.read'] };
const json = (v: unknown, status = 200) => new Response(JSON.stringify(v), { status });

const contract = (key: string, name: string, integration: string, status = 'online') => ({
  key, input_schema: schemas[key], name, description: `${name} description`, kind: 'scan', output_types: ['open_port'], timeout_seconds: 600, integration, integration_status: status, last_heartbeat_at: null,
});
const run = (extra = {}) => ({ id: 'r1', state: 'ended', outcome: 'success', error: null, created_at: '2026-10-29T10:00:00Z', started_at: '2026-10-29T10:00:01Z', ended_at: '2026-10-29T10:00:05Z', finding_count: 1, ...extra });
const test = (extra = {}) => ({
  id: 't1', name: 'Web top ports', contract_key: 'nmap.tcp_connect', contract_name: 'Nmap TCP connect scan', params: { profile: 'top100', service_detection: false },
  target_asset_id: 'a1', target_asset_name: 'srv-web', created_at: '2026-10-29T09:00:00Z', updated_at: '2026-10-29T09:00:00Z', version: 1, last_execution: null, ...extra,
});
const finding = (extra = {}) => ({
  id: 'f1', asset_id: 'a1', asset_name: 'srv-web', type: 'open_port', value: 'tcp/22', title: 'Open port 22/tcp (ssh)', severity: 'info', details: {},
  first_seen_at: '2026-10-29T10:00:05Z', last_seen_at: '2026-10-29T10:00:05Z', occurrence_count: 2, ...extra,
});

function route(user: object, handler: (url: string, init?: RequestInit) => Response | undefined) {
  return vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
    const url = String(input);
    if (url.includes('/auth/me')) return json(user);
    return handler(url, init) ?? json({}, 404);
  });
}

function mount(ui: React.ReactNode, path: string, routePath = '*') {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[path]}><Routes><Route path={routePath} element={ui} /></Routes></MemoryRouter>
    </QueryClientProvider>,
  );
}

afterEach(() => vi.restoreAllMocks());

describe('AtomicTestsPage', () => {
  it('menampilkan test beserta status run terakhir', async () => {
    route(analyst, (u) => {
      if (u.endsWith('/atomic-tests')) return json({ items: [test({ last_execution: run() }), test({ id: 't2', name: 'Never ran' })] });
      if (u.endsWith('/contracts')) return json({ items: [contract('nmap.tcp_connect', 'Nmap TCP connect scan', 'nmap')] });
    });
    mount(<AtomicTestsPage />, '/atomic');
    expect(await screen.findByText('Web top ports')).toBeInTheDocument();
    expect(screen.getByText('Succeeded')).toBeInTheDocument();
    expect(screen.getByText('Never run')).toBeInTheDocument();
    expect(screen.getByText('Not run yet')).toBeInTheDocument();
  });

  it('memperingatkan bila injector belum terhubung', async () => {
    route(analyst, (u) => {
      if (u.endsWith('/atomic-tests')) return json({ items: [] });
      if (u.endsWith('/contracts')) return json({ items: [contract('nmap.tcp_connect', 'Nmap', 'nmap', 'offline'), contract('nuclei.scan', 'Nuclei', 'nuclei', 'online')] });
    });
    mount(<AtomicTestsPage />, '/atomic');
    expect(await screen.findByRole('status')).toHaveTextContent(/nmap injector is not connected/i);
    expect(screen.getByText('No single tests yet')).toBeInTheDocument();
  });

  it('integrasi agent yang offline tidak memicu peringatan "injector belum terhubung"', async () => {
    route(analyst, (u) => {
      if (u.endsWith('/atomic-tests')) return json({ items: [] });
      if (u.endsWith('/contracts')) return json({ items: [contract('nmap.tcp_connect', 'Nmap', 'nmap', 'online'), { ...contract('agent.system_info', 'System info', 'agent', 'offline'), needs_agent: true }] });
    });
    mount(<AtomicTestsPage />, '/atomic');
    expect(await screen.findByText('No single tests yet')).toBeInTheDocument();
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
  });

  it('viewer tidak melihat tombol membuat test', async () => {
    route(viewer, (u) => (u.endsWith('/atomic-tests') ? json({ items: [] }) : u.endsWith('/contracts') ? json({ items: [] }) : undefined));
    mount(<AtomicTestsPage />, '/atomic');
    expect(await screen.findByText('No single tests yet')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /new single test|create the first test/i })).not.toBeInTheDocument();
  });

  it('dialog hanya menawarkan aset yang diotorisasi dan menampilkan alasan penolakan server', async () => {
    const fetchMock = route(analyst, (u, init) => {
      if (u.endsWith('/atomic-tests') && init?.method === 'POST') {
        return json({ title: 'Validation failed', status: 422, detail: 'x', fields: [{ name: 'params.ports', message: 'Use ports like 22,80,8000-8100 (at most 50 entries).', code: 'ports_invalid' }] }, 422);
      }
      if (u.endsWith('/atomic-tests')) return json({ items: [] });
      if (u.endsWith('/contracts')) return json({ items: [contract('nmap.tcp_connect', 'Nmap TCP connect scan', 'nmap'), contract('nuclei.scan', 'Nuclei vulnerability scan', 'nuclei')] });
      if (u.includes('/assets')) return json({ items: [{ id: 'a1', name: 'srv-ok', testing_authorized: true }, { id: 'a2', name: 'srv-no', testing_authorized: false }], next_cursor: null });
    });
    const user = userEvent.setup();
    mount(<AtomicTestsPage />, '/atomic');
    await user.click(await screen.findByRole('button', { name: /create the first test/i }));
    const dlg = await screen.findByRole('dialog');
    expect(await within(dlg).findByRole('option', { name: 'srv-ok' })).toBeInTheDocument();
    expect(within(dlg).queryByRole('option', { name: 'srv-no' })).not.toBeInTheDocument();
    await user.type(within(dlg).getByLabelText('Name'), 'My test');
    await user.selectOptions(within(dlg).getByLabelText('Target asset'), 'a1');
    await user.selectOptions(within(dlg).getByLabelText('Ports'), 'custom');
    await user.type(within(dlg).getByLabelText('Port list'), '22; id');
    await user.click(within(dlg).getByRole('button', { name: 'Create test' }));
    expect(await within(dlg).findByText(/use ports like 22,80/i)).toBeInTheDocument();
    const post = fetchMock.mock.calls.find(([, i]) => i?.method === 'POST');
    expect(JSON.parse(String(post![1]!.body))).toEqual({ name: 'My test', contract_key: 'nmap.tcp_connect', params: { profile: 'custom', ports: '22; id', service_detection: false }, target_asset_id: 'a1' });
  });
});

describe('AtomicTestDetailPage', () => {
  const page = (testExtra = {}, runs = [run()]) => (u: string) => {
    if (u.endsWith('/atomic-tests/t1')) return json(test({ executions: runs, last_execution: runs[0] ?? null, ...testExtra }));
    if (u.includes('/executions/r1')) {
      return json({ ...runs[0], atomic_test_id: 't1', contract_key: 'nmap.tcp_connect', target_asset_id: 'a1', target_address: '10.0.0.5', params: {}, exit_code: 0, raw_output_key: null,
        logs: [{ at: '2026-10-29T10:00:01Z', level: 'info', message: 'Started by the nmap injector.' }], findings: [finding()] });
    }
  };

  it('menampilkan hasil run: finding dan log', async () => {
    route(analyst, page());
    mount(<AtomicTestDetailPage />, '/atomic/t1', '/atomic/:id');
    expect(await screen.findByText('Open port 22/tcp (ssh)')).toBeInTheDocument();
    expect(screen.getByText('tcp/22')).toBeInTheDocument();
    expect(screen.getByText('10.0.0.5')).toBeInTheDocument();
    expect(screen.getByText(/started by the nmap injector/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /run/i })).toBeInTheDocument();
  });

  it('selagi berjalan tombol Run diganti Stop', async () => {
    route(analyst, page({}, [run({ state: 'running', outcome: null, ended_at: null, finding_count: 0 })]));
    mount(<AtomicTestDetailPage />, '/atomic/t1', '/atomic/:id');
    expect(await screen.findByRole('button', { name: /stop/i })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /^run$/i })).not.toBeInTheDocument();
  });

  it('menampilkan penolakan server saat Run (aset belum diotorisasi)', async () => {
    const base = page();
    route(analyst, (u, init) => (u.endsWith('/run') && init?.method === 'POST'
      ? json({ title: 'Validation failed', status: 422, detail: 'This asset is not authorized for testing.', code: 'testing_not_authorized' }, 422) : base(u)));
    const user = userEvent.setup();
    mount(<AtomicTestDetailPage />, '/atomic/t1', '/atomic/:id');
    await user.click(await screen.findByRole('button', { name: /^run$/i }));
    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent(/not authorized for testing/i));
  });

  it('aksi agent (tanpa pengaturan) tidak membuat halaman rusak', async () => {
    route(analyst, page({ contract_key: 'agent.listening_ports', contract_name: 'Listening ports (agent)', params: {} }));
    mount(<AtomicTestDetailPage />, '/atomic/t1', '/atomic/:id');
    expect(await screen.findByText(/runs on the endpoint, no settings/)).toBeInTheDocument();
  });

  it('viewer tidak melihat Run, Stop, maupun Delete', async () => {
    route(viewer, page());
    mount(<AtomicTestDetailPage />, '/atomic/t1', '/atomic/:id');
    await screen.findByText('Open port 22/tcp (ssh)');
    expect(screen.queryByRole('button', { name: /^run$|stop|delete/i })).not.toBeInTheDocument();
  });
});

describe('FindingsPage', () => {
  it('menampilkan finding, jumlah kemunculan, dan keadaan kosong saat disaring', async () => {
    const urls: string[] = [];
    route(analyst, (u) => {
      urls.push(u);
      if (u.includes('/findings')) return json(u.includes('q=zzz') ? { items: [], next_cursor: null, search: 'database' } : { items: [finding(), finding({ id: 'f2', severity: 'critical', title: 'Exposed .env', value: 'exposed-env@x', type: 'vulnerability' })], next_cursor: null, search: 'none' });
    });
    const user = userEvent.setup();
    mount(<FindingsPage />, '/findings');
    expect(await screen.findByText('Exposed .env')).toBeInTheDocument();
    expect(within(screen.getByRole('list', { name: 'Findings' })).getByText('Critical')).toBeInTheDocument();
    expect(screen.getAllByText(/seen 2×/i)).toHaveLength(2);
    await user.type(screen.getByLabelText('Search findings'), 'zzz');
    expect(await screen.findByText('No findings match')).toBeInTheDocument();
    expect(urls.some((u) => u.includes('q=zzz'))).toBe(true);
  });

  it('keadaan kosong mengajak membuka organisasi', async () => {
    route(analyst, (u) => (u.includes('/findings') ? json({ items: [], next_cursor: null, search: 'none' }) : undefined));
    mount(<FindingsPage />, '/findings');
    expect(await screen.findByText('No findings yet')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /open organisations/i })).toBeInTheDocument();
  });

  it('gagal memuat menampilkan error dan bisa dicoba lagi', async () => {
    let n = 0;
    route(analyst, (u) => {
      if (!u.includes('/findings')) return undefined;
      return ++n === 1 ? json({ title: 'x', status: 500, error_id: 'err_1' }, 500) : json({ items: [finding()], next_cursor: null, search: 'none' });
    });
    const user = userEvent.setup();
    mount(<FindingsPage />, '/findings');
    expect(await screen.findByText('Couldn’t load findings')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /try again/i }));
    expect(await screen.findByText('Open port 22/tcp (ssh)')).toBeInTheDocument();
  });
});

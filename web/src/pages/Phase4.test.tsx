const schemas: Record<string, unknown> = {"nmap.tcp_connect":{"type":"object","additionalProperties":false,"properties":{"profile":{"type":"string","label":"Ports","enum":["top100","top1000","web","custom"],"enum_labels":["Top 100 ports","Top 1000 ports","Web ports (80, 443, 8000, 8080, 8443)","Custom list"],"default":"top100"},"ports":{"type":"string","label":"Port list","pattern":"^[0-9]{1,5}(-[0-9]{1,5})?(,[0-9]{1,5}(-[0-9]{1,5})?){0,49}$","max_length":600,"help":"Ports and ranges separated by commas.","show_when":{"field":"profile","value":"custom"}},"service_detection":{"type":"boolean","label":"Detect services and versions","default":false,"help":"Slower, and sends probes to every open port."}},"order":["profile","ports","service_detection"]},"nuclei.scan":{"type":"object","additionalProperties":false,"properties":{"severity":{"type":"array","label":"Severity to report","items_enum":["info","low","medium","high","critical"],"min_items":1,"max_items":5,"default":["medium","high","critical"]},"tags":{"type":"array","label":"Template tags (optional)","items_pattern":"^[a-z0-9][a-z0-9_-]{0,31}$","max_items":10,"default":[],"help":"Limit the scan to templates with these tags."}},"order":["severity","tags"]},"agent.listening_ports":{"type":"object","properties":{},"order":[]},"agent.system_info":{"type":"object","properties":{},"order":[]}};
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { AgentsPage } from './AgentsPage';
import { AtomicTestDialog } from './AtomicTestDialog';

const admin = { id: 'u1', email: 'a@b.c', display_name: 'Ada', role: 'admin', permissions: ['agent.read', 'agent.manage', 'asset.read', 'atomic.read', 'atomic.write'] };
const viewer = { ...admin, role: 'viewer', permissions: ['agent.read', 'asset.read'] };
const json = (v: unknown, status = 200) => new Response(JSON.stringify(v), { status });
const problem = (status: number, extra = {}) => json({ title: 'x', status, detail: 'detail text', ...extra }, status);

const agent = (extra = {}) => ({ id: 'g1', asset_id: 'a1', asset_name: 'box1', hostname: 'box1', os: 'linux', arch: 'x86_64', mode: 'service', version: '1.0.0', status: 'online', last_heartbeat_at: '2026-10-29T10:00:00Z', enrolled_at: '2026-10-29T09:00:00Z', update_available: false, ...extra });
const token = (extra = {}) => ({ id: 'tk1', token: 'oen_SECRET', os: 'linux', arch: 'x86_64', mode: 'service', expires_at: '2026-10-29T11:00:00Z', version: '1.0.0', sha256: 'f'.repeat(64), file_name: 'agent', ...extra });

function route(user: object, handler: (url: string, init?: RequestInit) => Response | undefined) {
  return vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
    const url = String(input);
    if (url.includes('/auth/me')) return json(user);
    return handler(url, init) ?? json({}, 404);
  });
}
function mount(ui: React.ReactNode, path = '/assets/agents') {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}><MemoryRouter initialEntries={[path]}>{ui}</MemoryRouter></QueryClientProvider>);
}
afterEach(() => vi.restoreAllMocks());

describe('AgentsPage', () => {
  it('menampilkan status, versi, dan pembaruan tertunda; viewer tidak melihat tombol kelola', async () => {
    route(viewer, (u) => (u.endsWith('/agents') ? json({ items: [agent({ update_available: true }), agent({ id: 'g2', asset_name: 'old', status: 'revoked' }), agent({ id: 'g3', asset_name: 'down', status: 'offline' })] }) : undefined));
    mount(<AgentsPage />);
    const t = await screen.findByRole('table', { name: 'Agents' });
    expect(within(t).getByText('Online')).toBeInTheDocument();
    expect(within(t).getByText('Offline')).toBeInTheDocument();
    expect(within(t).getByText('Revoked')).toBeInTheDocument();
    expect(within(t).getByText(/release available/)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /install agent|revoke/i })).not.toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Agents' })).toHaveAttribute('aria-current', 'page');
  });
  it('kosong mengajak memasang agent pertama', async () => {
    route(admin, (u) => (u.endsWith('/agents') ? json({ items: [] }) : undefined));
    mount(<AgentsPage />);
    expect(await screen.findByText('No agents yet')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Install the first agent' })).toBeInTheDocument();
  });
  it('pasang: token dibuat sekali, perintah memuat checksum, lalu berubah hijau saat agent terhubung', async () => {
    let polls = 0;
    const f = route(admin, (u, init) => {
      if (u.endsWith('/agents/enrollment-tokens') && init?.method === 'POST') return json(token({ mode: 'user' }), 201);
      if (u.includes('/agents/enrollment-tokens/tk1')) { polls++; return json(polls < 2 ? { used: false, expired: false, agent_id: null, agent: null } : { used: true, expired: false, agent_id: 'g1', agent: agent() }); }
      if (u.endsWith('/agents')) return json({ items: [] });
    });
    const user = userEvent.setup();
    mount(<AgentsPage />);
    await user.click(await screen.findByRole('button', { name: 'Install the first agent' }));
    const dlg = await screen.findByRole('dialog');
    await user.selectOptions(within(dlg).getByLabelText('Run as'), 'user');
    await user.click(within(dlg).getByRole('button', { name: 'Create install command' }));
    const cmd = await within(dlg).findByTestId('install-command');
    expect(cmd.textContent).toContain('Authorization: Bearer oen_SECRET');
    expect(cmd.textContent).toContain('f'.repeat(64));
    expect(cmd.textContent).toContain('./orchis-agent run');
    expect(cmd.textContent).not.toContain('?token=');
    expect(JSON.parse(String(f.mock.calls.find(([, i]) => i?.method === 'POST')![1]!.body))).toEqual({ os: 'linux', arch: 'x86_64', mode: 'user' });
    expect(within(dlg).getByText('Waiting for the agent…')).toBeInTheDocument();
    expect(await within(dlg).findByText('Agent connected.', undefined, { timeout: 6000 })).toBeInTheDocument();
    expect(within(dlg).getByRole('link', { name: 'Open the asset' })).toHaveAttribute('href', '/assets/a1');
  }, 15000);
  it('server tanpa rilis menjelaskan sebabnya, dan alamat yang tidak valid tidak bisa dikirim', async () => {
    route(admin, (u, init) => {
      if (u.endsWith('/agents/enrollment-tokens') && init?.method === 'POST') return problem(409, { detail: 'This server has no agent release to install.', code: 'release_unavailable' });
      if (u.endsWith('/agents')) return json({ items: [] });
    });
    const user = userEvent.setup();
    mount(<AgentsPage />);
    await user.click(await screen.findByRole('button', { name: 'Install the first agent' }));
    const dlg = await screen.findByRole('dialog');
    const base = within(dlg).getByLabelText(/Platform address/);
    await user.clear(base);
    await user.type(base, 'ftp://nope');
    expect(within(dlg).getByRole('button', { name: 'Create install command' })).toBeDisabled();
    expect(within(dlg).getByText(/Enter a full address/)).toBeInTheDocument();
    await user.clear(base);
    await user.type(base, 'https://orchis.example.com');
    await user.click(within(dlg).getByRole('button', { name: 'Create install command' }));
    expect(await within(dlg).findByText(/no agent release to install/)).toBeInTheDocument();
  });
  it('mencabut meminta konfirmasi dan menampilkan galat server', async () => {
    const f = route(admin, (u, init) => {
      if (u.includes('/agents/g1/revoke') && init?.method === 'POST') return problem(409, { detail: 'This agent is already revoked.', code: 'agent_revoked' });
      if (u.endsWith('/agents')) return json({ items: [agent()] });
    });
    const user = userEvent.setup();
    mount(<AgentsPage />);
    await user.click(await screen.findByRole('button', { name: 'Revoke agent on box1' }));
    expect(f.mock.calls.some(([, i]) => i?.method === 'POST')).toBe(false);
    const dlg = await screen.findByRole('dialog');
    await user.click(within(dlg).getByRole('button', { name: 'Revoke agent' }));
    expect(await within(dlg).findByText('This agent is already revoked.')).toBeInTheDocument();
  });
});

describe('AtomicTestDialog with agent actions', () => {
  const contracts = { items: [
    { key: 'nmap.tcp_connect', input_schema: schemas['nmap.tcp_connect'], name: 'Nmap TCP connect scan', description: '', kind: 'scan', output_types: [], timeout_seconds: 600, integration: 'nmap', integration_status: 'online', last_heartbeat_at: null, needs_agent: false },
    { key: 'agent.listening_ports', input_schema: schemas['agent.listening_ports'], name: 'Listening ports (agent)', description: 'Lists listening ports.', kind: 'scan', output_types: [], timeout_seconds: 60, integration: 'agent', integration_status: 'online', last_heartbeat_at: null, needs_agent: true }] };
  const asset = (id: string, name: string, agent_state: string) => ({ id, name, testing_authorized: true, agent_state });
  it('aksi agent hanya menawarkan aset yang punya agent, tanpa pengaturan, dan mengirim params kosong', async () => {
    const f = route(admin, (u, init) => {
      if (u.endsWith('/atomic-tests') && init?.method === 'POST') return json({ id: 't1' }, 201);
      if (u.endsWith('/contracts')) return json(contracts);
      if (u.includes('/assets')) return json({ items: [asset('a1', 'agentless-box', 'none'), asset('a2', 'agent-box', 'online'), asset('a3', 'sleepy-box', 'offline')], next_cursor: null });
    });
    const user = userEvent.setup();
    mount(<AtomicTestDialog onClose={() => {}} onCreated={() => {}} />);
    const dlg = await screen.findByRole('dialog');
    await user.type(within(dlg).getByLabelText('Name'), 'Ports');
    await user.selectOptions(await within(dlg).findByLabelText('Action'), 'agent.listening_ports');
    expect(within(dlg).queryByRole('option', { name: 'agentless-box' })).not.toBeInTheDocument();
    expect(within(dlg).getByRole('option', { name: 'agent-box' })).toBeInTheDocument();
    expect(within(dlg).getByRole('option', { name: 'sleepy-box (agent offline)' })).toBeInTheDocument();
    expect(within(dlg).getByText(/takes no settings/)).toBeInTheDocument();
    expect(within(dlg).queryByLabelText('Ports')).not.toBeInTheDocument();
    await user.selectOptions(within(dlg).getByLabelText('Target asset'), 'a2');
    await user.click(within(dlg).getByRole('button', { name: 'Create test' }));
    await waitFor(() => expect(f.mock.calls.some(([, i]) => i?.method === 'POST')).toBe(true));
    expect(JSON.parse(String(f.mock.calls.find(([, i]) => i?.method === 'POST')![1]!.body))).toEqual({ name: 'Ports', contract_key: 'agent.listening_ports', params: {}, target_asset_id: 'a2' });
  });
});

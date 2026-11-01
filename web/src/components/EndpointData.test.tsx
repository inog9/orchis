import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { EndpointCatalog } from './EndpointCatalog';
import { EndpointDataPanel } from './EndpointDataPanel';

const json = (v: unknown, status = 200) => new Response(JSON.stringify(v), { status, headers: { 'Content-Type': 'application/json' } });
const q = (key: string, title: string, sensitivity: 'low' | 'medium', extra: object = {}) => ({
  key, title, description: `${title} description`, category: 'Endpoint', tables: ['system_info'], columns: ['a'], os: ['linux'], sensitivity, load: 'light', needs_root: false,
  default_enabled: sensitivity === 'low', timeout_seconds: 30, max_rows: 10, mitre: [{ id: 'T1082', reason: 'Reading system details is System Information Discovery.' }],
  attributes: ['os_name'], findings: [{ rule: 'r1', type: 'text', severity: 'high', title: 'Weak setting', explain: 'Because it is weak.' }], report_section: 'Endpoint posture',
  example: [{ a: 'example-value' }], example_findings: [{ rule: 'r1', type: 'text', severity: 'high', title: 'Weak setting' }], example_attributes: {}, ...extra,
});
const consent = { version: '2026-11-01.1', sha256: 'b'.repeat(64), text: 'Collection document: data, purpose, access, retention, stop and delete.' };
const catalog = { consent, osquery_version: '5.24.0', license: 'Apache-2.0', proven_os: ['linux'], queries: [q('osquery.inventory', 'System inventory', 'low'), q('osquery.users', 'Local accounts', 'medium', { tables: ['users', 'groups'] }), q('osquery.pack.it_compliance.mounts', 'mounts', 'medium', { pack: 'it-compliance', tables: ['mounts'] })],
  packs: [{ name: 'it-compliance', description: 'Basic IT compliance facts.', file: 'it-compliance.conf', sha256: 'a'.repeat(64), platform: '', tables_read: ['mounts'], queries: [
    { name: 'mounts', key: 'osquery.pack.it_compliance.mounts', description: '', status: 'runnable', reason: '', tables: ['mounts'] },
    { name: 'chrome_extensions', description: '', status: 'sensitive', reason: 'Skipped: sensitive data (chrome_extensions).', tables: ['chrome_extensions'] }] }] };

type Calls = { url: string; method: string; body?: unknown }[];
function stub(handler: (url: string, method: string) => Response | undefined, calls: Calls = []) {
  vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
    const url = String(input);
    const method = init?.method ?? 'GET';
    calls.push({ url, method, body: init?.body ? JSON.parse(String(init.body)) : undefined });
    if (url.endsWith('/api/v1/osquery/catalog')) return json(catalog);
    return handler(url, method) ?? json({}, 404);
  });
  return calls;
}
function mount(ui: React.ReactNode) {
  return render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}><MemoryRouter>{ui}</MemoryRouter></QueryClientProvider>);
}
afterEach(() => vi.restoreAllMocks());

describe('EndpointCatalog', () => {
  it('shows what each check reads, where it goes, and a real example, without controls when no organisation', async () => {
    stub(() => undefined);
    mount(<EndpointCatalog />);
    const card = await screen.findByRole('article', { name: 'System inventory' });
    expect(within(card).getByText('Linux: tested')).toBeInTheDocument();
    expect(within(card).getByText('macOS and Windows: not proven yet')).toBeInTheDocument();
    expect(within(card).queryByRole('button')).toBeNull();
    await userEvent.click(within(card).getByText('What it reads and where the data goes'));
    expect(within(card).getByText('system_info')).toBeInTheDocument();
    expect(within(card).getByText('Because it is weak.')).toBeInTheDocument();
    expect(within(card).getByText(/Reading system details is System Information Discovery/)).toBeInTheDocument();
    expect(within(card).getByText('example-value')).toBeInTheDocument();
    expect(within(card).getByText('Findings this example would produce')).toBeInTheDocument();
    expect(screen.getByText(/Approval is decided per organisation/)).toBeInTheDocument();
  });

  it('lets an admin approve a sensitive check only after seeing the tables, and turn a default check off directly', async () => {
    const calls = stub((url, method) => {
      if (url.endsWith('/organizations/o1/osquery') && method === 'GET') return json({ items: [{ key: 'osquery.inventory', enabled: true, explicit: false, updated_at: null }, { key: 'osquery.users', enabled: false, explicit: false, updated_at: null }] });
      if (url.includes('/organizations/o1/osquery/') && method === 'PUT') return new Response(null, { status: 204 });
      return undefined;
    });
    mount(<EndpointCatalog orgId="o1" canManage />);
    const users = await screen.findByRole('article', { name: 'Local accounts' });
    expect(within(users).getByText('Needs approval')).toBeInTheDocument();
    expect(within(await screen.findByRole('article', { name: 'System inventory' })).getByText('On by default')).toBeInTheDocument();
    await userEvent.click(within(users).getByRole('button', { name: 'Review and approve' }));
    const dlg = await screen.findByRole('dialog');
    expect(within(dlg).getByText('users')).toBeInTheDocument();
    expect(within(dlg).getByText('groups')).toBeInTheDocument();
    expect(within(dlg).getByRole('region',{name:'Collection document'})).toHaveTextContent(consent.text);
    expect(within(dlg).getByText(/2026-11-01.1/)).toBeInTheDocument();
    expect(calls.some((c) => c.method === 'PUT')).toBe(false);
    await userEvent.click(within(dlg).getByRole('button', { name: 'Approve' }));
    await vi.waitFor(() => expect(calls.find((c) => c.method === 'PUT')).toBeTruthy());
    expect(calls.find((c) => c.method === 'PUT')).toMatchObject({ url: expect.stringContaining('/organizations/o1/osquery/osquery.users'), body: { enabled: true, consent: {version: consent.version,sha256:consent.sha256} } });
    // Query bawaan (sensitivitas rendah) dimatikan langsung tanpa dialog.
    await userEvent.click(within(await screen.findByRole('article', { name: 'System inventory' })).getByRole('button', { name: 'Turn off' }));
    await vi.waitFor(() => expect(calls.filter((c) => c.method === 'PUT')).toHaveLength(2));
    expect(calls.filter((c) => c.method === 'PUT')[1]).toMatchObject({ body: { enabled: false } });
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('shows status but no approval buttons to someone who cannot manage settings', async () => {
    stub((url) => (url.endsWith('/organizations/o1/osquery') ? json({ items: [{ key: 'osquery.users', enabled: true, explicit: true, updated_at: '2026-11-01T00:00:00Z' }] }) : undefined));
    mount(<EndpointCatalog orgId="o1" canManage={false} />);
    const users = await screen.findByRole('article', { name: 'Local accounts' });
    expect(await within(users).findByText('Approved by an administrator')).toBeInTheDocument();
    expect(within(users).queryByRole('button')).toBeNull();
  });

  it('explains a failure to save', async () => {
    stub((url, method) => {
      if (url.endsWith('/organizations/o1/osquery')) return json({ items: [{ key: 'osquery.inventory', enabled: true, explicit: false, updated_at: null }] });
      if (method === 'PUT') return json({ title: 'Forbidden', status: 403, detail: 'You cannot change this.' }, 403);
      return undefined;
    });
    mount(<EndpointCatalog orgId="o1" canManage />);
    await userEvent.click(within(await screen.findByRole('article', { name: 'System inventory' })).getByRole('button', { name: 'Turn off' }));
    expect(await screen.findByText('Couldn’t save this choice')).toBeInTheDocument();
  });
});

const ds = (key: string, title: string, over: object = {}) => ({ key, title, status: 'never', reason: '', collected_at: null, last_attempt_at: null, osquery_version: '', attributes: {}, rows: [], truncated: false, enabled: true, synthetic: false, ...over });

describe('EndpointDataPanel', () => {
  const data = (over: object = {}) => ({
    asset_id: 'a1', version: 1, organization_id: 'o1', has_agent: true, findings: [{ id: 'f1', type: 'text', severity: 'high', title: 'ASLR is weak', value: 'v', query: 'osquery.hardening' }],
    items: [
      ds('osquery.inventory', 'System inventory', { status: 'ok', collected_at: '2026-11-01T01:00:00Z', last_attempt_at: '2026-11-01T01:00:00Z', osquery_version: '5.24.0', attributes: { os_name: 'Ubuntu', os_version: '26.04', kernel: '7.0', cpu_logical_cores: '6', physical_memory: '17179869184' }, rows: [{ os_name: 'Ubuntu' }] }),
      ds('osquery.users', 'Local accounts', { status: 'skipped', reason: 'not approved for this organisation', enabled: false }),
      ds('osquery.firewall', 'Firewall rules', { status: 'failed', reason: 'osquery is not installed' }),
      ds('osquery.network', 'Listening ports', { synthetic: true, status: 'ok', collected_at: '2026-11-01T01:00:00Z', attributes: { listening_port_count: 3 } }),
      ds('osquery.packages', 'Installed packages'),
    ], ...over,
  });

  it('shows each status honestly, the summary, findings, and a Demo data mark', async () => {
    stub((url) => (url.endsWith('/assets/a1/endpoint-data') ? json(data()) : undefined));
    mount(<EndpointDataPanel assetID="a1" allowed canRun />);
    expect(await screen.findAllByText('Collected')).toHaveLength(2);
    expect(screen.getByText(/Ubuntu 26.04 · kernel 7.0 · 6 CPUs · 16 GB RAM/)).toBeInTheDocument();
    expect(screen.getByText('Not approved for this organisation')).toBeInTheDocument();
    expect(screen.getByText('Failed: osquery is not installed')).toBeInTheDocument();
    expect(screen.getByText('Not collected yet')).toBeInTheDocument();
    expect(screen.getByText('Demo data')).toBeInTheDocument();
    expect(screen.getByText('ASLR is weak')).toBeInTheDocument();
    expect(screen.getByText(/3 listening ports/)).toBeInTheDocument();
  });

  it('runs a check now and explains why it could not start', async () => {
    const calls: Calls = [];
    stub((url, method) => {
      if (url.endsWith('/assets/a1/endpoint-data/refresh') && method === 'POST') return json({ title: 'Conflict', status: 409, detail: 'The agent on this asset is offline.' }, 409);
      if (url.endsWith('/assets/a1/endpoint-data')) return json(data());
      return undefined;
    }, calls);
    mount(<EndpointDataPanel assetID="a1" allowed canRun />);
    await userEvent.click(await screen.findByRole('button', { name: 'Run Firewall rules now' }));
    await vi.waitFor(() => expect(calls.find((c) => c.method === 'POST')).toBeTruthy());
    expect(calls.find((c) => c.method === 'POST')?.body).toEqual({ query_key: 'osquery.firewall' });
    expect(await screen.findByText('Couldn’t start the check')).toBeInTheDocument();
    expect(screen.getByText(/offline/)).toBeInTheDocument();
  });

  it('hides Run now without permission, and explains an asset with no agent', async () => {
    stub((url) => (url.endsWith('/assets/a1/endpoint-data') ? json(data({ has_agent: false, findings: [] })) : undefined));
    mount(<EndpointDataPanel assetID="a1" allowed canRun={false} />);
    expect(await screen.findByText(/has no agent, so no endpoint data can be collected/)).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Install an agent' })).toHaveAttribute('href', '/assets/agents');
    expect(screen.queryByRole('button', { name: /Run .* now/ })).toBeNull();
    expect(screen.getByText(/None yet/)).toBeInTheDocument();
  });

  it('hides Run now from a viewer even when the asset has an agent', async () => {
    stub((url) => (url.endsWith('/assets/a1/endpoint-data') ? json(data()) : undefined));
    mount(<EndpointDataPanel assetID="a1" allowed canRun={false} />);
    await screen.findByText('Local accounts');
    expect(screen.queryByRole('button', { name: /Run .* now/ })).toBeNull();
  });

  it('does not fetch or show data when the role cannot read assets', async () => {
    const calls = stub(() => undefined);
    mount(<EndpointDataPanel assetID="a1" allowed={false} canRun={false} />);
    expect(screen.getByText('Your role cannot read asset data.')).toBeInTheDocument();
    expect(calls.length).toBe(0);
  });
});

describe('Official packs', () => {
  it('lists runnable and skipped checks with reasons, and keeps pack checks out of the plain list', async () => {
    stub(() => undefined);
    mount(<EndpointCatalog />);
    const pack = await screen.findByRole('article', { name: 'it-compliance pack' });
    expect(within(pack).getByText('1 check runs')).toBeInTheDocument();
    expect(within(pack).getByText('1 skipped')).toBeInTheDocument();
    expect(within(pack).queryByRole('button')).toBeNull();
    await userEvent.click(within(pack).getByText('What this pack reads and what it skips'));
    expect(within(pack).getByText('Skipped: sensitive data')).toBeInTheDocument();
    expect(within(pack).getByText('Skipped: sensitive data (chrome_extensions).')).toBeInTheDocument();
    expect(screen.queryByRole('article', { name: 'mounts' })).toBeNull();
    const rows = within(pack).getAllByRole('row').slice(1).map((r) => within(r).getAllByRole('cell')[0]?.textContent);
    expect(rows).toEqual(['mounts', 'chrome_extensions']);
  });

  it('shows the tables in a dialog before an admin approves the whole pack, then turns it off directly', async () => {
    const calls = stub((url, method) => {
      if (url.endsWith('/organizations/o1/osquery') && method === 'GET') return json({ items: [{ key: 'osquery.pack.it_compliance.mounts', enabled: false, explicit: false, updated_at: null }] });
      if (url.includes('/organizations/o1/osquery-packs/') && method === 'PUT') return new Response(null, { status: 204 });
      return undefined;
    });
    mount(<EndpointCatalog orgId="o1" canManage />);
    const pack = await screen.findByRole('article', { name: 'it-compliance pack' });
    expect(within(pack).getByText('Needs approval')).toBeInTheDocument();
    await userEvent.click(within(pack).getByRole('button', { name: 'Review and approve' }));
    const dlg = await screen.findByRole('dialog');
    expect(within(dlg).getByText('mounts')).toBeInTheDocument();
    expect(calls.some((c) => c.method === 'PUT')).toBe(false);
    await userEvent.click(within(dlg).getByRole('button', { name: 'Approve pack' }));
    await vi.waitFor(() => expect(calls.find((c) => c.method === 'PUT')).toBeTruthy());
    expect(calls.find((c) => c.method === 'PUT')).toMatchObject({ url: expect.stringContaining('/organizations/o1/osquery-packs/it-compliance'), body: { enabled: true, consent: {version: consent.version,sha256:consent.sha256} } });
  });

  it('hides the approve button from people who cannot manage settings', async () => {
    stub((url) => (url.endsWith('/organizations/o1/osquery') ? json({ items: [] }) : undefined));
    mount(<EndpointCatalog orgId="o1" />);
    const pack = await screen.findByRole('article', { name: 'it-compliance pack' });
    expect(within(pack).queryByRole('button')).toBeNull();
  });
});


describe('Endpoint dataset deletion',()=>{
 it('requires confirmation, sends the current asset version, and discloses retained records',async()=>{
  const calls=stub((url,method)=>{
   if(url.endsWith('/assets/a1/endpoint-data'))return json({asset_id:'a1',version:7,read_only:false,has_agent:true,organization_id:null,items:[],findings:[]});
   if(url.endsWith('/endpoint-data?version=7') && method==='DELETE')return new Response(null,{status:204});
   return undefined;
  });
  mount(<EndpointDataPanel assetID="a1" allowed canRun canDelete />);
  await userEvent.click(await screen.findByRole('button',{name:'Delete collected data'}));
  const dlg=screen.getByRole('dialog');
  expect(within(dlg).getByText(/Findings, frozen reports, audit events and backups remain/)).toBeInTheDocument();
  expect(calls.some(c=>c.method==='DELETE')).toBe(false);
  await userEvent.click(within(dlg).getByRole('button',{name:'Delete data'}));
  await vi.waitFor(()=>expect(calls.some(c=>c.method==='DELETE' && c.url.endsWith('?version=7'))).toBe(true));
 });
 it('hides deletion for a read-only demo asset',async()=>{
  stub(url=>url.endsWith('/assets/a1/endpoint-data')?json({asset_id:'a1',version:1,read_only:true,has_agent:false,organization_id:null,items:[],findings:[]}):undefined);
  mount(<EndpointDataPanel assetID="a1" allowed canRun={false} canDelete />);
  await screen.findByText(/This asset has no agent/);
  expect(screen.queryByRole('button',{name:'Delete collected data'})).toBeNull();
 });
});

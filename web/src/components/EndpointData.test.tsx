import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { EndpointCatalog } from './EndpointCatalog';
import { EndpointDataPanel } from './EndpointDataPanel';

const json = (v: unknown, status = 200) => new Response(JSON.stringify(v), { status, headers: { 'Content-Type': 'application/json' } });
const q = (key: string, title: string, sensitivity: 'low' | 'medium', extra: object = {}) => ({
  key, title, description: `${title} description`, category: 'DefenseBot', tables: ['system_info'], columns: ['a'], os: ['linux'], sensitivity, load: 'light', needs_root: false,
  default_enabled: sensitivity === 'low', timeout_seconds: 30, max_rows: 10, mitre: [{ id: 'T1082', reason: 'Reading system details is System Information Discovery.' }],
  attributes: ['os_name'], findings: [{ rule: 'r1', type: 'text', severity: 'high', title: 'Weak setting', explain: 'Because it is weak.' }], report_section: 'Endpoint posture',
  example: [{ a: 'example-value' }], example_findings: [{ rule: 'r1', type: 'text', severity: 'high', title: 'Weak setting' }], example_attributes: {}, ...extra,
});
const consent = { version: '2026-11-01.1', sha256: 'b'.repeat(64), text: 'Collection document: data, purpose, access, retention, stop and delete.' };
const catalog = { consent, osquery_version: '5.24.0', license: 'Apache-2.0', proven_os: ['linux'], queries: [q('osquery.inventory', 'System inventory', 'low'), q('osquery.users', 'Local accounts', 'medium', { tables: ['users', 'groups'] }), q('osquery.pack.it_compliance.mounts', 'mounts', 'medium', { pack: 'it-compliance', tables: ['mounts'] })],
  packs: [{ name: 'it-compliance', description: 'Basic IT compliance facts.', file: 'it-compliance.conf', sha256: 'a'.repeat(64), platform: '', tables_read: ['mounts'], queries: [
    { name: 'mounts', platform: 'posix', key: 'osquery.pack.it_compliance.mounts', description: '', status: 'runnable', reason: '', tables: ['mounts'] },
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
function mount(ui: React.ReactNode, client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })) {
  return render(<QueryClientProvider client={client}><MemoryRouter>{ui}</MemoryRouter></QueryClientProvider>);
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

const user = { id: 'u1', email: 'a@b.c', display_name: 'Ada', role: 'analyst', permissions: ['simulation.read', 'assessment.read', 'report.read', 'asset.read'] };
const endpointData = (over: object = {}) => ({ asset_id: 'a1', version: 1, read_only: false, organization_id: 'o1', has_agent: true, supported: true, findings: [],
  items: [{ key: 'osquery.packages', title: 'Installed packages', status: 'ok', reason: '', collected_at: '2026-11-08T01:00:00Z', last_attempt_at: '2026-11-08T01:00:00Z', osquery_version: '5.24.0', attributes: {}, rows: [], truncated: false, enabled: true, synthetic: false }], ...over });
const defenseScan = { id: 's1', organization_id: 'o1', name: 'Org', status: 'finished', runner: 'injector', created_at: '2026-11-08T02:00:00Z', ended_at: '2026-11-08T02:05:00Z', options: { organization_id: 'o1', asset_ids: ['a1'], tools: ['defensebot'], preset: 'defense' } };
const defenseDetail = (disposition: 'complete' | 'limited') => ({ ...defenseScan, report_id: 'r1', report_status: 'ready', jobs: [{ id: 'j1', asset_id: 'a1', range_id: null, name: 'box', target: 'local', tool: 'defensebot', status: disposition === 'complete' ? 'finished' : 'incomplete', error: '', simulation_id: 'x', findings: [],
  defense: { schema: 1, disposition, software_checked: 964, software_unmapped: 3, findings: 4, omitted: 1, by_severity: { high: 2, medium: 2 }, known_exploited: 1, exposed: 3, reboot_required: 1, hardening: 1, notes: [{ code: 'apps_unmapped', count: 3, limiting: true }], provenance: { engine: 'defensebot/1', queried_at: '2026-11-08T02:00:00Z' } } }] });
function defenseStub(over: { data?: object; scans?: object[]; detail?: object; me?: object | null } = {}, calls: Calls = []) {
  return stub((url, method) => {
    if (url.endsWith('/auth/me')) return over.me === null ? json({}, 401) : json(over.me ?? user);
    if (url.endsWith('/assets/a1/endpoint-data')) return json(endpointData(over.data));
    if (url.includes('/api/v1/scans?org=o1')) return json({ items: over.scans ?? [], next_cursor: null });
    if (url.endsWith('/api/v1/scans/s1')) return json(over.detail ?? defenseDetail('complete'));
    if (url.endsWith('/api/v1/scans') && method === 'POST') return json({ ...defenseScan, status: 'running', id: 's2' }, 201);
    return undefined;
  }, calls);
}

describe('DefenseBot panel', () => {
  it('says DefenseBot has not run yet and starts a run for this asset only when the button is pressed', async () => {
    const calls: Calls = [];
    defenseStub({}, calls);
    mount(<EndpointDataPanel assetID="a1" allowed canRun />);
    expect(await screen.findByText('DefenseBot has not run on this asset yet.')).toBeInTheDocument();
    expect(calls.some((c) => c.method === 'POST')).toBe(false);
    await userEvent.click(screen.getByRole('button', { name: 'Run DefenseBot' }));
    await vi.waitFor(() => expect(calls.find((c) => c.method === 'POST')).toBeTruthy());
    expect(calls.find((c) => c.method === 'POST')?.body).toMatchObject({ organization_id: 'o1', asset_ids: ['a1'], tools: ['defensebot'] });
  });

  it('shows the latest run as counts and coverage notes, never as an inventory', async () => {
    defenseStub({ scans: [defenseScan], detail: defenseDetail('limited') });
    mount(<EndpointDataPanel assetID="a1" allowed canRun />);
    expect(await screen.findByText('964')).toBeInTheDocument();
    const adv = screen.getByText('Advisories').parentElement!;
    expect(within(adv).getByText('4')).toBeInTheDocument();
    expect(within(screen.getByText('Known exploited').parentElement!).getByText('1')).toBeInTheDocument();
    expect(screen.getByText(/Incomplete result/)).toBeInTheDocument();
    expect(screen.getByText('3 applications could not be mapped to vulnerability data.')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Open report' })).toHaveAttribute('href', '/reports?assessment_id=s1');
    expect(screen.getByRole('link', { name: 'Open scan' })).toHaveAttribute('href', '/simulations/s1');
    expect(screen.queryByText('Installed packages')).toBeNull();
    expect(screen.queryByRole('table')).toBeNull();
  });

  it('does not call a complete run incomplete', async () => {
    defenseStub({ scans: [defenseScan], detail: defenseDetail('complete') });
    mount(<EndpointDataPanel assetID="a1" allowed canRun />);
    await screen.findByText('964');
    expect(screen.queryByText(/Incomplete result/)).toBeNull();
  });

  it('explains an asset with no agent and keeps the button off', async () => {
    defenseStub({ data: { has_agent: false } });
    mount(<EndpointDataPanel assetID="a1" allowed canRun />);
    expect(await screen.findByText(/Install the agent on this asset to run DefenseBot/)).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Install an agent' })).toHaveAttribute('href', '/assets/agents');
    expect(screen.getByRole('button', { name: 'Run DefenseBot' })).toBeDisabled();
  });

  it('explains an agent that cannot collect and an asset without testing authorization', async () => {
    defenseStub({ data: { supported: false } });
    mount(<EndpointDataPanel assetID="a1" allowed canRun />);
    expect(await screen.findByText('DefenseBot supports Linux and macOS agents only.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Run DefenseBot' })).toBeDisabled();
  });

  it('explains missing authorization in words', async () => {
    defenseStub();
    mount(<EndpointDataPanel assetID="a1" allowed canRun authorized={false} />);
    expect(await screen.findByText(/Testing must be authorized/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Run DefenseBot' })).toBeDisabled();
  });

  it('hides the button from someone who may not run scans', async () => {
    defenseStub();
    mount(<EndpointDataPanel assetID="a1" allowed canRun={false} />);
    await screen.findByText('DefenseBot has not run on this asset yet.');
    expect(screen.queryByRole('button', { name: 'Run DefenseBot' })).toBeNull();
  });

  it('shows why a launch was refused', async () => {
    stub((url, method) => {
      if (url.endsWith('/auth/me')) return json(user);
      if (url.endsWith('/assets/a1/endpoint-data')) return json(endpointData());
      if (url.includes('/api/v1/scans?org=o1')) return json({ items: [], next_cursor: null });
      if (url.endsWith('/api/v1/scans') && method === 'POST') return json({ title: 'Conflict', status: 409, detail: 'DefenseBot collection is not approved for this organisation.', code: 'conflict' }, 409);
      return undefined;
    });
    mount(<EndpointDataPanel assetID="a1" allowed canRun />);
    await userEvent.click(await screen.findByRole('button', { name: 'Run DefenseBot' }));
    expect(await screen.findByRole('alert')).toBeInTheDocument();
  });

  it('does not fetch or show data when the role cannot read assets', async () => {
    const calls = stub(() => undefined);
    mount(<EndpointDataPanel assetID="a1" allowed={false} canRun={false} />);
    expect(screen.getByText('Your role cannot read asset data.')).toBeInTheDocument();
    expect(calls.filter((c) => c.url.includes('/endpoint-data')).length).toBe(0);
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

  it('includes POSIX checks and their versions when approving a Linux pack', async () => {
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
    expect(calls.find((c) => c.method === 'PUT')).toMatchObject({ url: expect.stringContaining('/organizations/o1/osquery-packs/it-compliance'), body: { enabled: true, versions: {'osquery.pack.it_compliance.mounts':0}, consent: {version: consent.version,sha256:consent.sha256} } });
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
  await screen.findByText(/Install the agent on this asset to run DefenseBot/);
  expect(screen.queryByRole('button',{name:'Delete collected data'})).toBeNull();
 });
});


describe('Collection review versions', () => {
  it.each(['query', 'pack'])('keeps the %s version reviewed until a conflict is explicitly reloaded', async (kind) => {
    const key = kind === 'pack' ? 'osquery.pack.it_compliance.mounts' : 'osquery.users';
    let version = 2;
    const calls = stub((url, method) => {
      if (method === 'GET' && url.endsWith('/organizations/o1/osquery')) return json({ items: [{ key, version, enabled: false, explicit: true }] });
      if (method === 'PUT') return version === 3 && calls.filter(c => c.method === 'PUT').length === 1
        ? json({ title: 'Conflict', status: 409, detail: 'Collection choices changed; reload before saving.', code: 'collection_conflict' }, 409)
        : new Response(null, { status: 204 });
      return undefined;
    });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
    mount(<EndpointCatalog orgId="o1" canManage />, client);
    const article = await screen.findByRole('article', { name: kind === 'pack' ? 'it-compliance pack' : 'Local accounts' });
    await userEvent.click(await within(article).findByRole('button', { name: 'Review and approve' }));
    version = 3;
    await client.invalidateQueries();
    const button = kind === 'pack' ? 'Approve pack' : 'Approve';
    await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: button }));
    const dialog = screen.getByRole('dialog');
    expect(await within(dialog).findByText('Collection choices changed; reload before saving.')).toBeInTheDocument();
    const sent = calls.filter(c => c.method === 'PUT')[0]!.body;
    expect(sent).toMatchObject(kind === 'pack' ? { versions: { [key]: 2 } } : { version: 2 });
    await userEvent.click(within(dialog).getByRole('button', { name: 'Reload choices' }));
    expect(screen.queryByRole('dialog')).toBeNull();
    await userEvent.click(await within(article).findByRole('button', { name: 'Review and approve' }));
    await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: button }));
    await vi.waitFor(() => expect(calls.filter(c => c.method === 'PUT')).toHaveLength(2));
    expect(calls.filter(c => c.method === 'PUT')[1]!.body).toMatchObject(kind === 'pack' ? { versions: { [key]: 3 } } : { version: 3 });
  });
});

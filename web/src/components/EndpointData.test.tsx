import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { EndpointDataPanel } from './EndpointDataPanel';

const json = (v: unknown, status = 200) => new Response(JSON.stringify(v), { status, headers: { 'Content-Type': 'application/json' } });

type Calls = { url: string; method: string; body?: unknown }[];
function stub(handler: (url: string, method: string) => Response | undefined, calls: Calls = []) {
  vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
    const url = String(input);
    const method = init?.method ?? 'GET';
    calls.push({ url, method, body: init?.body ? JSON.parse(String(init.body)) : undefined });
    return handler(url, method) ?? json({}, 404);
  });
  return calls;
}
function mount(ui: React.ReactNode, client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })) {
  return render(<QueryClientProvider client={client}><MemoryRouter>{ui}</MemoryRouter></QueryClientProvider>);
}
afterEach(() => vi.restoreAllMocks());

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
    expect(screen.queryByText('3 applications could not be mapped to vulnerability data.')).toBeNull();
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

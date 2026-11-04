import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, expect, it, vi } from 'vitest';
import type { AgentRow } from '../api/types';
import { OrganizationEndpoints } from './OrganizationEndpoints';
const permissions = ['organization.read', 'asset.read', 'agent.read', 'agent.manage', 'settings.manage', 'organization.write', 'asset.authorize', 'asset.write'];
const base: AgentRow = { id: '11111111-1111-4111-8111-111111111111', asset_id: 'asset-1', asset_name: 'Endpoint', instance_name: 'macbook', hostname: 'mb', os: 'macos', arch: 'x86_64', mode: 'service', version: '1.3.1', status: 'online', last_heartbeat_at: null, enrolled_at: '2026-11-04T00:00:00Z', control_version: 1, control_protocol: 1, update_available: false } as AgentRow;
function mount(agent: AgentRow) {
  vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => { const u = String(input); if (u.includes('/auth/me')) return Response.json({ id: 'u', email: 'a@b.c', display_name: 'Ada', role: 'admin', permissions }); if (u.endsWith('/networks')) return Response.json({ reported: [], approvals: [], max_hosts: 256, runner: 'injector' }); if (u.includes('/osquery/catalog')) return Response.json({ consent: { version: 'v1', sha256: 'x', text: 't' }, osquery_version: '5.24.0', license: 'Apache-2.0', proven_os: ['linux'], queries: [], packs: [] }); if (u.includes('/endpoint')) return Response.json({ asset_id: 'asset-1', version: 1, read_only: false, organization_id: 'org-1', has_agent: true, supported: true, items: [], findings: [] }); return Response.json({ items: [], next_cursor: null }); });
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><MemoryRouter initialEntries={[`/organizations/org-1?tab=agents&endpoint=${agent.id}`]}><OrganizationEndpoints org="org-1" agents={[agent]} /></MemoryRouter></QueryClientProvider>);
}
afterEach(() => vi.restoreAllMocks());
it('shows one clear notice for a macOS agent instead of controls that can never work', async () => {
  mount(base);
  expect(await screen.findByText(/only available for Linux amd64 agents/)).toBeInTheDocument();
  expect(await screen.findByRole('button', { name: 'Pause agent' })).toBeEnabled();
  for (const name of ['Collect data now', 'Delete collected data', 'Approve range']) expect(screen.queryByRole('button', { name })).toBeNull();
  expect(screen.queryByText('Network ranges reported by this agent')).toBeNull();
});
it('keeps endpoint data, collection and network ranges for a Linux amd64 agent', async () => {
  mount({ ...base, os: 'linux', instance_name: 'lab' } as AgentRow);
  expect(await screen.findByText('Network ranges reported by this agent')).toBeInTheDocument();
  expect(await screen.findByRole('button', { name: 'Collect data now' })).toBeInTheDocument();
  expect(screen.queryByText(/only available for Linux amd64 agents/)).toBeNull();
  expect(screen.queryByRole('link', { name: /Review collection consent/ })).toBeNull();
});

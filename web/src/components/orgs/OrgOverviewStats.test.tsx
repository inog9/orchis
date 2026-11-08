import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, expect, it, vi } from 'vitest';
import type { AgentRow } from '../../api/types';
import { agentCounts, OrgOverviewStats, severityTotal } from './OrgOverviewStats';

const json = (v: unknown) => new Response(JSON.stringify(v));
const ag = (status: AgentRow['status']) => ({ status }) as AgentRow;
afterEach(() => vi.restoreAllMocks());

it('agentCounts mengabaikan agent yang dicabut; severityTotal menghitung info', () => {
  expect(agentCounts([ag('online'), ag('offline'), ag('offline'), ag('revoked')])).toEqual({ total: 3, online: 1, offline: 2 });
  expect(severityTotal({ info: 13 })).toBe(13);
  expect(severityTotal(undefined)).toBe(0);
});

it('kartu: aset dan otorisasi dari /home, agent online/offline, temuan dari laporan, scan terakhir dari /scans', async () => {
  vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
    const u = new URL(String(input), 'http://x');
    if (u.pathname === '/api/v1/home') return json({ organizations: [{ id: 'o1', name: 'O', assets: 2, authorized: 1, risk: '', comparison: '', latest_assessment: null, latest_status: '' }], assets: null, findings: null, atomic: null, chains: null, latest_simulations: [], latest_runs: [], most_exposed: [], injectors: [], generated_at: '' });
    if (u.pathname === '/api/v1/scans') return json({ items: [{ id: 's', organization_id: 'o1', name: 'n', status: 'finished', runner: 'injector', options: { tools: ['nmap'], preset: 'standard', asset_ids: [], range_ids: [], organization_id: 'o1' }, created_at: new Date(Date.now() - 3 * 86_400_000).toISOString(), ended_at: null }], next_cursor: null });
    return json({ items: [] });
  });
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><MemoryRouter><OrgOverviewStats orgID="o1" agents={[ag('online'), ag('offline'), ag('offline')]} bySeverity={{ info: 13 }} canAssets canScans canFindings /></MemoryRouter></QueryClientProvider>);
  const assets = await screen.findByRole('group', { name: 'Assets' });
  expect(await within(assets).findByText('1 authorised')).toBeInTheDocument();
  expect(within(screen.getByRole('group', { name: 'Agents' })).getByText('1 online · 2 offline')).toBeInTheDocument();
  expect(within(screen.getByRole('group', { name: 'Findings' })).getByLabelText('13')).toBeInTheDocument();
  const last = screen.getByRole('group', { name: 'Last scan' });
  expect(await within(last).findByText('Finished')).toBeInTheDocument();
  expect(within(last).queryByText(/standard/)).toBeNull();
});

it('tanpa izin tidak ada kartu dan tidak ada panggilan API', () => {
  const f = vi.spyOn(globalThis, 'fetch').mockResolvedValue(json({}));
  render(<QueryClientProvider client={new QueryClient()}><MemoryRouter><OrgOverviewStats orgID="o1" agents={[]} bySeverity={undefined} canAssets={false} canScans={false} canFindings={false} /></MemoryRouter></QueryClientProvider>);
  expect(screen.getAllByRole('group')).toHaveLength(1);
  expect(f).not.toHaveBeenCalled();
});

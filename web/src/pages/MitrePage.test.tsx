import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { MitrePage } from './MitrePage';

const json = (value: unknown) => new Response(JSON.stringify(value), { headers: { 'Content-Type': 'application/json' } });
function mount(trained: boolean) {
  const cell = { id: 'T1046', name: 'Network Service Discovery', mapped: true, exercised: trained, simulated: true, simulated_runs: 6, simulated_findings: 2, runs: trained ? 2 : 0, last_run_at: trained ? '2026-11-01T00:00:00Z' : null, platforms: ['Linux'], subs: [] };
  vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => String(input).endsWith('/mitre/matrix')
    ? json({ imported: true, total: 1, mapped: 1, exercised: trained ? 1 : 0, simulated: 1, platforms: ['Linux'], last_run: null, tactics: [{ id: 'TA0007', name: 'Discovery', shortname: 'discovery', techniques: [cell] }] })
    : json({ ...cell, description: 'A technique fixture.', tactics: ['Discovery'], url: 'https://attack.mitre.org/techniques/T1046', actions: ['Nmap'], mappings: [], mitigations: [] }));
  return render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><MemoryRouter><MitrePage /></MemoryRouter></QueryClientProvider>);
}
afterEach(() => vi.restoreAllMocks());

describe('MITRE evidence', () => {
  it('focuses the selected details and returns to the technique when closed with Escape', async () => {
    mount(false);
    const cell = await screen.findByRole('button', { name: /^T1046 / });
    await userEvent.click(cell);
    expect(await screen.findByRole('region', { name: 'Technique details' })).toHaveFocus();
    await userEvent.keyboard('{Escape}');
    expect(screen.queryByRole('region', { name: 'Technique details' })).toBeNull();
    expect(cell).toHaveFocus();
  });
  it.each([false, true])('keeps demo evidence visible when trained=%s', async (trained) => {
    mount(trained);
    const cell = await screen.findByRole('button', { name: /^T1046 / });
    expect(cell).toHaveAccessibleName(new RegExp(trained ? 'trained:.*simulated:' : 'mapped:.*simulated:'));
    expect(within(cell).getByText('Demo')).toBeVisible();
    await userEvent.click(cell);
    const panel = await screen.findByRole('region', { name: 'Technique details' });
    expect(within(panel).getByText(trained ? '2 successful runs' : 'Not yet')).toBeVisible();
    expect(within(panel).getByText(/6 illustrative successful runs and 2 finding references/)).toBeVisible();
    expect(within(panel).getByText(/These never count as trained/)).toBeVisible();
  });

  it('explains overlapping counts and retains the demo badge when filtering mapped actions', async () => {
    mount(false);
    await screen.findByRole('button', { name: /^T1046 / });
    await userEvent.click(screen.getByText('What do the colours mean?'));
    expect(screen.getByText(/These counts can overlap/)).toBeVisible();
    await userEvent.click(screen.getByRole('checkbox', { name: 'Only mapped techniques' }));
    expect(within(screen.getByRole('button', { name: /^T1046 / })).getByText('Demo')).toBeVisible();
  });
});

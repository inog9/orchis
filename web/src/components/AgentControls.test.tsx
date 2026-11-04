import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import type { AgentRow } from '../api/types';
import { AgentControls, AgentRollout } from './AgentControls';
const agent: AgentRow = { id: '11111111-1111-4111-8111-111111111111', asset_id: 'asset', asset_name: 'Endpoint alpha', hostname: 'fixture', os: 'linux', arch: 'x86_64', mode: 'user', version: '1.0.0', status: 'online', last_heartbeat_at: null, enrolled_at: '2026-11-03T00:00:00Z', update_available: true, control_version: 7, control_protocol: 1, paused: false, uninstall_status: 'none' };
function mount(ui: React.ReactNode) { render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}>{ui}</QueryClientProvider>); }
afterEach(() => vi.restoreAllMocks());
it('sends the observed control version and exact uninstall confirmation', async () => {
 const fetch = vi.spyOn(globalThis, 'fetch').mockResolvedValue(Response.json({ ...agent, uninstall_status: 'queued' }));
 mount(<AgentControls agent={agent} />); const user = userEvent.setup();
 await user.click(screen.getByRole('button', { name: 'Uninstall remotely' }));
 const confirm = screen.getByRole('button', { name: 'Confirm remote uninstall' });
 expect(confirm).toBeDisabled(); await user.type(screen.getByLabelText('Type the agent ID to confirm'), 'wrong'); expect(confirm).toBeDisabled();
 await user.clear(screen.getByLabelText('Type the agent ID to confirm')); await user.type(screen.getByLabelText('Type the agent ID to confirm'), agent.id); await user.click(confirm);
 expect(fetch).toHaveBeenCalledTimes(1); expect(JSON.parse(String(fetch.mock.calls[0]?.[1]?.body))).toEqual({ action: 'uninstall', version: 7, confirmation: agent.id });
 expect(fetch.mock.calls[0]?.[1]?.headers).toMatchObject({ 'X-Requested-With': 'orchis-web' });
});
it('keeps queued uninstall visibly unconfirmed and blocks new commands', () => {
 mount(<AgentControls agent={{ ...agent, status: 'offline', paused: true, uninstall_status: 'dispatched' }} />);
 expect(screen.getByText(/Removal has not been confirmed/)).toBeInTheDocument();
 for (const name of ['Update now', 'Resume agent', 'Collect data now', 'Uninstall remotely']) expect(screen.getByRole('button', { name })).toBeDisabled();
 expect(screen.queryByText(/confirmed physical removal/)).not.toBeInTheDocument();
});
it('disables collection while paused and reports missing osquery honestly', () => {
 mount(<AgentControls agent={{ ...agent, paused: true, control_protocol: 0, osquery_version: '' }} />);
 expect(screen.getByRole('button', { name: 'Collect data now' })).toBeDisabled(); expect(screen.getByRole('button', { name: 'Uninstall remotely' })).toBeDisabled();
 expect(screen.getByText(/osquery not reported/)).toBeInTheDocument();
});
it('saves manual rollout with the frozen policy version', async () => {
 const fetch = vi.spyOn(globalThis, 'fetch').mockImplementation(async (_, init) => Response.json(init?.method === 'PUT' ? { mode: 'manual', version: 5 } : { mode: 'automatic', version: 4 }));
 mount(<AgentRollout />); const user = userEvent.setup(); await user.selectOptions(await screen.findByLabelText('Rollout policy'), 'manual'); await user.click(screen.getByRole('button', { name: 'Save rollout policy' }));
 const save = fetch.mock.calls.find(([, init]) => init?.method === 'PUT'); expect(JSON.parse(String(save?.[1]?.body))).toEqual({ mode: 'manual', version: 4 });
});

it('shows confirmed removal without obsolete pause or update instructions',()=>{
 mount(<AgentControls agent={{...agent,status:'revoked',paused:true,uninstall_status:'completed',update_status:'unknown'}}/>);
 expect(screen.getByText(/confirmed physical removal/)).toBeInTheDocument();
 expect(screen.queryByText(/Agent paused:/)).not.toBeInTheDocument();
 expect(screen.queryByText(/Release status is unknown/)).not.toBeInTheDocument();
 expect(screen.queryByRole('button',{name:'Update now'})).not.toBeInTheDocument();
});
it('does not offer data collection on an agent type that cannot collect', () => {
 mount(<AgentControls agent={{ ...agent, os: 'macos' }} />);
 expect(screen.queryByRole('button', { name: 'Collect data now' })).toBeNull();
 expect(screen.getByRole('button', { name: 'Pause agent' })).toBeEnabled();
 expect(screen.getByRole('button', { name: 'Uninstall remotely' })).toBeEnabled();
});
it('explains next to the buttons why a control is off', () => {
 mount(<AgentControls agent={{ ...agent, paused: true, update_available: false }} />);
 const note = screen.getByRole('note');
 expect(note).toHaveTextContent('Update now is off: this agent already runs the latest release.');
 expect(note).toHaveTextContent('Collect data now is off while the agent is paused. Resume it first.');
});
it('says the agent is offline when collection is unavailable for that reason', () => {
 mount(<AgentControls agent={{ ...agent, status: 'offline', update_available: true }} />);
 expect(screen.getByRole('note')).toHaveTextContent('Collect data now is off while the agent is offline.');
});

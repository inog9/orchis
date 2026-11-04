import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { NotificationPolicy } from './NotificationPolicy';
import { notificationEN, notificationID, notificationTitle } from '../i18n/notifications';
import { AlertItem, alertTarget } from '../pages/NotificationsPage';
import type { AlertRow } from '../api/types';
function mount() { render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}><NotificationPolicy /></QueryClientProvider>); }
afterEach(() => vi.restoreAllMocks());
it('submits the observed notification policy version and uses CSRF', async () => {
 const fetch = vi.spyOn(globalThis, 'fetch').mockImplementation(async (_u, init) => Response.json(init?.method === 'PUT' ? { offline_minutes: 20, version: 8 } : { offline_minutes: 15, version: 7 }));
 mount(); const user = userEvent.setup(); const input = await screen.findByLabelText('Offline threshold (minutes)');
 expect(screen.getByRole('button', { name: 'Save notification policy' })).toBeDisabled();
 await user.clear(input); await user.type(input, '20'); await user.click(screen.getByRole('button', { name: 'Save notification policy' }));
 await waitFor(() => expect(fetch.mock.calls.some(c => c[1]?.method === 'PUT')).toBe(true));
 const call = fetch.mock.calls.find(c => c[1]?.method === 'PUT');
 expect(JSON.parse(String(call?.[1]?.body))).toEqual({ offline_minutes: 20, version: 7 });
 expect(call?.[1]?.headers).toMatchObject({ 'X-Requested-With': 'orchis-web' });
 await waitFor(() => expect(screen.getByRole('button', { name: 'Save notification policy' })).toBeDisabled());
});
it('keeps out-of-range policy values unsavable and offers reload on conflict', async () => {
 vi.spyOn(globalThis, 'fetch').mockImplementation(async (_u, init) => init?.method === 'PUT' ? Response.json({ title: 'Conflict', detail: 'Changed policy' }, { status: 409 }) : Response.json({ offline_minutes: 15, version: 1 }));
 mount(); const user = userEvent.setup(); const input = await screen.findByLabelText('Offline threshold (minutes)');
 await user.clear(input); await user.type(input, '1'); expect(screen.getByRole('button', { name: 'Save notification policy' })).toBeDisabled();
 await user.clear(input); await user.type(input, '10081'); expect(screen.getByRole('button', { name: 'Save notification policy' })).toBeDisabled();
 await user.clear(input); await user.type(input, '30'); await user.click(screen.getByRole('button', { name: 'Save notification policy' }));
 await user.click(await screen.findByRole('button', { name: 'Reload saved notification policy' }));
 expect(input).toHaveValue(15);
});
it('has matching EN/ID event keys and routes each new event to an existing object page', () => {
 expect(Object.keys(notificationID).sort()).toEqual(Object.keys(notificationEN).sort());
 for (const key of Object.keys(notificationEN)) { expect(notificationTitle(key, 'RAW', 'en')).not.toBe('RAW'); expect(notificationTitle(key, 'RAW', 'id')).not.toBe('RAW'); }
 const a = { resource_id: 'object' } as AlertRow;
 for (const [type, path] of [['scan', '/simulations/object'], ['report', '/reports/object'], ['agent', '/assets/agents?agent=object'], ['asset', '/assets/object'], ['organization', '/organizations/object']] as const) expect(alertTarget({ ...a, resource_type: type })).toBe(path);
 expect(alertTarget({ ...a, resource_type: 'unknown' })).toBeNull();
});

it('escapes object labels and distinguishes report references', () => {
 render(<ul><AlertItem a={{ id: 'a', kind: 'report_ready', title: 'RAW SERVER TITLE', severity: 'info', resource_type: 'report', resource_id: 'prefix-12345678', resource_label: '<script>private fixture</script>', body: null, created_at: '2026-11-03T00:00:00Z', read: false }} onOpen={() => {}} /></ul>);
 expect(screen.getByText('Report PDF is ready')).toBeVisible();
 expect(screen.getByText('<script>private fixture</script> · 12345678')).toBeVisible();
 expect(document.querySelector('script')).toBeNull();
 expect(screen.queryByText('RAW SERVER TITLE')).not.toBeInTheDocument();
});

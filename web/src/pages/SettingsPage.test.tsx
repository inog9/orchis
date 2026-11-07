import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, expect, it, vi } from 'vitest';
import { PlatformPanel } from './PlatformPanel';
import { SettingsPage } from './SettingsPage';

const perms = ['settings.manage', 'user.manage', 'integration.read', 'audit.read', 'asset.read', 'scenario.read'];
const json = (v: unknown, status = 200) => new Response(JSON.stringify(v), { status });
function stub() {
  vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
    const u = new URL(String(input), 'http://x');
    if (u.pathname.endsWith('/auth/me')) return json({ id: 'u', permissions: perms });
    if (u.pathname === '/api/v1/settings') return json({ platform_name: 'Acme', version: 'v9', components: [{ name: 'PostgreSQL', status: 'up' }] });
    if (u.pathname.endsWith('/scan-control')) return json({ paused: false, global_paused: false, window: {}, version: 1, allowed_now: true });
    if (u.pathname.endsWith('/retention')) return json({ findings_days: 180, scan_days: 90, report_days: 180, endpoint_days: 30, audit_days: 365, version: 1, cleanup_pending: 0 });
    return json({ items: [], next_cursor: null, runs: [], last_run: null });
  });
}
const mount = (initial: string) => render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><MemoryRouter initialEntries={[initial]}><Routes><Route path="/settings" element={<SettingsPage />}><Route index element={<PlatformPanel />} /><Route path="users" element={<p>users-tab</p>} /></Route></Routes></MemoryRouter></QueryClientProvider>);
afterEach(() => vi.restoreAllMocks());

it('Hentikan darurat dan Retensi hanya muncul di tab Platform, tidak di tab lain', async () => {
  stub();
  const { unmount } = mount('/settings');
  expect(await screen.findByRole('button', { name: /stop all/i })).toBeInTheDocument();
  expect(screen.getByRole('heading', { name: /data retention/i })).toBeInTheDocument();
  unmount();
  mount('/settings/users');
  expect(await screen.findByText('users-tab')).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: /stop all/i })).toBeNull();
  expect(screen.queryByRole('heading', { name: /data retention/i })).toBeNull();
});
it('tab aktif ditandai dan hanya tab yang diizinkan tampil', async () => {
  stub(); mount('/settings');
  expect(await screen.findByRole('link', { name: 'Platform' })).toHaveClass('is-active');
  expect(await screen.findByRole('link', { name: 'Audit log' })).toBeInTheDocument();
});

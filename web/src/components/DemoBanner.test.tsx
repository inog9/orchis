import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { DemoBanner, type DemoStatus } from './DemoBanner';

type Me = { id: string; email: string; display_name: string; role: string; permissions: string[] } | null;

const admin: Me = { id: 'u1', email: 'admin@example.test', display_name: 'Admin', role: 'admin', permissions: ['settings.manage'] };
const viewer: Me = { id: 'u2', email: 'viewer@example.test', display_name: 'Viewer', role: 'viewer', permissions: [] };

function fetchMock({ me, status, onPurge }: { me: Me; status: DemoStatus; onPurge?: () => Response | Promise<Response> }) {
  let current = status;
  const fn = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    const method = (init?.method ?? 'GET').toUpperCase();
    if (url.startsWith('/api/v1/auth/me')) return new Response(JSON.stringify(me));
    if (url === '/api/v1/demo/status' && method === 'GET') return new Response(JSON.stringify(current));
    if (url === '/api/v1/demo/purge' && method === 'POST') {
      if (onPurge) { const response = await onPurge(); if (response.ok) current = await response.clone().json(); return response; }
      current = { has_demo: false, counts: {} };
      return new Response(JSON.stringify(current));
    }
    return new Response('{}');
  });
  vi.spyOn(globalThis, 'fetch').mockImplementation(fn as unknown as typeof fetch);
  return fn;
}

function mount(opts: { me: Me; status: DemoStatus; onPurge?: () => Response | Promise<Response> }) {
  const fetch = fetchMock(opts);
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <DemoBanner />
    </QueryClientProvider>,
  );
  return { fetch };
}

afterEach(() => vi.restoreAllMocks());

describe('DemoBanner visibility', () => {
  it('stays absent when there is no demo data', async () => {
    const { fetch } = mount({ me: admin, status: { has_demo: false, counts: {} } });
    await waitFor(() => expect(fetch).toHaveBeenCalledWith('/api/v1/demo/status', expect.anything()));
    expect(screen.queryByText('Demo data')).toBeNull();
    expect(screen.queryByRole('status')).toBeNull();
  });

  it('appears with a clear English label and counts when demo data is present', async () => {
    mount({ me: admin, status: { has_demo: true, counts: { assets: 3, findings: 5, audit_log: 2, tags: 2 }, seed: 42 } });
    expect(await screen.findByText('Demo data')).toBeInTheDocument();
    const banner = screen.getByRole('status', { name: 'Demo data notice' });
    expect(banner).toHaveTextContent('synthetic demo data');
    expect(banner).toHaveTextContent('3 assets');
    expect(banner).toHaveTextContent('5 findings');
    expect(banner).toHaveTextContent('seed 42');
    expect(banner).not.toHaveTextContent('audit_log');
    expect(banner).toHaveTextContent('not evidence of any real security issue');
  });
});

describe('DemoBanner permissions', () => {
  it('lets a viewer see the notice without any purge action', async () => {
    mount({ me: viewer, status: { has_demo: true, counts: { assets: 1 } } });
    expect(await screen.findByText('Demo data')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /purge/i })).toBeNull();
  });
});

describe('DemoBanner purge (settings.manage)', () => {
  it('opens a confirmation modal instead of window.confirm, and cancel closes it without purging', async () => {
    const confirmSpy = vi.spyOn(window, 'confirm').mockImplementation(() => false);
    const { fetch } = mount({ me: admin, status: { has_demo: true, counts: { assets: 2 } } });
    await screen.findByText('Demo data');
    const user = userEvent.setup();
    await user.click(screen.getByRole('button', { name: 'Purge demo data' }));

    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByText('Purge demo data?')).toBeInTheDocument();
    expect(confirmSpy).not.toHaveBeenCalled();

    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }));
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(fetch).not.toHaveBeenCalledWith('/api/v1/demo/purge', expect.anything());
  });

  it('purges on confirm, invalidates and the banner disappears', async () => {
    const { fetch } = mount({ me: admin, status: { has_demo: true, counts: { assets: 2 } } });
    await screen.findByText('Demo data');
    const user = userEvent.setup();
    await user.click(screen.getByRole('button', { name: 'Purge demo data' }));
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: 'Purge demo data' }));

    expect(fetch).toHaveBeenCalledWith('/api/v1/demo/purge', expect.objectContaining({ method: 'POST' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    await waitFor(() => expect(screen.queryByText('Demo data')).toBeNull());
  });

  it('disables the actions and shows a pending state while the purge request is in flight', async () => {
    let resolvePurge!: (r: Response) => void;
    const pending = new Promise<Response>((resolve) => {
      resolvePurge = resolve;
    });
    mount({ me: admin, status: { has_demo: true, counts: { assets: 2 } }, onPurge: () => pending });
    await screen.findByText('Demo data');
    const user = userEvent.setup();
    await user.click(screen.getByRole('button', { name: 'Purge demo data' }));
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: 'Purge demo data' }));

    expect(await within(dialog).findByRole('button', { name: 'Purging…' })).toBeDisabled();
    expect(within(dialog).getByRole('button', { name: 'Cancel' })).toBeDisabled();

    resolvePurge(new Response(JSON.stringify({ has_demo: false, counts: {} })));
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
  });

  it('shows a readable error without leaking server details when the purge fails', async () => {
    mount({
      me: admin,
      status: { has_demo: true, counts: { assets: 2 } },
      onPurge: () =>
        new Response(JSON.stringify({ title: 'Internal Server Error', detail: 'panic: leaked stack trace at db.go:42', error_id: 'err_abc' }), { status: 500 }),
    });
    await screen.findByText('Demo data');
    const user = userEvent.setup();
    await user.click(screen.getByRole('button', { name: 'Purge demo data' }));
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: 'Purge demo data' }));

    expect(await within(dialog).findByRole('alert')).toHaveTextContent('The platform hit an unexpected error');
    expect(screen.queryByText(/leaked stack trace/)).toBeNull();
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(within(dialog).getByRole('button', { name: 'Purge demo data' })).not.toBeDisabled();
  });
});


describe('Demo artifact cleanup', () => {
  it('shows pending cleanup after records are gone and retries without asking to delete records again', async () => {
    const { fetch } = mount({ me: admin, status: { has_demo: false, cleanup_pending: 2, counts: {} } });
    expect(await screen.findByText('Demo cleanup pending')).toBeInTheDocument();
    expect(screen.getByRole('status')).toHaveTextContent('Storage or search cleanup is still pending');
    await userEvent.setup().click(screen.getByRole('button', { name: 'Retry demo cleanup' }));
    expect(screen.queryByRole('dialog')).toBeNull();
    await waitFor(() => expect(fetch).toHaveBeenCalledWith('/api/v1/demo/purge', expect.objectContaining({ method: 'POST' })));
    await waitFor(() => expect(screen.queryByRole('status')).toBeNull());
  });

  it('keeps pending cleanup visible to a viewer without granting retry permission', async () => {
    mount({ me: viewer, status: { has_demo: false, cleanup_pending: 1, counts: {} } });
    expect(await screen.findByText('Demo cleanup pending')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /retry/i })).toBeNull();
  });
});

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { exposureLevel, HomePage } from './HomePage';

const me = { id: 'u', email: 'a@b.c', display_name: 'Ada', role: 'analyst', permissions: ['organization.read', 'organization.write', 'finding.read', 'integration.read', 'assessment.run'] };
const json = (v: unknown, status = 200) => new Response(JSON.stringify(v), { status });

const home = {
  organizations: [{ id: 'org1', name: 'Acme', assets: 3, authorized: 2, risk: 'medium', comparison: '', latest_assessment: null, latest_status: 'assessed' }],
  assets: { total: 3, authorized: 2, internet_facing: 1 },
  findings: { total: 6, vulnerabilities: 2, open_ports: 3, vulnerable_assets: 1, new_7d: 1, by_severity: { critical: 1, high: 2, medium: 1, low: 0, info: 2 } },
  atomic: { tests: 4, active: 1, runs_7d: 5, failed_7d: 1 },
  chains: { scenarios: 2, simulations: 7, running: 0 },
  latest_simulations: [{ id: 's1', code: 'OP-0028', name: 'srv-web', status: 'finished', started_at: '2026-11-08T17:30:00Z', created_at: '2026-11-08T17:30:00Z' }],
  latest_runs: [],
  most_exposed: [{ asset_id: 'a1', name: 'srv-web', critical: 1, high: 0, medium: 1, low: 0, open_ports: 3 }],
  injectors: [
    { key: 'nmap', name: 'Nmap', status: 'online' },
    { key: 'nuclei', name: 'Nuclei', status: 'online' },
    { key: 'hackbot', name: 'Hackbot', status: 'online' },
    { key: 'defensebot', name: 'orchis-defense-agent', status: 'online' },
  ],
  generated_at: '2026-11-08T18:06:00Z',
};

const assetsPage = {
  items: [{ id: 'a1', category: 'web_application', name: 'srv-web', addresses: [{ kind: 'url', value: 'https://srv-web.example', is_primary: true }] }],
  next_cursor: null,
};

const findingsPage = {
  items: [
    { id: 'f1', asset_id: 'a1', asset_name: 'srv-web', type: 'vulnerability', value: 'tpl@srv', title: 'SQL Injection di /rest/products/search', severity: 'critical', details: { template_id: 'sqli', matched_at: '/rest/products/search' }, first_seen_at: '2026-11-08T10:00:00Z', last_seen_at: '2026-11-08T11:00:00Z', occurrence_count: 1 },
    { id: 'f2', asset_id: 'a1', asset_name: 'srv-web', type: 'vulnerability', value: 'cve@tar', title: 'CVE-2026-59873 in tar', severity: 'high', details: { package: 'tar', installed_version: '7.5.15', fixed_version: '7.5.19', fix_status: 'fixed', method: 'osv-distro' }, first_seen_at: '2026-11-08T09:00:00Z', last_seen_at: '2026-11-08T12:00:00Z', occurrence_count: 1 },
    { id: 'f3', asset_id: null, asset_name: null, type: 'vulnerability', value: 'unknown', title: 'Unmapped source finding', severity: 'medium', details: {}, first_seen_at: '2026-11-08T08:00:00Z', last_seen_at: '2026-11-08T08:30:00Z', occurrence_count: 1 },
  ],
  next_cursor: null,
};

const runningSim = {
  items: [{
    id: 'sim1', number: 28, code: 'OP-0029', name: 'srv-web', status: 'running', scenario_id: 'sc1', scenario_name: 'Web chain',
    safe_mode: true, timeout_seconds: 600, cancel_reason: null, started_at: '2026-11-09T08:00:00Z', ended_at: null,
    created_at: '2026-11-09T08:00:00Z', target_count: 2, step_count: 3, steps_done: 1, finding_count: 0,
  }],
};

function mount(summary: unknown, overrides: { simulations?: unknown; assets?: unknown; findings?: unknown; organizations?: unknown } = {}) {
  vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
    const url = String(input);
    if (url.includes('/auth/me')) return json(me);
    if (url.includes('/simulations')) return json(overrides.simulations ?? { items: [] });
    if (url.includes('/findings')) return json(overrides.findings ?? findingsPage);
    if (url.includes('/assets')) return json(overrides.assets ?? assetsPage);
    if (url.includes('/organizations')) return json(overrides.organizations ?? { items: [], next_cursor: null });
    return json(summary);
  });
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}><MemoryRouter><HomePage /></MemoryRouter></QueryClientProvider>);
}

afterEach(() => vi.restoreAllMocks());

describe('exposureLevel', () => {
  it('menurunkan tingkat dari severity tertinggi', () => {
    expect(exposureLevel({}, 0).label).toBe('No data yet');
    expect(exposureLevel({ critical: 1, high: 3 }, 4).label).toBe('High exposure');
    expect(exposureLevel({ high: 1 }, 1).label).toBe('Elevated exposure');
    expect(exposureLevel({ medium: 2 }, 2).label).toBe('Moderate exposure');
    expect(exposureLevel({ low: 2 }, 2).label).toBe('Low exposure');
  });
});

describe('HomePage', () => {
  it('menampilkan notice, heading, badge IDLE, metrik, panel, dan status engine', async () => {
    mount(home);
    expect(await screen.findByText(/Data updated/)).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Overview' })).toBeInTheDocument();
    expect(screen.getByText('IDLE · no scan running')).toBeInTheDocument();
    expect(screen.getByText('1 critical · 2 high · 1 medium · 0 low · 2 info')).toBeInTheDocument();
    expect(screen.getByText('Assets monitored')).toBeInTheDocument();
    expect(screen.getByText('2 authorized · 1 internet-facing')).toBeInTheDocument();
    expect(screen.getByText('Atomic tests')).toBeInTheDocument();
    expect(screen.getByText('1 active · 5 runs / 7 days')).toBeInTheDocument();
    expect(screen.getByText('Simulations')).toBeInTheDocument();
    expect(screen.getByText('0 running · 2 scenarios')).toBeInTheDocument();
    expect((await screen.findAllByText('WEB')).length).toBeGreaterThan(0);
    expect(screen.getByText('+1 more findings')).toHaveAttribute('href', '/findings');
    expect(screen.getAllByText('Online')).toHaveLength(4);
    const srvLinks = screen.getAllByRole('link', { name: /srv-web/ });
    expect(srvLinks.some((l) => l.getAttribute('href') === '/assets/a1')).toBe(true);
    expect(srvLinks.some((l) => l.getAttribute('href') === '/simulations/legacy/s1')).toBe(true);
  });

  it('node temuan bisa diexpand untuk melihat bukti lalu diciutkan kembali', async () => {
    mount(home);
    const web = await screen.findByRole('button', { name: /SQL Injection di \/rest\/products\/search/ });
    expect(web).toHaveAttribute('aria-expanded', 'false');
    await userEvent.click(web);
    expect(web).toHaveAttribute('aria-expanded', 'true');
    expect(within(web).getByText('Template: sqli')).toBeInTheDocument();
    expect(within(web).getByText('Matched: /rest/products/search')).toBeInTheDocument();
    expect(within(web).getByText(/Last seen/)).toBeInTheDocument();
    await userEvent.click(web);
    expect(web).toHaveAttribute('aria-expanded', 'false');
    expect(within(web).queryByText('Template: sqli')).not.toBeInTheDocument();
  });

  it('node temuan orchis-defense-agent menampilkan bukti paket dan fix dari details yang ada', async () => {
    mount(home);
    const def = await screen.findByRole('button', { name: /CVE-2026-59873 in tar/ });
    await userEvent.click(def);
    expect(within(def).getByText('Package: tar 7.5.15 → 7.5.19')).toBeInTheDocument();
    expect(within(def).getByText('Fix: fixed')).toBeInTheDocument();
  });

  it('state LIVE merender satu graph terpisah per simulasi yang berjalan', async () => {
    mount({ ...home, chains: { scenarios: 2, simulations: 7, running: 2 } }, { simulations: runningSim });
    expect(await screen.findByText('LIVE')).toBeInTheDocument();
    expect(await screen.findByText('OP-0029')).toBeInTheDocument();
    expect(screen.getByText('1/3 steps done')).toBeInTheDocument();
    expect(screen.getByText('2 targets')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /SQL Injection di \/rest\/products\/search/ })).toBeInTheDocument();
  });

  it('bagian yang tidak dikirim server (null) tidak dirender', async () => {
    mount({ ...home, findings: null, atomic: null });
    expect(await screen.findByText(/Data updated/)).toBeInTheDocument();
    expect(screen.queryByText('Total findings')).not.toBeInTheDocument();
    expect(screen.queryByText('Atomic tests')).not.toBeInTheDocument();
    expect(screen.getByText('Assets monitored')).toBeInTheDocument();
  });

  it('workspace kosong memusatkan langkah awal, bukan deretan nol', async () => {
    mount({
      ...home,
      organizations: [],
      assets: { total: 0, authorized: 0, internet_facing: 0 },
      findings: { total: 0, vulnerabilities: 0, open_ports: 0, vulnerable_assets: 0, new_7d: 0, by_severity: {} },
      atomic: { tests: 0, active: 0, runs_7d: 0, failed_7d: 0 },
      chains: { scenarios: 0, simulations: 0, running: 0 },
      latest_simulations: [],
      most_exposed: [],
    });
    expect(await screen.findByRole('heading', { name: 'No assessment results yet' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Bring your assets together' })).toBeInTheDocument();
    expect(screen.queryByText('Live scan')).not.toBeInTheDocument();
    expect(screen.queryByText('Total findings')).not.toBeInTheDocument();
  });

  it('memperingatkan bila ada injector yang belum terhubung', async () => {
    mount({ ...home, injectors: [{ key: 'nmap', name: 'Nmap', status: 'offline' }, { key: 'hackbot', name: 'Hackbot', status: 'online' }] });
    await screen.findByText(/Data updated/);
    expect(screen.getByRole('status')).toHaveTextContent(/Nmap injector is not connected/i);
    expect(screen.getByText('Offline')).toBeInTheDocument();
  });

  it('tombol Segarkan memuat ulang data', async () => {
    mount(home);
    expect(await screen.findByText(/Data updated/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Refresh' }));
    expect(await screen.findByText(/Data updated/)).toBeInTheDocument();
  });

  it('gagal memuat menampilkan error dan bisa dicoba lagi', async () => {
    let n = 0;
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
      const url = String(input);
      if (url.includes('/auth/me')) return json(me);
      if (url.includes('/findings')) return json(findingsPage);
      if (url.includes('/assets')) return json(assetsPage);
      return ++n === 1 ? json({ title: 'x', status: 500, error_id: 'err_9' }, 500) : json(home);
    });
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(<QueryClientProvider client={qc}><MemoryRouter><HomePage /></MemoryRouter></QueryClientProvider>);
    await userEvent.click(await screen.findByRole('button', { name: /try again/i }));
    expect(await screen.findByText(/Data updated/)).toBeInTheDocument();
  });
});

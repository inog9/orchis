import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { exposureLevel, HomePage } from './HomePage';

const me = { id: 'u', email: 'a@b.c', display_name: 'Ada', role: 'analyst', permissions: [] };
const json = (v: unknown, status = 200) => new Response(JSON.stringify(v), { status });
const base = {
  assets: { total: 3, authorized: 2, internet_facing: 1 },
  findings: { total: 6, vulnerabilities: 2, open_ports: 3, vulnerable_assets: 1, new_7d: 4, by_severity: { critical: 1, high: 0, medium: 1, low: 0, info: 4 } },
  atomic: { tests: 2, active: 0, runs_7d: 5, failed_7d: 1 },
  latest_runs: [{ execution_id: 'e1', test_id: 't1', test_name: 'Web exposure', contract_name: 'Nuclei', target: 'srv-web', state: 'ended', outcome: 'success', created_at: '2026-10-29T10:00:00Z', finding_count: 2 }],
  most_exposed: [{ asset_id: 'a1', name: 'srv-web', critical: 1, high: 0, medium: 1, low: 0, open_ports: 3 }],
  injectors: [{ key: 'nmap', name: 'Nmap', status: 'online' }, { key: 'nuclei', name: 'Nuclei', status: 'online' }],
  generated_at: '2026-10-29T10:00:00Z',
};

function mount(summary: unknown, status = 200) {
  vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => (String(input).includes('/auth/me') ? json(me) : json(summary, status)));
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
  it('menampilkan angka, postur paparan, run terbaru, dan aset paling terpapar', async () => {
    mount(base);
    const kpis = await screen.findByRole('list', { name: 'Key numbers' });
    expect(within(kpis).getByText('Total findings').parentElement).toHaveTextContent('6');
    expect(within(kpis).getByText('+4 this week')).toBeInTheDocument();
    expect(within(kpis).getByText('2 authorized for testing')).toBeInTheDocument();
    expect(within(kpis).getByText('1 did not complete')).toBeInTheDocument();
    expect(within(kpis).getByText('Injectors online').parentElement).toHaveTextContent('2');
    expect(within(kpis).getAllByRole('listitem')).toHaveLength(8);
    expect(screen.getByText('High exposure')).toBeInTheDocument();
    expect(within(screen.getByRole('list', { name: 'Vulnerabilities by severity' })).getByText('Critical').parentElement).toHaveTextContent('1');
    expect(screen.getByRole('link', { name: 'Web exposure' })).toHaveAttribute('href', '/atomic/t1');
    expect(screen.getByRole('link', { name: 'srv-web' })).toHaveAttribute('href', '/assets/a1');
    expect(screen.queryByText('Get started')).not.toBeInTheDocument();
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
  });

  it('data kosong menampilkan langkah awal, bukan deretan nol yang menyesatkan', async () => {
    mount({ ...base, assets: { total: 0, authorized: 0, internet_facing: 0 }, atomic: { tests: 0, active: 0, runs_7d: 0, failed_7d: 0 }, latest_runs: [], most_exposed: [],
      findings: { total: 0, vulnerabilities: 0, open_ports: 0, vulnerable_assets: 0, new_7d: 0, by_severity: {} } });
    expect(await screen.findByText('No data yet')).toBeInTheDocument();
    expect(screen.queryByText('Run an atomic test')).not.toBeInTheDocument();
    expect(screen.getByText('No data yet')).toBeInTheDocument();
    expect(screen.getByText('Nothing has run yet.')).toBeInTheDocument();
    expect(screen.getByText('No findings on your assets yet.')).toBeInTheDocument();
  });

  it('workspace organisasi baru memusatkan langkah berikutnya tanpa tabel nol', async () => {
    mount({...base,organizations:[],assets:{total:0,authorized:0,internet_facing:0},findings:null,atomic:null,chains:null,latest_runs:[],most_exposed:[]});
    expect(await screen.findByRole('heading',{name:'No assessment results yet'})).toBeInTheDocument();
    expect(screen.queryByRole('list',{name:'Key numbers'})).not.toBeInTheDocument();
  });

  it('hanya menampilkan bagian yang diizinkan server (bagian null disembunyikan)', async () => {
    mount({ ...base, findings: null, atomic: null, latest_runs: [], most_exposed: [], injectors: [] });
    const kpis = await screen.findByRole('list', { name: 'Key numbers' });
    expect(within(kpis).getAllByRole('listitem')).toHaveLength(1);
    expect(screen.queryByText('Exposure posture')).not.toBeInTheDocument();
    expect(screen.queryByText('Latest single tests')).not.toBeInTheDocument();
  });

  it('memperingatkan bila ada injector yang belum terhubung', async () => {
    mount({ ...base, injectors: [{ key: 'nmap', name: 'Nmap', status: 'offline' }, { key: 'nuclei', name: 'Nuclei', status: 'online' }] });
    expect(await screen.findByRole('status')).toHaveTextContent(/nmap injector is not connected/i);
  });

  it('gagal memuat menampilkan error dan bisa dicoba lagi', async () => {
    let n = 0;
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
      if (String(input).includes('/auth/me')) return json(me);
      return ++n === 1 ? json({ title: 'x', status: 500, error_id: 'err_9' }, 500) : json(base);
    });
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(<QueryClientProvider client={qc}><MemoryRouter><HomePage /></MemoryRouter></QueryClientProvider>);
    await userEvent.click(await screen.findByRole('button', { name: /try again/i }));
    expect(await screen.findByText('High exposure')).toBeInTheDocument();
  });
});

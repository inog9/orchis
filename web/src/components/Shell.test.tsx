import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { Shell } from './Shell';

const me = { id: 'u', email: 'a@b.c', display_name: 'Ada', role: 'admin', permissions: ['organization.read', 'organization.write', 'finding.read', 'simulation.read', 'settings.manage'] };
const json = (v: unknown) => new Response(JSON.stringify(v), { status: 200 });

function mount(path = '/organizations') {
  vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
    const u = new URL(String(input), 'http://x');
    if (u.pathname.endsWith('/auth/me')) return json(me);
    if (u.pathname === '/api/v1/home') return json({ organizations: [], assets: null, findings: { total: 12, vulnerabilities: 12, open_ports: 0, vulnerable_assets: 1, by_severity: { critical: 1 }, new_7d: 0 }, atomic: null, chains: null, latest_simulations: [], latest_runs: [], most_exposed: [], injectors: [{ key: 'nmap', name: 'nmap', status: 'online' }], generated_at: '' });
    return json({ items: [], unread: 0 });
  });
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}><MemoryRouter initialEntries={[path]}><Shell><p>isi</p></Shell></MemoryRouter></QueryClientProvider>);
}

beforeEach(() => localStorage.clear());
afterEach(() => vi.restoreAllMocks());

describe('Shell', () => {
  it('label WORKSPACE tidak diulang sebagai judul bilah samping', async () => {
    const { container } = mount();
    await screen.findByRole('link', { name: /organisations/i });
    expect(container.querySelector('.nav-brand')).toBeNull();
  });

  it('bilah samping bisa diciutkan dan pilihannya diingat', async () => {
    const { container } = mount();
    const btn = await screen.findByRole('button', { name: 'Collapse sidebar' });
    expect(container.querySelector('.dock--nav')?.classList.contains('is-collapsed')).toBe(false);
    await userEvent.click(btn);
    expect(container.querySelector('.dock--nav')?.classList.contains('is-collapsed')).toBe(true);
    expect(localStorage.getItem('orchis.nav.collapsed')).toBe('1');
    expect(screen.getByRole('button', { name: 'Expand sidebar' })).toHaveAttribute('aria-expanded', 'false');
  });

  it('lencana temuan menampilkan jumlah dengan teks, bukan hanya warna', async () => {
    mount();
    const link = await screen.findByRole('link', { name: /findings, 12 findings/i });
    expect(link.querySelector('.nav-badge')?.textContent).toBe('12'); // count nav netral (AUDIT UI-025)
  });

  it('pemilih bahasa tetap terlihat di bilah atas, tanpa membuka menu', async () => {
    // Keputusan pemilik (AUDIT UI-028): dropdown bahasa diganti toggle minimal EN | ID.
    const { container } = mount();
    await screen.findByRole('link', { name: /organisations/i });
    expect(container.querySelector('.topbar .lang-toggle')).not.toBeNull();
    expect(screen.getByRole('button', { name: 'EN', pressed: true })).toBeVisible();
    expect(screen.getByRole('button', { name: 'ID' })).toBeVisible();
  });
});

describe('Shell tema', () => {
  it('tombol tema membalik data-theme, menyimpan pilihan, dan labelnya ikut berganti', async () => {
    localStorage.setItem('orchis.theme', 'dark');
    document.documentElement.setAttribute('data-theme', 'dark');
    mount();
    const btn = await screen.findByRole('button', { name: 'Switch to light theme' });
    await userEvent.click(btn);
    expect(document.documentElement.getAttribute('data-theme')).toBe('light');
    expect(localStorage.getItem('orchis.theme')).toBe('light');
    expect(screen.getByRole('button', { name: 'Switch to dark theme' })).toBeInTheDocument();
  });
});

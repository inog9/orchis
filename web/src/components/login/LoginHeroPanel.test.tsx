import { act, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { LoginHeroPanel } from './LoginHeroPanel';
import { TelemetryFeed } from './TelemetryFeed';

const mediaMock = (reduced: boolean) => vi.stubGlobal('matchMedia', (query: string) => ({
  matches: query.includes('reduced-motion') ? reduced : true,
  media: query, addEventListener: () => {}, removeEventListener: () => {}, addListener: () => {}, removeListener: () => {}, dispatchEvent: () => false, onchange: null,
}));

beforeEach(() => {
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null);
});
afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  vi.useRealTimers();
});

const mount = () => render(<MemoryRouter><LoginHeroPanel /></MemoryRouter>);

describe('LoginHeroPanel', () => {
  it('menampilkan merek, status, dan judul; dekorasi tersembunyi dari pembaca layar dan tidak ada kontrol interaktif', () => {
    mediaMock(false);
    const { container } = mount();
    expect(screen.getByText('All systems operational')).toBeInTheDocument();
    expect(screen.getByRole('img', { name: 'ORCHIS' })).toBeInTheDocument();
    expect(container.querySelector('.hero-headline')?.textContent?.replace(/\s+/g, ' ').trim()).toBe('Watchful by nature. Precise by design.');
    for (const sel of ['.hero-bg', '.radar', '.tele']) expect(container.querySelector(sel)?.getAttribute('aria-hidden')).toBe('true');
    expect(container.querySelectorAll('button, a, input, select, textarea').length).toBe(0);
    expect(container.querySelectorAll('.hero-word--shine').length).toBe(2);
  });

  it('kartu metrik pada status berupa tooltip yang terhubung dengan aria-describedby', () => {
    mediaMock(false);
    const { container } = mount();
    const pill = container.querySelector('.hero-pill')!;
    const tip = container.querySelector('[role="tooltip"]')!;
    expect(pill.getAttribute('aria-describedby')).toBe(tip.id);
    expect(tip.textContent).toMatch(/Uptime.*99\.98%.*Last scan.*2m ago.*Region.*ID-JKT.*Latency.*24 ms/);
    expect(pill.getAttribute('tabindex')).toBe('0');
  });

  it('gerak dikurangi: tampilan statis, tiga baris telemetri tetap, tanpa loop animasi', () => {
    mediaMock(true);
    const raf = vi.spyOn(window, 'requestAnimationFrame');
    const { container } = mount();
    expect(container.querySelector('.hero')?.classList.contains('is-still')).toBe(true);
    expect(container.querySelectorAll('.tele__line').length).toBe(3);
    expect(raf).not.toHaveBeenCalled();
  });
});

describe('TelemetryFeed', () => {
  it('mengetik bertahap dan tidak pernah menampilkan lebih dari tiga baris', () => {
    vi.useFakeTimers();
    const { container } = render(<TelemetryFeed running still={false} />);
    expect(container.querySelectorAll('.tele__line').length).toBe(1); // hanya kursor
    act(() => { vi.advanceTimersByTime(1500 + 30 * 8); });
    const first = container.querySelector('.tele__line')!.textContent!;
    expect(first.length).toBeGreaterThan(0);
    expect(first.length).toBeLessThan(25); // belum selesai diketik
    act(() => { vi.advanceTimersByTime(60_000); });
    const lines = container.querySelectorAll('.tele__line');
    expect(lines.length).toBeLessThanOrEqual(3);
    expect(lines.length).toBe(3);
  });

  it('berhenti saat tidak aktif (tab tersembunyi): tidak ada baris baru', () => {
    vi.useFakeTimers();
    const { container, rerender } = render(<TelemetryFeed running still={false} />);
    act(() => { vi.advanceTimersByTime(5000); });
    const before = container.textContent;
    rerender(<TelemetryFeed running={false} still={false} />);
    act(() => { vi.advanceTimersByTime(30_000); });
    expect(container.textContent).toBe(before);
  });
});

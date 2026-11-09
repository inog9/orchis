import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, expect, it, vi } from 'vitest';
import { LandingPage } from './LandingPage';
import { setLanguage } from '../i18n/runtime';

afterEach(() => { setLanguage('en'); vi.restoreAllMocks(); });
const mount = () => render(<MemoryRouter><LandingPage/></MemoryRouter>);
it('describes benefits without public operational data or data requests', () => {
    const fetch = vi.spyOn(globalThis,'fetch');
    const {container} = mount();
    expect(screen.getByRole('heading',{level:1})).toHaveTextContent('Know what to fix.');
    expect(screen.getByRole('heading',{name:'Protect what your work depends on.'})).toBeInTheDocument();
    expect(fetch).not.toHaveBeenCalled();
    expect(screen.getByText('Supported by')).toBeInTheDocument();
    expect(screen.getByRole('link',{name:'Jogja Cyber Security'})).toHaveAttribute('href','https://jogjacybersec.com/');
    expect(container.querySelector('.supporter-credit img')).toHaveAttribute('src');
    expect(container.textContent).not.toMatch(/\d|Nmap|Nuclei|MITRE|osquery|Juice Shop|uptime|latency|Start free|simulations executed/i);
    for (const link of screen.getAllByRole('link')) expect(link.getAttribute('href')).toMatch(/^(\/login|\/|#main|#why-orchis|https:\/\/jogjacybersec\.com\/)$/);
});
it('makes the same product story available in Indonesian without exposing metrics', async () => {
    const {container} = mount();
    await userEvent.click(screen.getByRole('button',{name:'ID'}));
    expect(screen.getByRole('heading',{level:1})).toHaveTextContent('Pahami kondisi Anda.');
    expect(screen.getByText('Buka ruang kerja Anda',{selector:'.public-hero a'})).toHaveAttribute('href','/login');
    expect(container.textContent).not.toMatch(/\d|113|telemetri|99\.98/);
});

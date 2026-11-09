import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
import { render, screen, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it } from 'vitest';
import type { Finding } from '../../api/types';
import { FindingDrawer } from './FindingDrawer';

const finding = (details: Record<string, unknown>): Finding => ({ id: 'f1', asset_id: 'a1', asset_name: 'web-01', type: 'vulnerability', value: 'defensebot:x', title: 'CVE-2024-1 in openssl 3.0.13', severity: 'high', details,
  first_seen_at: '2026-11-08T00:00:00Z', last_seen_at: '2026-11-08T01:00:00Z', occurrence_count: 1, organization_id: 'o1', organization_name: 'Org' });

describe('FindingDrawer for orchis-defense-agent findings', () => {
  it('shows the advisory, versions, status, exposure and method in words, with a safe external link', () => {
    render(<QueryClientProvider client={qc}><MemoryRouter><FindingDrawer canAsset onClose={() => {}} finding={finding({ method: 'osv-distro', cve: 'CVE-2024-1', package: 'openssl', installed_version: '3.0.13-0ubuntu3.4', fixed_version: '3.0.13-0ubuntu3.5', fix_status: 'fix_available', kev: true, epss: 0.314,
      exposure: 'exposed', exposure_ports: '443/tcp', advisory_url: 'https://ubuntu.com/security/CVE-2024-1', severity_source: 'distribution priority', data_as_of: '2026-11-08T01:00:00Z', confirmed_by: 'Nuclei' })} /></MemoryRouter></QueryClientProvider>);
    const section = screen.getByRole('heading', { name: 'orchis-defense-agent' }).closest('section')!;
    expect(within(section).getAllByText('Known exploited').length).toBeGreaterThan(0);
    expect(within(section).getByText('EPSS 31.4%')).toBeInTheDocument();
    expect(within(section).getByText('Also found by Nuclei')).toBeInTheDocument();
    expect(within(section).getByText('Fixed in 3.0.13-0ubuntu3.5')).toBeInTheDocument();
    expect(within(section).getByText('Seen by scanner (443/tcp)')).toBeInTheDocument();
    expect(within(section).getByText('distribution priority')).toBeInTheDocument();
    const link = within(section).getByRole('link', { name: 'CVE-2024-1' });
    expect(link).toHaveAttribute('href', 'https://ubuntu.com/security/CVE-2024-1');
    expect(link).toHaveAttribute('rel', expect.stringContaining('noopener'));
  });
  it('shows no orchis-defense-agent section for other findings', () => {
    render(<QueryClientProvider client={qc}><MemoryRouter><FindingDrawer canAsset onClose={() => {}} finding={finding({ template_id: 'x' })} /></MemoryRouter></QueryClientProvider>);
    expect(screen.queryByRole('heading', { name: 'orchis-defense-agent' })).toBeNull();
  });
});

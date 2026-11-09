import { describe, expect, it } from 'vitest';
import type { Finding } from '../api/types';
import { applyDefenseFilters, defenseOf } from './findingsView';

const f = (id: string, details: Record<string, unknown>, type = 'vulnerability'): Finding => ({ id, asset_id: 'a', asset_name: 'A', type, value: id, title: id, severity: 'high', details,
  first_seen_at: '2026-11-08T00:00:00Z', last_seen_at: '2026-11-08T00:00:00Z', occurrence_count: 1 });

describe('orchis-defense-agent evidence', () => {
  it('reads evidence only from orchis-defense-agent vulnerability findings and never trusts a link scheme', () => {
    const d = defenseOf(f('x', { method: 'osv-distro', cve: 'CVE-1', package: 'openssl', installed_version: '1', fixed_version: '2', fix_status: 'fix_available', kev: true, epss: 0.31, exposure: 'exposed', exposure_ports: '443/tcp', advisory_url: 'https://ubuntu.com/security/CVE-1', confirmed_by: 'Nuclei' }));
    expect(d).toMatchObject({ cve: 'CVE-1', pkg: 'openssl', fixed: '2', kev: true, epss: 0.31, exposure: 'exposed', url: 'https://ubuntu.com/security/CVE-1', confirmedBy: 'Nuclei' });
    expect(defenseOf(f('y', { method: 'osv-exact', advisory_url: 'javascript:alert(1)' }))?.url).toBe('');
    expect(defenseOf(f('z', { method: 'nuclei' }))).toBeNull();
    expect(defenseOf(f('t', { method: 'osv-distro' }, 'text'))).toBeNull();
  });
  it('filters known-exploited and seen-by-scanner findings, and keeps everything when no filter is on', () => {
    const items = [f('a', { method: 'osv-distro', kev: true, exposure: 'exposed' }), f('b', { method: 'osv-distro', kev: true, exposure: 'listening' }), f('c', { method: 'osv-distro', exposure: 'exposed' }), f('d', { template_id: 'x' })];
    expect(applyDefenseFilters(items, { kev: false, exposed: false })).toHaveLength(4);
    expect(applyDefenseFilters(items, { kev: true, exposed: false }).map((x) => x.id)).toEqual(['a', 'b']);
    expect(applyDefenseFilters(items, { kev: false, exposed: true }).map((x) => x.id)).toEqual(['a', 'c']);
    expect(applyDefenseFilters(items, { kev: true, exposed: true }).map((x) => x.id)).toEqual(['a']);
  });
});

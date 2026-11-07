import { useQuery } from '@tanstack/react-query';
import { api } from './client';
import type { Asset } from './types';

// Kesiapan per aset: satu sumber untuk wizard, checklist aset, dan dashboard cakupan.
export type ReadinessItem = {
    id: string;
    name: string;
    authorized: boolean;
    reason: string;
    web_reason: string;
    defense_reason: string;
    os?: string;
    arch?: string;
    agent_status: 'none' | 'online' | 'offline';
    url?: string;
    last_scan_at?: string;
    last_scan_status?: string;
};
export type Readiness = {
    total: number;
    authorized: number;
    ready: number;
    scan_window_open: boolean;
    emergency_stop: boolean;
    control_reason?: string;
    items: ReadinessItem[];
};

export function useReadiness(org: string, enabled = true) {
    return useQuery({
        queryKey: ['readiness', org],
        enabled: enabled && !!org,
        queryFn: () => api.get<Readiness>(`/api/v1/organizations/${encodeURIComponent(org)}/readiness`),
        staleTime: 15_000,
    });
}

// Cadangan bila readiness belum tersedia: turunkan kesiapan dari data aset itu sendiri
// (otorisasi, URL disetujui, status agent). Keadaan consent hanya diketahui server.
export function synthesizedReadiness(a: Asset): ReadinessItem {
    const url = a.addresses.find((d) => d.kind === 'url' && d.scan_approved !== false && /^https?:\/\//i.test(d.value))?.value;
    const unauthorized = !a.testing_authorized;
    return {
        id: a.id,
        name: a.name,
        authorized: !unauthorized,
        reason: unauthorized ? 'Not authorized for testing' : '',
        web_reason: unauthorized ? 'Not authorized for testing' : url ? '' : 'No approved web URL on this asset.',
        defense_reason: a.agent_state === 'online' ? '' : a.agent_state === 'offline'
            ? 'The agent is offline; bring it back online to collect software data.'
            : 'No agent is installed on this asset.',
        agent_status: a.agent_state === 'online' ? 'online' : a.agent_state === 'offline' ? 'offline' : 'none',
        url,
    };
}

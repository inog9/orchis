import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from './client';
import { language } from '../i18n/runtime';
import type { Scan } from './scans';

export type DefenseNote = { code: string; count?: number; detail?: string; limiting?: boolean };
export type DefenseSummary = {
  schema: number; disposition: 'complete' | 'limited'; platform?: string; os_name?: string; os_version?: string;
  software_checked: number; software_unmapped: number; software_ignored?: number; findings: number; omitted: number; by_severity: Record<string, number>;
  known_exploited: number; exposed: number; reboot_required: number; hardening: number; notes: DefenseNote[];
  provenance: { engine: string; ecosystem?: string; collected_at?: string; queried_at: string; kev_date?: string; epss_date?: string };
};
export type DefenseScan = Scan;

export const isDefenseScan = (s: Pick<Scan, 'options'>): boolean => (s.options?.tools ?? []).includes('defensebot');
export const defenseJob = (s: DefenseScan | undefined) => s?.jobs?.find((j) => j.tool === 'defensebot');
export const defenseScansForAsset = (items: Scan[], assetID: string): Scan[] => items.filter((s) => isDefenseScan(s) && (s.options?.asset_ids ?? []).includes(assetID));

// orchis-defense-agent never needs a network target: a run is started for one enrolled asset and the server collects, maps and reports.
export function useLaunchDefense(organizationID: string | null | undefined, assetID: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.postIdempotent<Scan>('/api/v1/scans', { language: language(), organization_id: organizationID, asset_ids: [assetID], tools: ['defensebot'] }, crypto.randomUUID()),
    onSuccess: async () => { await qc.invalidateQueries({ queryKey: ['scans'] }); },
  });
}

// Latest orchis-defense-agent run of an asset: the scan list is only filterable per organisation, so it is filtered here.
export function useLatestDefenseScan(assetID: string, organizationID: string | null | undefined, enabled: boolean) {
  const list = useQuery({
    queryKey: ['scans', 'asset', assetID], enabled: enabled && !!organizationID, retry: false,
    queryFn: () => api.get<{ items: Scan[]; next_cursor: string | null }>(`/api/v1/scans?org=${encodeURIComponent(organizationID!)}`),
    refetchInterval: (query) => (defenseScansForAsset(query.state.data?.items ?? [], assetID)[0]?.status === 'running' ? 4000 : false),
  });
  const latest = defenseScansForAsset(list.data?.items ?? [], assetID)[0];
  const detail = useQuery({
    queryKey: ['scan', latest?.id], enabled: !!latest, retry: false,
    queryFn: () => api.get<DefenseScan>(`/api/v1/scans/${latest!.id}`),
    refetchInterval: (query) => (query.state.data?.status === 'running' ? 4000 : false),
  });
  return { list, latest, detail, scan: detail.data };
}

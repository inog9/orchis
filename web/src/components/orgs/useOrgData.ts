import { useQueries, useQuery } from '@tanstack/react-query';
import { useEffect, useState } from 'react';
import { api } from '../../api/client';

export type Org = { id: string; name: string; slug: string; description: string; industry: string; contact: string; version: number };
type Page<T> = { items: T[]; next_cursor: string | null };
export type Severity = 'critical' | 'high' | 'medium' | 'low';
export const SEVERITIES: Severity[] = ['critical', 'high', 'medium', 'low'];
export type OrgReport = { created_at: string; bySeverity: Record<string, number> } | null;

// Semua organisasi (API berhalaman kursor; satu tenant biasanya puluhan). Saring, urut, dan nomor halaman dikerjakan di klien
// agar "Showing 1–12 of 40" jujur. Dibatasi 10 halaman x 100 sebagai pengaman.
export function useAllOrganizations() {
    return useQuery({
        queryKey: ['organizations', 'all'],
        queryFn: async () => {
            const items: Org[] = [];
            let cursor = '';
            for (let i = 0; i < 10; i++) {
                const p = await api.get<Page<Org>>(`/api/v1/organizations?limit=100&cursor=${encodeURIComponent(cursor)}`);
                items.push(...p.items);
                if (!p.next_cursor) break;
                cursor = p.next_cursor;
            }
            return items;
        },
    });
}

// Laporan terbaru per organisasi (endpoint yang sama dengan halaman detail): sumber tingkat keparahan dan "pindai terakhir".
export function useOrgReports(ids: string[], enabled: boolean): Record<string, OrgReport | undefined> {
    const results = useQueries({
        queries: ids.map((id) => ({
            queryKey: ['organization-report-card', id],
            enabled,
            staleTime: 30_000,
            retry: false,
            queryFn: async (): Promise<OrgReport> => {
                const r = await api.get<{ items: { created_at: string; snapshot: { by_severity?: Record<string, number> } }[] }>(`/api/v1/reports?organization_id=${id}`);
                const first = r.items[0];
                return first ? { created_at: first.created_at, bySeverity: first.snapshot?.by_severity ?? {} } : null;
            },
        })),
    });
    const out: Record<string, OrgReport | undefined> = {};
    ids.forEach((id, i) => { out[id] = results[i]?.data; });
    return out;
}

// Sama dengan KPI /home: semua tingkat keparahan dihitung, termasuk info (tidak lagi menampilkan "0 open" saat KPI menunjukkan temuan info).
export const totalFindings = (r: OrgReport | undefined) => (r ? [...SEVERITIES, 'info'].reduce((n, s) => n + (r.bySeverity[s] ?? 0), 0) : 0);

export type Health = 'attn' | 'some' | 'ok' | 'none';
export function healthOf(r: OrgReport | undefined | null): Health {
    if (!r) return 'none';
    if ((r.bySeverity.critical ?? 0) + (r.bySeverity.high ?? 0) > 0) return 'attn';
    if ((r.bySeverity.medium ?? 0) + (r.bySeverity.low ?? 0) > 0) return 'some';
    return 'ok';
}

// Pilihan tampilan (kisi/daftar) diingat di peramban; gagal membaca/menulis tidak berpengaruh.
export function useStoredChoice<T extends string>(key: string, fallback: T, allowed: readonly T[]): [T, (v: T) => void] {
    const [value, setValue] = useState<T>(() => {
        try { const v = localStorage.getItem(key); return v && (allowed as readonly string[]).includes(v) ? (v as T) : fallback; } catch { return fallback; }
    });
    useEffect(() => { try { localStorage.setItem(key, value); } catch { /* abaikan */ } }, [key, value]);
    return [value, setValue];
}

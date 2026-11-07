import type { Finding } from '../api/types';

export const SEVERITIES: Finding['severity'][] = ['critical', 'high', 'medium', 'low', 'info'];
export type SortKey = 'severity' | 'recent' | 'frequent';
export type ViewKey = 'org' | 'asset' | 'flat';
const DAY = 86_400_000;

export const sevRank = (s: Finding['severity']) => SEVERITIES.indexOf(s);

// "Baru" = pertama kali terlihat dalam 7 hari terakhir. Tanggal tak valid tidak dianggap baru.
export function isNew(f: Pick<Finding, 'first_seen_at'>, now = Date.now()): boolean {
    const t = Date.parse(f.first_seen_at);
    return !Number.isNaN(t) && now - t <= 7 * DAY && now - t >= -DAY;
}

// API hanya menyaring satu severity dan mengurutkan menurut terakhir terlihat; sisanya dikerjakan di sini atas data yang sudah dimuat.
export function applySeverities(items: Finding[], selected: ReadonlySet<string>): Finding[] {
    return selected.size < 2 ? items : items.filter((f) => selected.has(f.severity));
}

export function sortFindings(items: Finding[], key: SortKey): Finding[] {
    const copy = [...items];
    const time = (f: Finding) => Date.parse(f.last_seen_at) || 0;
    if (key === 'severity') return copy.sort((a, b) => sevRank(a.severity) - sevRank(b.severity) || time(b) - time(a));
    if (key === 'frequent') return copy.sort((a, b) => b.occurrence_count - a.occurrence_count || time(b) - time(a));
    return copy.sort((a, b) => time(b) - time(a));
}

export type FindingGroup = { key: string; id: string | null; name: string | null; items: Finding[] };

export function groupByAsset(items: Finding[]): FindingGroup[] {
    const map = new Map<string, FindingGroup>();
    for (const f of items) {
        const key = f.asset_id ?? 'none';
        const g = map.get(key) ?? { key, id: f.asset_id, name: f.asset_name, items: [] };
        g.items.push(f);
        map.set(key, g);
    }
    return [...map.values()];
}

export function countBySeverity(items: Finding[]): Record<Finding['severity'], number> {
    const out = { critical: 0, high: 0, medium: 0, low: 0, info: 0 };
    for (const f of items) out[f.severity] += 1;
    return out;
}

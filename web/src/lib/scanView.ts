import type { Scan, ScanFinding, ScanJob } from '../api/scans';

// Durasi hanya dihitung bila scan sudah berakhir; created_at dan ended_at adalah satu-satunya cap waktu yang diberikan API.
export function durationMs(s: Pick<Scan, 'created_at' | 'ended_at'>): number | null {
    if (!s.ended_at) return null;
    const a = Date.parse(s.created_at), b = Date.parse(s.ended_at);
    return Number.isNaN(a) || Number.isNaN(b) || b < a ? null : b - a;
}

// 3mnt 12dtk -> "3m 12s" / "42s" / "1h 02m"; bahasa ditangani pemanggil lewat satuan.
export function formatDuration(ms: number | null, u: { h: string; m: string; s: string }): string {
    if (ms === null) return '—';
    const total = Math.max(0, Math.round(ms / 1000));
    const h = Math.floor(total / 3600), m = Math.floor((total % 3600) / 60), s = total % 60;
    if (h) return `${h}${u.h} ${String(m).padStart(2, '0')}${u.m}`;
    if (m) return `${m}${u.m} ${s}${u.s}`;
    return `${s}${u.s}`;
}

export type Tone = 'live' | 'ok' | 'bad' | 'warn' | 'idle';
export function statusTone(status: string): Tone {
    switch (status) {
        case 'running': case 'generating': case 'pending': return 'live';
        case 'finished': case 'done': case 'ready': return 'ok';
        case 'failed': return 'bad';
        case 'incomplete': case 'canceled': case 'skipped': return 'warn';
        default: return 'idle';
    }
}

export function jobCounts(jobs: ScanJob[] = []) {
    const c = { total: jobs.length, finished: 0, running: 0, waiting: 0, failed: 0, other: 0 };
    for (const j of jobs) {
        if (j.status === 'finished') c.finished += 1;
        else if (j.status === 'running') c.running += 1;
        else if (j.status === 'waiting') c.waiting += 1;
        else if (j.status === 'failed') c.failed += 1;
        else c.other += 1;
    }
    return c;
}

export type FlatFinding = ScanFinding & { tool: string; target: string };
export function flattenFindings(jobs: ScanJob[] = []): FlatFinding[] {
    const rank = ['critical', 'high', 'medium', 'low', 'info'];
    return jobs.flatMap((j) => (j.findings ?? []).map((f) => ({ ...f, tool: j.tool, target: j.target })))
        .sort((a, b) => rank.indexOf(a.severity) - rank.indexOf(b.severity));
}

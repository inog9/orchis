import { describe, expect, it } from 'vitest';
import type { ScanJob } from '../api/scans';
import { durationMs, flattenFindings, formatDuration, jobCounts, statusTone } from './scanView';

const u = { h: 'h', m: 'm', s: 's' };
const job = (o: Partial<ScanJob>): ScanJob => ({ id: 'j', asset_id: null, range_id: null, name: 'n', target: 't', tool: 'nmap', status: 'finished', error: '', simulation_id: null, findings: [], ...o });

describe('scanView', () => {
    it('durationMs hanya untuk scan yang berakhir dan masuk akal', () => {
        expect(durationMs({ created_at: '2026-01-01T00:00:00Z', ended_at: '2026-01-01T00:03:12Z' })).toBe(192_000);
        expect(durationMs({ created_at: '2026-01-01T00:00:00Z', ended_at: null })).toBeNull();
        expect(durationMs({ created_at: '2026-01-01T00:05:00Z', ended_at: '2026-01-01T00:00:00Z' })).toBeNull();
        expect(durationMs({ created_at: 'x', ended_at: 'y' })).toBeNull();
    });
    it('formatDuration', () => {
        expect(formatDuration(null, u)).toBe('—');
        expect(formatDuration(42_000, u)).toBe('42s');
        expect(formatDuration(192_000, u)).toBe('3m 12s');
        expect(formatDuration(3_720_000, u)).toBe('1h 02m');
    });
    it('statusTone dan jobCounts', () => {
        expect(statusTone('running')).toBe('live'); expect(statusTone('failed')).toBe('bad'); expect(statusTone('aneh')).toBe('idle');
        expect(jobCounts([job({}), job({ status: 'running' }), job({ status: 'waiting' }), job({ status: 'failed' }), job({ status: 'skipped' })])).toEqual({ total: 5, finished: 1, running: 1, waiting: 1, failed: 1, other: 1 });
    });
    it('flattenFindings mengurutkan severity dan membawa alat', () => {
        const f = (severity: string) => ({ id: severity, type: 't', value: 'v', title: severity, severity, evidence: {} });
        const out = flattenFindings([job({ tool: 'nmap', findings: [f('low')] }), job({ tool: 'nuclei', findings: [f('critical')] })]);
        expect(out.map((x) => [x.severity, x.tool])).toEqual([['critical', 'nuclei'], ['low', 'nmap']]);
    });
});

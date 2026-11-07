import { describe, expect, it } from 'vitest';
import type { Finding } from '../api/types';
import { applySeverities, countBySeverity, groupByAsset, isNew, sortFindings } from './findingsView';

const f = (o: Partial<Finding>): Finding => ({ id: 'x', asset_id: null, asset_name: null, type: 'text', value: 'v', title: 't', severity: 'info', details: {}, first_seen_at: '2026-01-01T00:00:00Z', last_seen_at: '2026-01-01T00:00:00Z', occurrence_count: 1, ...o });

describe('findingsView', () => {
    const now = Date.parse('2026-01-10T00:00:00Z');
    it('isNew: tepat 7 hari baru, lebih lama tidak, tanggal rusak tidak', () => {
        expect(isNew({ first_seen_at: '2026-01-03T00:00:00Z' }, now)).toBe(true);
        expect(isNew({ first_seen_at: '2026-01-02T23:59:00Z' }, now)).toBe(false);
        expect(isNew({ first_seen_at: 'bukan tanggal' }, now)).toBe(false);
    });
    it('applySeverities: satu pilihan diserahkan ke server, dua atau lebih disaring', () => {
        const items = [f({ id: '1', severity: 'high' }), f({ id: '2', severity: 'low' }), f({ id: '3', severity: 'info' })];
        expect(applySeverities(items, new Set(['high']))).toHaveLength(3);
        expect(applySeverities(items, new Set(['high', 'low'])).map((i) => i.id)).toEqual(['1', '2']);
    });
    it('sortFindings: severity, terbaru, paling sering; tidak mengubah input', () => {
        const items = [f({ id: 'a', severity: 'low', occurrence_count: 9, last_seen_at: '2026-01-02T00:00:00Z' }), f({ id: 'b', severity: 'critical', occurrence_count: 1, last_seen_at: '2026-01-01T00:00:00Z' })];
        expect(sortFindings(items, 'severity').map((i) => i.id)).toEqual(['b', 'a']);
        expect(sortFindings(items, 'recent').map((i) => i.id)).toEqual(['a', 'b']);
        expect(sortFindings(items, 'frequent').map((i) => i.id)).toEqual(['a', 'b']);
        expect(items[0]!.id).toBe('a');
    });
    it('groupByAsset dan countBySeverity', () => {
        const items = [f({ asset_id: 'A', asset_name: 'a', severity: 'high' }), f({ asset_id: 'A', severity: 'high' }), f({ asset_id: null })];
        expect(groupByAsset(items).map((g) => [g.key, g.items.length])).toEqual([['A', 2], ['none', 1]]);
        expect(countBySeverity(items).high).toBe(2);
    });
});

import { describe, expect, it } from 'vitest';
import { totalFindings } from './useOrgData';

describe('totalFindings', () => {
    it('menghitung info juga, sama dengan KPI total', () => {
        expect(totalFindings({ created_at: '', bySeverity: { info: 13 } })).toBe(13);
        expect(totalFindings({ created_at: '', bySeverity: { critical: 1, high: 2, medium: 3, low: 4, info: 5 } })).toBe(15);
        expect(totalFindings(undefined)).toBe(0);
    });
});

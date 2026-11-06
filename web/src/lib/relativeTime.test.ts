import { describe, expect, it } from 'vitest';
import { relativeTime } from './relativeTime';

const now = Date.parse('2026-11-06T12:00:00Z');

describe('relativeTime', () => {
  it('memilih satuan terbesar yang pas', () => {
    expect(relativeTime('2026-11-06T10:00:00Z', now)).toMatch(/2\s?h/);
    expect(relativeTime('2026-11-04T12:00:00Z', now)).toMatch(/2\s?d|2 days|2d/);
    expect(relativeTime('2026-11-06T11:58:00Z', now)).toMatch(/2\s?m/);
  });
  it('kosong atau tidak valid menghasilkan string kosong', () => {
    expect(relativeTime(null, now)).toBe('');
    expect(relativeTime('bukan tanggal', now)).toBe('');
  });
});

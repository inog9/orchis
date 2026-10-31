import { describe, expect, it } from 'vitest';
import { layoutChain, nextKey, wouldCycle } from './chainLayout';

describe('layoutChain', () => {
  it('menaruh anak di lapisan setelah semua induknya (join di lapisan paling kanan)', () => {
    const p = Object.fromEntries(layoutChain(['a', 'b', 'c', 'd'], [{ parent: 'a', child: 'b' }, { parent: 'a', child: 'c' }, { parent: 'b', child: 'd' }, { parent: 'c', child: 'd' }]).map((x) => [x.key, x]));
    expect([p.a!.layer, p.b!.layer, p.c!.layer, p.d!.layer]).toEqual([0, 1, 1, 2]);
    expect(p.b!.row).not.toBe(p.c!.row);
    expect(p.b!.x).toBeGreaterThan(p.a!.x);
  });
  it('induk dengan jalur panjang mendorong anak lebih jauh', () => {
    const p = Object.fromEntries(layoutChain(['a', 'b', 'c'], [{ parent: 'a', child: 'b' }, { parent: 'b', child: 'c' }, { parent: 'a', child: 'c' }]).map((x) => [x.key, x]));
    expect(p.c!.layer).toBe(2);
  });
  it('tidak menggantung pada siklus dan mengabaikan tepi ke langkah yang tidak ada', () => {
    expect(layoutChain(['a', 'b'], [{ parent: 'a', child: 'b' }, { parent: 'b', child: 'a' }])).toHaveLength(2);
    expect(layoutChain(['a'], [{ parent: 'zzz', child: 'a' }])[0]!.layer).toBe(0);
  });
});

describe('nextKey / wouldCycle', () => {
  it('memberi kunci unik', () => {
    expect(nextKey([])).toBe('step-1');
    expect(nextKey(['step-1', 'step-3'])).toBe('step-2');
  });
  it('mendeteksi siklus sebelum tepi ditambahkan', () => {
    const e = [{ parent: 'a', child: 'b' }, { parent: 'b', child: 'c' }];
    expect(wouldCycle(e, 'c', 'a')).toBe(true);
    expect(wouldCycle(e, 'a', 'c')).toBe(false);
    expect(wouldCycle(e, 'a', 'a')).toBe(true);
  });
});

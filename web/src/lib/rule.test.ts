import { describe, expect, it } from 'vitest';
import { changeField, changeOperator, cleanRule, describeRule, FIELDS, newFilter } from './rule';

describe('rule helpers', () => {
  it('filter baru memakai nilai awal yang sah untuk kolomnya', () => {
    expect(newFilter('platform')).toEqual({ field: 'platform', operator: 'eq', values: ['windows'] });
    expect(newFilter('is_internet_facing').values).toEqual(['true']);
    expect(newFilter('tag').values).toEqual(['']);
  });

  it('mengganti kolom membuang nilai yang tidak lagi sah', () => {
    const f = { field: 'platform', operator: 'in', values: ['windows', 'linux'] };
    const g = changeField(f, 'criticality');
    expect(g.field).toBe('criticality');
    expect(g.operator).toBe('eq');
    expect(g.values).toEqual(['low']);
    expect(changeField(f, 'platform')).toBe(f);
  });

  it('mengganti operator menyesuaikan jumlah nilai', () => {
    const f = { field: 'tag', operator: 'eq', values: ['prod'] };
    expect(changeOperator(f, 'is_empty').values).toEqual([]);
    expect(changeOperator(f, 'in').values).toEqual(['prod']);
    expect(changeOperator({ ...f, operator: 'in', values: ['a', 'b'] }, 'eq').values).toEqual(['a']);
  });

  it('operator per kolom sejalan dengan yang dipakai server', () => {
    expect(FIELDS.is_internet_facing!.operators).toEqual(['eq', 'neq']);
    expect(FIELDS.hostname!.operators).toContain('contains');
    expect(FIELDS.platform!.operators).not.toContain('contains');
  });

  it('menjelaskan aturan dalam kalimat', () => {
    expect(describeRule({ mode: 'and', filters: [] })).toBe('Every asset');
    expect(describeRule({ mode: 'and', filters: [{ field: 'platform', operator: 'eq', values: ['macos'] }, { field: 'arch', operator: 'eq', values: ['arm64'] }] }))
      .toBe('Platform is macOS  AND  Architecture is arm64');
    expect(describeRule({ mode: 'or', filters: [{ field: 'tag', operator: 'is_empty', values: [] }, { field: 'criticality', operator: 'in', values: ['high', 'critical'] }] }))
      .toBe('Tag is empty  OR  Criticality is any of High, Critical');
  });

  it('cleanRule membuang spasi dan menolak nilai kosong', () => {
    expect(cleanRule({ mode: 'and', filters: [{ field: 'tag', operator: 'in', values: [' a ', '', 'b'] }] }).rule.filters[0]!.values).toEqual(['a', 'b']);
    expect(cleanRule({ mode: 'and', filters: [{ field: 'hostname', operator: 'eq', values: ['  '] }] }).problem).toMatch(/Hostname/);
    expect(cleanRule({ mode: 'and', filters: [{ field: 'tag', operator: 'is_not_empty', values: ['sisa'] }] }).rule.filters[0]!.values).toEqual([]);
  });
});

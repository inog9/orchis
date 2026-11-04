import { describe, expect, it } from 'vitest';
import { groupByOrganization } from './groupByOrganization';

const row = (id: string, org: string | null, name: string | null = null) => ({ id, organization_id: org, organization_name: name });

describe('groupByOrganization', () => {
  it('mengelompokkan per organisasi, urut nama, tanpa organisasi di akhir', () => {
    const g = groupByOrganization([row('1', null), row('2', 'b', 'Bravo'), row('3', 'a', 'Alpha'), row('4', 'b', 'Bravo')]);
    expect(g.map((x) => x.name)).toEqual(['Alpha', 'Bravo', null]);
    expect(g[1]!.items.map((x) => x.id)).toEqual(['2', '4']);
    expect(g[2]!.id).toBeNull();
  });
  it('daftar kosong tidak menghasilkan kelompok', () => {
    expect(groupByOrganization([])).toEqual([]);
  });
});

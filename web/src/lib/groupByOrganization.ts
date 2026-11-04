export type OrgGroup<T> = { key: string; id: string | null; name: string | null; items: T[] };

type InOrg = { organization_id?: string | null; organization_name?: string | null };

// Mengelompokkan daftar yang sudah dimuat per organisasi. Organisasi bernama diurutkan menurut nama; yang tanpa organisasi
// selalu terakhir. Urutan item di dalam kelompok mengikuti urutan masukan.
export function groupByOrganization<T extends InOrg>(items: T[]): OrgGroup<T>[] {
    const byKey = new Map<string, OrgGroup<T>>();
    for (const item of items) {
        const id = item.organization_id ?? null;
        const key = id ?? 'none';
        let g = byKey.get(key);
        if (!g) {
            g = { key, id, name: id ? (item.organization_name ?? null) : null, items: [] };
            byKey.set(key, g);
        }
        g.items.push(item);
    }
    return [...byKey.values()].sort((a, b) => {
        if (!a.id !== !b.id) return a.id ? -1 : 1;
        return (a.name ?? '').localeCompare(b.name ?? '');
    });
}

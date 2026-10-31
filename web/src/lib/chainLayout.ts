import type { ChainEdge } from '../api/types';

export const CARD_W = 232;
export const CARD_H = 92;
export const GAP_X = 64;
export const GAP_Y = 24;

export type Placed = { key: string; layer: number; row: number; x: number; y: number };

// Menyusun langkah dalam lapisan: lapisan = jarak terpanjang dari akar. Langkah yang saling bergantung selalu ada di lapisan
// berbeda, sehingga panah selalu mengarah ke kanan. Tahan siklus (langkah dalam siklus ditaruh di lapisan terakhir yang bisa dihitung).
export function layoutChain(keys: string[], edges: ChainEdge[]): Placed[] {
  const parents = new Map<string, string[]>();
  for (const k of keys) parents.set(k, []);
  for (const e of edges) if (parents.has(e.child) && parents.has(e.parent)) parents.get(e.child)!.push(e.parent);

  const layer = new Map<string, number>();
  const visiting = new Set<string>();
  const depth = (k: string): number => {
    const known = layer.get(k);
    if (known !== undefined) return known;
    if (visiting.has(k)) return 0;
    visiting.add(k);
    const d = Math.max(-1, ...parents.get(k)!.map(depth)) + 1;
    visiting.delete(k);
    layer.set(k, d);
    return d;
  };
  keys.forEach(depth);

  const rowOf = new Map<number, number>();
  return keys.map((key) => {
    const l = layer.get(key) ?? 0;
    const row = rowOf.get(l) ?? 0;
    rowOf.set(l, row + 1);
    return { key, layer: l, row, x: l * (CARD_W + GAP_X), y: row * (CARD_H + GAP_Y) };
  });
}

export function nextKey(existing: string[]): string {
  for (let i = 1; ; i++) {
    const k = `step-${i}`;
    if (!existing.includes(k)) return k;
  }
}

// Apakah menambah tepi parent -> child membentuk siklus? (child sudah mencapai parent)
export function wouldCycle(edges: ChainEdge[], parent: string, child: string): boolean {
  if (parent === child) return true;
  const out = new Map<string, string[]>();
  for (const e of edges) out.set(e.parent, [...(out.get(e.parent) ?? []), e.child]);
  const stack = [child];
  const seen = new Set<string>();
  while (stack.length) {
    const k = stack.pop()!;
    if (k === parent) return true;
    if (seen.has(k)) continue;
    seen.add(k);
    stack.push(...(out.get(k) ?? []));
  }
  return false;
}

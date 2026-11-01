// Mengubah komponen "Raven Skull Mark" di design/orchis.pen menjadi data path SVG.
import { readFileSync, writeFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const pen = JSON.parse(readFileSync(path.join(root, '..', 'design', 'orchis.pen'), 'utf8'));
const find = (n, id) => (n.id === id ? n : (n.children ?? []).map((c) => find(c, id)).find(Boolean));
const raven = pen.children.map((t) => find(t, 'e5wJS')).find(Boolean);
const paths = (raven.children ?? [])
  .filter((c) => c.type === 'path' && c.fill && c.geometry)
  .map((c) => ({ x: c.x, y: c.y, fill: c.fill, d: c.geometry }));
const src = `// DIHASILKAN dari design/orchis.pen oleh scripts/extract-raven.mjs. Jangan diubah manual.
export const RAVEN_SIZE = ${raven.width};
export const RAVEN_PATHS: ReadonlyArray<{ x: number; y: number; fill: string; d: string }> = ${JSON.stringify(paths)};
`;
writeFileSync(path.join(root, 'src', 'assets', 'raven-paths.ts'), src);
console.log(`ditulis src/assets/raven-paths.ts (${paths.length} path)`);

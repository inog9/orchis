// Mengekstrak variabel desain dari design/orchis.pen menjadi src/styles/tokens.css.
// Warna dan font TIDAK boleh diketik manual di CSS; ubah di file desain lalu jalankan `npm run tokens`.
import { readFileSync, writeFileSync, existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const pen = JSON.parse(readFileSync(path.join(root, '..', 'design', 'orchis.pen'), 'utf8'));
const out = path.join(root, 'src', 'styles', 'tokens.css');

const stacks = {
  'Space Grotesk': '"Space Grotesk Variable", "Space Grotesk", system-ui, sans-serif',
  Inter: '"Inter Variable", Inter, system-ui, sans-serif',
  'IBM Plex Mono': '"IBM Plex Mono", ui-monospace, SFMono-Regular, Menlo, monospace',
};

const lines = ['/* DIHASILKAN dari design/orchis.pen oleh scripts/extract-tokens.mjs. Jangan diubah manual. */', ':root {'];
for (const [name, v] of Object.entries(pen.variables)) {
  const value = v.type === 'string' ? (stacks[v.value] ?? v.value) : v.value;
  lines.push(`  --${name}: ${value};`);
}
lines.push(`  --font-mono: ${stacks['IBM Plex Mono']};`, '  color-scheme: dark;', '}', '');
const css = lines.join('\n');

if (process.argv.includes('--check')) {
  const current = existsSync(out) ? readFileSync(out, 'utf8') : '';
  if (current !== css) {
    console.error('tokens.css tidak sinkron dengan design/orchis.pen. Jalankan: npm run tokens');
    process.exit(1);
  }
  console.log('tokens.css sinkron dengan design/orchis.pen');
} else {
  writeFileSync(out, css);
  console.log(`ditulis ${path.relative(root, out)} (${Object.keys(pen.variables).length} variabel)`);
}

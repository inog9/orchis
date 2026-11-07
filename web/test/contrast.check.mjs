import {test} from 'node:test';
import assert from 'node:assert/strict';
const expect = (value, message) => ({toBeGreaterThanOrEqual: minimum => assert.ok(value >= minimum, `${message ?? 'contrast'}: ${value.toFixed(2)} < ${minimum}`)});
import {readFileSync} from 'node:fs';
const palette = readFileSync(new URL('../src/styles/ui2.css', import.meta.url), 'utf8');
const legacy = readFileSync(new URL('../src/styles/tokens.css', import.meta.url), 'utf8');
const rgb = (hex) => hex.slice(1, 7).match(/../g).map(x => parseInt(x, 16) / 255);
const luminance = (c) => c.reduce((sum, x, i) => sum + [0.2126, 0.7152, 0.0722][i] * (x <= .04045 ? x / 12.92 : ((x + .055) / 1.055) ** 2.4), 0);
const contrast = (a, b) => (Math.max(luminance(a), luminance(b)) + .05) / (Math.min(luminance(a), luminance(b)) + .05);
const tokens = (block) => Object.fromEntries([...block.matchAll(/(--[\w-]+):\s*(#[\da-f]{6,8})/gi)].map(m => [m[1], m[2]]));
const blocks = [...palette.matchAll(/([^{}]+)\{([^{}]+)\}/g)];
const dark = tokens(blocks.find(m => m[1].trim().endsWith(':root'))[2]);
const light = {...dark, ...tokens(blocks.find(m => m[1].trim() === ":root[data-theme='light']")[2])};
for (const [theme, colors] of [['dark', dark], ['light', light]]) {
  const surfaces = ['--v2-bg', '--v2-surface', '--v2-surface-2'];
  for (const text of ['--v2-text', '--v2-text-2', '--v2-text-3']) test(`${theme} ${text} stays readable on each surface`, () => {
    for (const surface of surfaces) expect(contrast(rgb(colors[text]), rgb(colors[surface])), `${text} on ${surface}`).toBeGreaterThanOrEqual(4.5);
  });
  test(`${theme} keeps status text readable on its translucent badge background`, () => {
    for (const name of ['success', 'critical', 'high', 'medium', 'low', 'info', 'accent']) {
      const foreground = rgb(colors[`--v2-${name}`]);
      const hex = colors[`--v2-${name}-soft`];
      const alpha = parseInt(hex.slice(7), 16) / 255;
      for (const surface of surfaces) {
        const background = rgb(hex).map((x, i) => x * alpha + rgb(colors[surface])[i] * (1 - alpha));
        expect(contrast(foreground, background), `${name} badge on ${surface}`).toBeGreaterThanOrEqual(4.5);
      }
    }
  });
  test(`${theme} distinguishes controls and focus indicators from surfaces`, () => {
    for (const surface of surfaces) {
      expect(contrast(rgb(colors['--v2-control-border']), rgb(colors[surface]))).toBeGreaterThanOrEqual(3);
      expect(contrast(rgb(colors['--v2-accent']), rgb(colors[surface]))).toBeGreaterThanOrEqual(3);
    }
  });
  test(`${theme} keeps primary button labels readable`, () => {
    expect(contrast(rgb(colors['--v2-accent-ink']), rgb(colors['--v2-accent']))).toBeGreaterThanOrEqual(4.5);
  });
}
test('keeps the pinned dark login palette readable', () => {
  const colors = tokens(legacy);
  for (const text of ['--ink', '--ink-2', '--ink-3']) {
    for (const surface of ['--bg', '--input-bg', '--surface-1']) expect(contrast(rgb(colors[text]), rgb(colors[surface]))).toBeGreaterThanOrEqual(4.5);
  }
});

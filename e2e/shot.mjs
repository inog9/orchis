import { chromium } from 'playwright';
const b = await chromium.launch();
const p = await b.newPage({ viewport: { width: 1440, height: 900 } });
await p.goto('http://127.0.0.1:8090/login');
await p.waitForSelector('h1');
await p.waitForTimeout(500);
await p.screenshot({ path: 'shots/login.png' });
const m = await p.evaluate(() => {
  const r = (s) => { const e = document.querySelector(s); if (!e) return null; const b = e.getBoundingClientRect(); const c = getComputedStyle(e); return { w: Math.round(b.width), h: Math.round(b.height), x: Math.round(b.x), y: Math.round(b.y), fs: c.fontSize, fw: c.fontWeight, ff: c.fontFamily.slice(0, 30), color: c.color, bg: c.backgroundColor, br: c.borderRadius }; };
  return { brand: r('.auth__brand'), title: r('.title'), eyebrow: r('.eyebrow'), subtitle: r('.subtitle'), column: r('.auth__column'), input: r('.field__input'), btn: r('.btn--primary'), headline: r('.auth__headline'), orbit: r('.orbit'), raven: r('.orbit__raven'), fonts: [...document.fonts].filter(f => f.status === 'loaded').map(f => f.family) };
});
console.log(JSON.stringify(m, null, 1));
await b.close();

// Halaman cetak mandiri: tidak ada JavaScript target atau akses jaringan saat render.
import { createRequire } from 'node:module';
const require = createRequire(import.meta.url);
let chromium;
try { ({chromium} = require('playwright')); } catch { ({chromium} = require('../../e2e/node_modules/playwright')); }
let html = '';
for await (const chunk of process.stdin) { html += chunk; if (Buffer.byteLength(html) > 16 * 1024 * 1024) throw new Error('Report input exceeds limit'); }
const browser = await chromium.launch();
try {
  const context = await browser.newContext({ javaScriptEnabled: false, serviceWorkers: 'block', offline: true });
  await context.route('**/*', route => route.abort('blockedbyclient'));
  const page = await context.newPage();
  await page.setContent(html, { waitUntil: 'load', timeout: 15000 });
  await page.emulateMedia({ media: 'print' });
  const marker=page.locator('meta[name="orchis-document-type"]');
  const documentType=await marker.count()?await marker.getAttribute('content'):'';
  const caption=documentType==='ai-advice'?'ORCHIS | AI advisory supplement - review required':documentType==='demo-assessment'?'ORCHIS | DEMO / SYNTHETIC report - not a real scan':'ORCHIS | Frozen organisation report';
  const pdf = await page.pdf({ format: 'A4', printBackground: true, preferCSSPageSize: true, tagged: true, displayHeaderFooter: true, headerTemplate: '<span></span>', footerTemplate: `<div style="width:100%;margin:0 20mm;font:9px Arial,sans-serif;color:#52606e;display:flex;justify-content:space-between"><span>${caption}</span><span><span class="pageNumber"></span> / <span class="totalPages"></span></span></div>`  });
  process.stdout.write(pdf);
} finally { await browser.close(); }

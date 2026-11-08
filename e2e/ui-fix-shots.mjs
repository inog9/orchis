import { chromium } from 'playwright';
const POP='http://127.0.0.1:18190', OUT='/tmp/uifix';
const b=await chromium.launch(); const ctx=await b.newContext({viewport:{width:1440,height:900}});
const page=await ctx.newPage();
await page.goto(POP+'/login',{waitUntil:'networkidle'});
await page.locator('input[type=email]').first().fill('admin@orchis.local'); await page.locator('input[type=password]').first().fill('Orchis-Dev-Pass-1'); await page.keyboard.press('Enter'); await page.waitForTimeout(2000);
await ctx.request.patch(POP+'/api/v1/auth/preferences',{headers:{'X-Requested-With':'orchis-web'},data:{ui_language:'en'}});
const api=async p=>(await ctx.request.get(POP+'/api/v1'+p,{headers:{'X-Requested-With':'orchis-web'}})).json();
const assets=(await api('/assets?limit=100')).items; const a=assets.find(x=>x.category==='web_application')||assets[0];
const rep=(await api('/reports')).items[0]; const scan=((await api('/scans')).items||[])[0]; const grp=(await api('/asset-groups')).items?.[0]; const org=(await api('/organizations')).items[0];
const T=[['asset','/assets/'+a.id],['report','/reports/'+rep.id],['group','/assets/groups/'+grp.id],['org','/organizations/'+org.id+'?tab=overview']]; if(scan) T.push(['scan','/simulations/'+scan.id]);
for (const [n,p] of T){await page.goto(POP+p,{waitUntil:'networkidle'}); await page.waitForTimeout(700); await page.screenshot({path:OUT+'/'+n+'.png',fullPage:false});}
await b.close();

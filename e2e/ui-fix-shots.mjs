import { chromium } from 'playwright';
const POP='http://127.0.0.1:18190', OUT='/tmp/uifix';
const b=await chromium.launch(); const ctx=await b.newContext({viewport:{width:1440,height:900}});
const page=await ctx.newPage();
await page.addInitScript(()=>{try{localStorage.setItem('orchis.lang','en')}catch{}});
await page.goto(POP+'/login',{waitUntil:'networkidle'}); await page.waitForTimeout(2500);
await page.screenshot({path:OUT+'/login.png'});
await page.locator('input[type=email],input[name=email]').first().focus(); await page.waitForTimeout(400);
await page.screenshot({path:OUT+'/login-focus.png'});
await page.locator('input[type=email],input[name=email]').first().fill('admin@orchis.local');
await page.locator('input[type=password]').first().fill('Orchis-Dev-Pass-1');
await page.keyboard.press('Enter'); await page.waitForTimeout(2000);
await ctx.request.patch(POP+'/api/v1/auth/preferences',{headers:{'X-Requested-With':'orchis-web'},data:{ui_language:'en'}});
const api=async p=>(await ctx.request.get(POP+'/api/v1'+p,{headers:{'X-Requested-With':'orchis-web'}})).json();
const a=(await api('/assets')).items[0];
for (const [n,p] of [['orgs','/organizations'],['asset','/assets/'+a.id],['newscan','/simulations/new']]) { await page.goto(POP+p,{waitUntil:'networkidle'}); await page.waitForTimeout(600); await page.screenshot({path:OUT+'/'+n+'.png'}); }
await b.close();

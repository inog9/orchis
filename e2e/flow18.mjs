// Regresi kegagalan CSS: pulih sebelum workspace dipasang, atau tampilkan pemulihan yang terbaca.
import {chromium} from 'playwright';
import AxeBuilder from '@axe-core/playwright';
import {mkdirSync,writeFileSync} from 'node:fs';
const base=process.env.E2E_BASE??'http://127.0.0.1:18090';
const out=new URL('./shots/style-recovery/',import.meta.url).pathname;mkdirSync(out,{recursive:true});
const browser=await chromium.launch();let pass=0;
const check=(value,label)=>{if(!value)throw new Error(label);pass++;console.log('LULUS: '+label)};
const isCSS=r=>new URL(r.request().url()).pathname.endsWith('.css');
try{
 for(const width of [1280,834,390]){
  const ctx=await browser.newContext({viewport:{width,height:900}});const page=await ctx.newPage();let attempts=0;const errors=[];
  page.on('pageerror',e=>errors.push(String(e)));
  await page.route('**/*',route=>{if(isCSS(route)&&++attempts===1)return route.fulfill({status:503,contentType:'text/plain',body:'Controlled stylesheet failure'});return route.continue()});
  await page.goto(base+'/login');await page.getByLabel('Email').fill('admin@orchis.local');await page.getByLabel('Password',{exact:true}).fill('Orchis-Dev-Pass-1');await page.getByRole('button',{name:/sign in/i}).click();await page.getByRole('heading',{name:'Organisations',exact:true}).waitFor();
  check(attempts===2,`one failed stylesheet recovered with one retry @${width}`);
  check(await page.evaluate(()=>getComputedStyle(document.documentElement).getPropertyValue('--orchis-ui-styled').trim()==='1'),`workspace has applied styles @${width}`);
  const axe=await new AxeBuilder({page}).analyze();writeFileSync(out+`axe-${width}.json`,JSON.stringify(axe.violations,null,2));
  check(axe.violations.length===0,`recovered empty organisations axe0 @${width}`);
  check(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),`no horizontal overflow @${width}`);
  check(errors.length===0,`no browser runtime errors @${width}`);
  await page.screenshot({path:out+`recovered-${width}.png`,fullPage:true});await ctx.close();
 }
 const ctx=await browser.newContext({viewport:{width:1280,height:900}});const page=await ctx.newPage();let attempts=0,authRequests=0;
 page.on('request',r=>{if(new URL(r.url()).pathname==='/api/v1/auth/me')authRequests++});
 await page.route('**/*',route=>{if(isCSS(route)){attempts++;return route.fulfill({status:503,contentType:'text/plain',body:'Controlled persistent stylesheet failure'})}return route.continue()});
 await page.goto(base+'/login');await page.getByRole('heading',{name:'Couldn’t load the application',exact:true}).waitFor();
 check(attempts===3,'persistent CSS failure stops after two retries');
 check(authRequests===0,'application does not mount or request session before styles are ready');
 for(const width of [1280,834,390]){
  await page.setViewportSize({width,height:900});const axe=await new AxeBuilder({page}).analyze();writeFileSync(out+`axe-fallback-${width}.json`,JSON.stringify(axe.violations,null,2));check(axe.violations.length===0,`standalone recovery screen axe0 @${width}`);
  check(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),`recovery screen no horizontal overflow @${width}`);
  await page.screenshot({path:out+`unavailable-${width}.png`,fullPage:true});
 }
 await page.unrouteAll({behavior:'wait'});await page.getByRole('button',{name:'Reload application',exact:true}).click();await page.getByLabel('Email').waitFor();
 check(await page.evaluate(()=>getComputedStyle(document.documentElement).getPropertyValue('--orchis-ui-styled').trim()==='1'),'manual reload recovers once stylesheet delivery returns');
 await ctx.close();
 const clockCtx=await browser.newContext();const clockPage=await clockCtx.newPage();const cssURLs=[];
 await clockPage.addInitScript(()=>{Date.now=()=>1700000000000});
 await clockPage.route('**/*',route=>{if(isCSS(route)){cssURLs.push(route.request().url());if(cssURLs.length<=2)return route.fulfill({status:503,contentType:'text/plain',body:'Controlled stylesheet failure'})}return route.continue()});
 await clockPage.goto(base+'/login');await clockPage.getByLabel('Email').waitFor();
 check(cssURLs.length===3,'second bounded retry recovers after two stylesheet failures');
 check(new Set(cssURLs).size===3,'cache retry URLs remain distinct with a fixed clock');
 await clockCtx.close();
}finally{await browser.close()}
console.log(`Stylesheet recovery: ${pass} passed`);

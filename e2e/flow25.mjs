// A-2.6: persisted account language, browser fallback, real local scan and frozen bilingual PDF.
import assert from 'node:assert/strict';
import {createServer} from 'node:http';
import {mkdirSync,writeFileSync} from 'node:fs';
import {createRequire} from 'node:module';
import {chromium} from 'playwright';
import AxeBuilder from '@axe-core/playwright';
const require=createRequire(import.meta.url);const parsePDF=require('../deploy/report/node_modules/pdf-parse/lib/pdf-parse.js');
const BASE=process.env.E2E_BASE;if(!BASE)throw Error('Requires isolated flow25 wrapper');
const port=Number(process.env.E2E_TARGET_PORT)+4;let checks=0;
const fixture=createServer((req,res)=>{const timer=setTimeout(()=>res.end(req.url==='/.env'?'E2E_SECRET_MARKER=local-fixture':'bilingual local fixture'),1500);timer.unref();res.on('close',()=>clearTimeout(timer));});await new Promise(resolve=>fixture.listen(port,'127.0.0.1',resolve));
const browser=await chromium.launch();const context=await browser.newContext({locale:'id-ID',viewport:{width:1280,height:900}});const page=await context.newPage();const errors=[];page.on('pageerror',e=>errors.push(e.message));
const shots='e2e/shots/language';mkdirSync(shots,{recursive:true});
const headers={'X-Requested-With':'orchis-web'};const check=(value,message)=>{assert(value,message);checks++;console.log('LULUS: '+message)};
const api=async(method,path,data,extra={})=>{const r=await context.request.fetch(BASE+'/api/v1'+path,{method,data,headers:{...headers,...extra}});assert(r.ok(),`${method} ${path}: ${r.status()}`);return r.status()===204?null:r.json()};
const wait=async(fn,ms=120000)=>{const until=Date.now()+ms;while(Date.now()<until){const value=await fn();if(value)return value;await new Promise(resolve=>setTimeout(resolve,250))}throw Error('Language fixture timed out')};
async function choose(language){await page.locator('#language-preference').selectOption(language);await wait(async()=> (await api('GET','/auth/me')).ui_language===language);await page.waitForFunction(l=>document.documentElement.lang===l,language);}
async function audit(name){for(const width of [1280,390]){await page.setViewportSize({width,height:900});const axe=await new AxeBuilder({page}).withTags(['wcag2a','wcag2aa','wcag21a','wcag21aa','wcag22aa','best-practice']).analyze();check(axe.violations.length===0,`${name}: axe0 @${width}: ${axe.violations.map(v=>v.id)}`);check(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),`${name}: no overflow @${width}`);await page.screenshot({path:`${shots}/${name}-${width}.png`,fullPage:true})}await page.setViewportSize({width:1280,height:900});}
try{
 await page.goto(BASE+'/login');await page.getByRole('heading',{name:'Masuk ke ORCHIS'}).waitFor();check(await page.locator('html').getAttribute('lang')==='id','browser Indonesian is default before any saved preference');
 await page.getByLabel('Email',{exact:true}).fill('admin@orchis.local');await page.getByLabel('Kata sandi',{exact:true}).fill('Orchis-Dev-Pass-1');await page.getByRole('button',{name:'Masuk',exact:true}).click();await page.waitForURL(BASE+'/organizations');await page.getByRole('heading',{name:'Organisasi',exact:true}).waitFor();
 await choose('en');await page.getByRole('heading',{name:'Organisations',exact:true}).waitFor();await page.reload();await page.getByRole('heading',{name:'Organisations',exact:true}).waitFor();check(await page.locator('#language-preference').inputValue()==='en','saved English survives reload despite Indonesian browser');
 const org=await api('POST','/organizations',{name:'Language laboratory',slug:'language-laboratory'});const asset=await api('POST','/assets',{name:'Bilingual target',category:'host',organization_id:org.id,testing_authorized:true,addresses:[{kind:'url',value:`http://127.0.0.1:${port}`} ]});
 await page.goto(BASE+`/organizations/${org.id}`);await page.getByRole('heading',{name:org.name,exact:true}).waitFor();await audit('organisation-en');await choose('id');await audit('organisation-id');
 await page.goto(BASE+`/simulations/new?org=${org.id}`);await page.getByRole('heading',{name:'Scan baru',exact:true}).waitFor();await page.getByRole('checkbox',{name:/Bilingual target/}).check();await page.locator('label.check').filter({has:page.getByRole('checkbox',{name:/^Nmap/})}).click();assert.equal(await page.getByRole('checkbox',{name:/^Nmap/}).isChecked(),false);await audit('scan-form-id');
 await page.getByRole('button',{name:'Jalankan scan',exact:true}).click();await page.waitForURL(/\/simulations\/[0-9a-f-]+$/);const scanID=page.url().split('/').at(-1);await choose('en');
 const report=await wait(async()=>{const reports=await api('GET',`/reports?assessment_id=${scanID}`);return reports.items[0]});check(report.snapshot.language==='id','UI launch freezes Indonesian before later account preference changes');
 const snap=await api('GET',`/reports/${report.id}/export/json`);check(snap.language==='id'&&snap.organization===org.name&&'by_severity' in snap&&!('organisasi' in snap),'JSON keeps English keys and original names');
 await wait(async()=>{const r=await api('GET',`/reports/${report.id}`);if(r.status==='failed')throw Error('PDF failed: '+r.error);return r.status==='ready'});
 const pdf=await context.request.get(BASE+`/api/v1/reports/${report.id}/export/pdf`);assert(pdf.ok());const bytes=await pdf.body();writeFileSync(`${shots}/report-id.pdf`,bytes);const content=(await parsePDF(bytes)).text;check(content.includes('Ringkasan eksekutif')&&content.includes('Cakupan dan pemeriksaan'),'real PDF uses frozen Indonesian labels');
 await page.goto(BASE+`/reports/${report.id}`);await page.getByRole('heading',{name:org.name,exact:true}).waitFor();await audit('report-en');await choose('id');await audit('report-id');await choose('en');
 const again=await api('GET',`/reports/${report.id}/export/json`);check(JSON.stringify(again)===JSON.stringify(snap),'language changes never mutate the frozen snapshot');const pdfAgain=await context.request.get(BASE+`/api/v1/reports/${report.id}/export/pdf`);check(bytes.equals(await pdfAgain.body()),'stored PDF is unchanged after preference changes');

 const second=await api('POST','/scans',{language:'en',organization_id:org.id,asset_ids:[asset.id],tools:['nuclei'],preset:'quick'},{'Idempotency-Key':'language-english'});
 const english=await wait(async()=>{const list=await api('GET',`/reports?assessment_id=${second.id}`);return list.items[0]?.status==='ready'?list.items[0]:null});
 const enPDF=await context.request.get(BASE+`/api/v1/reports/${english.id}/export/pdf`);assert(enPDF.ok());const enBytes=await enPDF.body();writeFileSync(`${shots}/report-en.pdf`,enBytes);const enText=(await parsePDF(enBytes)).text;check(english.snapshot.language==='en'&&enText.includes('Executive summary')&&enText.includes('Scope and coverage'),'second real scan produces frozen English PDF');
 await page.goto(BASE+'/profile');await page.getByRole('heading',{name:'Profile',exact:true}).waitFor();await audit('profile-en');await choose('id');await page.getByRole('heading',{name:'Profil',exact:true}).waitFor();await audit('profile-id');
 await api('POST','/auth/logout',{});await page.goto(BASE+'/login');await page.getByLabel('Email',{exact:true}).fill('admin@orchis.local');await page.getByLabel('Kata sandi',{exact:true}).fill('Orchis-Dev-Pass-1');await page.getByRole('button',{name:'Masuk',exact:true}).click();await page.waitForURL(BASE+'/organizations');await page.getByRole('heading',{name:'Organisasi',exact:true}).waitFor();check((await api('GET','/auth/me')).ui_language==='id','saved Indonesian survives logout and login');
 check(errors.length===0,'no browser runtime errors: '+errors.join('; '));
}finally{fixture.closeAllConnections();fixture.close();await browser.close()}
console.log(`Language: ${checks} passed`);

import {settleUI} from './settle-ui.mjs';
import {startPentestFixture} from './pentest-fixture.mjs';
// A-2.9: genuine Nuclei checks of two loopback fixtures, frozen deltas and exports.
import assert from 'node:assert/strict';
import {createServer} from 'node:http';
import {mkdirSync,writeFileSync} from 'node:fs';
import {createRequire} from 'node:module';
import {chromium} from 'playwright';
import AxeBuilder from '@axe-core/playwright';
const require=createRequire(import.meta.url);const parsePDF=require('../deploy/report/node_modules/pdf-parse/lib/pdf-parse.js');
const BASE=process.env.E2E_BASE??'http://127.0.0.1:18090';const port=Number(process.env.E2E_TARGET_PORT??18081)+4;const shots='e2e/shots/scan-comparison';mkdirSync(shots,{recursive:true});let exposed=true,slow=false,checks=0;
const fixture=()=>createServer((req,res)=>{const reply=()=>res.end(exposed&&req.url==='/.env'?'E2E_SECRET_MARKER=local-fixture':'local comparison fixture');if(slow&&req.url==='/.env'){const timer=setTimeout(reply,20000);timer.unref();res.on('close',()=>clearTimeout(timer));}else reply()});
const servers=[fixture(),fixture()];for(let i=0;i<2;i++)await new Promise(r=>servers[i].listen(port+i,'127.0.0.1',r));
const browser=await chromium.launch();const context=await browser.newContext({viewport:{width:1280,height:900}});const page=await context.newPage();const errors=[];page.on('pageerror',e=>errors.push(e.message));
const check=(v,m)=>{assert(v,m);checks++;console.log('LULUS: '+m)};const headers={'X-Requested-With':'orchis-web'};
const api=async(method,path,data,extra={})=>{const r=await context.request.fetch(BASE+'/api/v1'+path,{method,data,headers:{...headers,...extra}});assert(r.ok(),`${method} ${path}: ${r.status()} ${await r.text()}`);return r.status()===204?null:r.json()};
const wait=async(fn,ms=120000)=>{const until=Date.now()+ms;while(Date.now()<until){const v=await fn();if(v)return v;await new Promise(r=>setTimeout(r,300))}throw Error('comparison fixture timed out')};
async function audit(name){for(const width of [1280,834,390]){await page.setViewportSize({width,height:900});await settleUI(page);const axe=await new AxeBuilder({page}).withTags(['wcag2a','wcag2aa','wcag21a','wcag21aa','wcag22aa','best-practice']).analyze();check(axe.violations.length===0,`${name}: axe0 @${width}: ${JSON.stringify(axe.violations.map(v=>({id:v.id,nodes:v.nodes.map(n=>({target:n.target,summary:n.failureSummary}))})))}`);check(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),`${name}: no overflow @${width}`);await page.screenshot({path:`${shots}/${name}-${width}.png`,fullPage:true})}}
let reviewer;
try{
 await page.goto(BASE+'/login');await page.getByLabel('Email').fill('admin@orchis.local');await page.getByLabel('Password',{exact:true}).fill('Orchis-Dev-Pass-1');await page.getByRole('button',{name:'Sign in',exact:true}).click();await page.waitForURL(BASE+'/overview');await page.goto(BASE+'/organizations');
 const org=await api('POST','/organizations',{name:'Comparison laboratory',slug:'comparison-laboratory'});const assets=[];
 for(let i=0;i<2;i++)assets.push(await api('POST','/assets',{name:`Comparison target ${i+1}`,category:'host',organization_id:org.id,testing_authorized:true,addresses:[{kind:'url',value:`http://127.0.0.1:${port+i}`}]}));
 reviewer=await startPentestFixture(api);
 let sequence=0;const launch=()=>api('POST','/scans',{organization_id:org.id,asset_ids:assets.map(a=>a.id),tools:['nmap','nuclei','hackbot'],preset:'quick'},{'Idempotency-Key':`comparison-${++sequence}`});
 const finish=async scan=>wait(async()=>{const s=await api('GET',`/scans/${scan.id}`);return s.status!=='running'&&s.comparison_details?s:null});
 const first=await finish(await launch());check(first.comparison_details.by_severity.high.new===2,'first real Nuclei observations classified new');check(first.comparison_details.by_severity.high.new===2,'new count by severity');
 const second=await finish(await launch());check(second.comparison_details.by_severity.high.persistent===2&&second.comparison_details.by_severity.high.new===0,'second real scan keeps both findings still present');
 exposed=false;servers[1].closeAllConnections();await new Promise(r=>servers[1].close(r));
 const third=await finish(await launch());check(third.jobs.some(j=>j.status==='failed'),'unreachable fixture is failed');check(third.comparison_details.by_severity.high.resolved===1&&third.comparison_details.by_severity.high.uncompared===1,'partial scan resolves only completed target');check(third.comparison_details.by_severity.high.resolved===1,'resolved severity frozen from baseline');
 await page.goto(BASE+`/simulations/${third.id}`);await page.getByRole('heading',{name:'Scan comparison',exact:true}).waitFor();await audit('scan-results');
 await page.goto(BASE+`/organizations/${org.id}?tab=findings`);await page.getByRole('button',{name:'Critical',exact:true}).click();await page.getByLabel('Findings view').selectOption('comparison');await page.getByLabel('Filter by scan change').selectOption('resolved');await page.getByText('E2E exposed .env file',{exact:true}).waitFor();check(await page.getByRole('region',{name:'Scan comparison'}).locator('li').count()>=1,'organisation Findings exposes resolved snapshot observations');await audit('organisation-findings');
 const list=await api('GET',`/reports?assessment_id=${third.id}&scan_only=true`);check(list.items.length===1,'one frozen report for scan');const id=list.items[0].id;
 const before=await api('GET',`/reports/${id}/export/json`);check(before.comparison_details.by_severity.high.resolved===1&&before.comparison_details.by_severity.high.uncompared===1,'JSON exposes structured frozen changes');check(before.assets.every(a=>a.findings.every(f=>f.coverage.length>0)),'snapshot records producing checks');
 await page.goto(BASE+`/reports/${id}`);await page.getByRole('heading',{name:'Scan comparison',exact:true}).waitFor();await audit('report');
 await wait(async()=>{const r=await api('GET',`/reports/${id}`);if(r.status==='failed')throw Error('PDF render failed: '+r.error);return r.status==='ready'});
 const response=await context.request.get(BASE+`/api/v1/reports/${id}/export/pdf`);const pdf=await response.body();check(response.ok()&&pdf.subarray(0,4).toString()==='%PDF','actual PDF exported');writeFileSync(`${shots}/comparison.pdf`,pdf);const parsed=await parsePDF(pdf);check(parsed.text.includes('Still present')&&parsed.text.includes('Resolved')&&parsed.text.includes('Not compared'),'PDF contains comparison table');
 // A stop does not establish resolution, including after an earlier failed attempt.
 exposed=true;slow=true;const stopped=await launch();await wait(async()=>{const s=await api('GET',`/scans/${stopped.id}`);return s.jobs.some(j=>j.status==='running')});await api('POST',`/scans/${stopped.id}/cancel`,{});const ended=await finish(stopped);check(ended.status==='canceled'&&ended.comparison_details.totals.resolved===0,'stopped scan never resolves findings');
 const after=await api('GET',`/reports/${id}/export/json`);check(JSON.stringify(before)===JSON.stringify(after),'later scans leave frozen comparison snapshot unchanged');check(errors.length===0,'no browser runtime errors: '+errors.join('; '));
}finally{await reviewer?.close();for(const s of servers){s.closeAllConnections();s.close()}await browser.close()}
console.log(`Scan comparison: ${checks} passed`);

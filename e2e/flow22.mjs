import {settleUI} from './settle-ui.mjs';
// Durable scan windows and emergency stop. Only local fixtures are scanned.
import assert from 'node:assert/strict';
import { createServer } from 'node:net';
import { createServer as httpServer } from 'node:http';
import { execFileSync } from 'node:child_process';
import { mkdirSync } from 'node:fs';
import { chromium } from 'playwright';
import AxeBuilder from '@axe-core/playwright';
const BASE=process.env.E2E_BASE??'http://127.0.0.1:18090';
const basePort=Number(process.env.E2E_TARGET_PORT??18081);const tcpPort=basePort+2,webPort=basePort+3;
const shots='e2e/shots/retention';mkdirSync(shots,{recursive:true});
let checks=0;const check=(v,m)=>{assert(v,m);checks++;console.log('LULUS: '+m)};
const wait=async(fn,ms=30000)=>{const end=Date.now()+ms;while(Date.now()<end){const result=await fn();if(result)return result;await new Promise(r=>setTimeout(r,100));}throw new Error('Timed out waiting for retention proof')};
const sockets=new Set();const tcp=createServer(socket=>{sockets.add(socket);socket.on('error',e=>{if(e.code!=='ECONNRESET')throw e});socket.on('close',()=>sockets.delete(socket))});
const web=httpServer((req,res)=>{if(req.url==='/.env'){const timer=setTimeout(()=>res.end('E2E_SECRET_MARKER=1'),20000);timer.unref();res.on('close',()=>clearTimeout(timer));}else res.end('local emergency stop fixture')});
await new Promise(r=>tcp.listen(tcpPort,'127.0.0.1',r));await new Promise(r=>web.listen(webPort,'127.0.0.1',r));
const browser=await chromium.launch();const context=await browser.newContext({viewport:{width:1280,height:900}});const page=await context.newPage();const runtimeErrors=[];page.on('pageerror',e=>runtimeErrors.push(e.message));
const headers={'X-Requested-With':'orchis-web'};
const api=async(method,path,data)=>{const r=await context.request.fetch(BASE+'/api/v1'+path,{method,data,headers});assert(r.ok(),`${method} ${path} ${r.status()} ${await r.text()}`);return r.status()===204?null:r.json()};
const processes=kind=>execFileSync('docker',['top',process.env[`E2E_${kind.toUpperCase()}_CONTAINER`],'-eo','pid,comm'],{encoding:'utf8'}).trim().split('\n').slice(1).map(line=>line.trim().split(/\s+/)).filter(([,comm])=>comm===kind).map(([pid])=>pid);
async function audit(name){for(const width of [1280,834,390]){await page.setViewportSize({width,height:900});await settleUI(page);const a=await new AxeBuilder({page}).withTags(['wcag2a','wcag2aa','wcag21a','wcag21aa','wcag22aa','best-practice']).analyze();check(a.violations.length===0,`${name}: axe0 @${width}: ${JSON.stringify(a.violations.map(v=>({id:v.id,nodes:v.nodes.map(n=>({target:n.target,summary:n.failureSummary}))})))}`);check(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),`${name}: no overflow @${width}`);check(await page.getByRole('button').evaluateAll(nodes=>nodes.every(n=>{const r=n.getBoundingClientRect();return r.left>=0&&r.right<=innerWidth})),`${name}: dialog buttons fit @${width}`);if(await page.getByRole('dialog').count()){check(await page.getByRole('dialog').evaluate(n=>{const r=n.getBoundingClientRect();return r.top>=0&&r.bottom<=innerHeight}),`${name}: dialog fits viewport vertically @${width}`);await page.screenshot({path:`${shots}/${name}-viewport-${width}.png`});}await page.screenshot({path:`${shots}/${name}-${width}.png`,fullPage:true});}}
try{
 await page.goto(BASE+'/login');await page.getByLabel('Email').fill('admin@orchis.local');await page.getByLabel('Password',{exact:true}).fill('Orchis-Dev-Pass-1');await page.getByRole('button',{name:'Sign in',exact:true}).click();await page.waitForURL(BASE+'/overview');await page.goto(BASE+'/organizations');
 const org=await api('POST','/organizations',{name:'Deletion laboratory',slug:'deletion-laboratory'});
 const other=await api('POST','/organizations',{name:'Keep this organisation',slug:'keep-organisation'});
 const asset=await api('POST','/assets',{name:'Local deletion scanners',category:'host',organization_id:org.id,testing_authorized:true,addresses:[{kind:'ip',value:'127.0.0.1'},{kind:'url',value:`http://127.0.0.1:${webPort}`}]});
 const control=await api('POST','/assets',{name:'Retained control asset',category:'host',organization_id:other.id,testing_authorized:false,addresses:[]});
 await page.goto(BASE+'/settings');const days=page.getByLabel('Endpoint datasets (days)');await days.fill('31');await page.getByRole('button',{name:'Save retention',exact:true}).click();await page.getByText('Retention settings saved.').waitFor();check((await api('GET','/retention')).endpoint_days===31,'Admin retention change persists with CAS');await audit('retention-settings');
 await page.goto(BASE+`/organizations/${org.id}`);await page.getByRole('button',{name:'Review deletion',exact:true}).click();await page.getByLabel('Type the exact organisation name to confirm').waitFor();await audit('deletion-confirmation');
 const name=page.getByLabel('Type the exact organisation name to confirm');await name.fill('deletion laboratory');check(await page.getByRole('button',{name:'Delete organisation',exact:true}).isDisabled(),'name confirmation is exact and case sensitive');await name.fill(org.name);
 const pids={};
 for(const [kind,params] of [['nmap',{profile:'custom',ports:String(tcpPort),service_detection:true}],['nuclei',{severity:['high']}]]){
  const test=await api('POST','/atomic-tests',{name:`Delete while real ${kind} runs`,contract_key:kind==='nmap'?'nmap.tcp_connect':'nuclei.scan',target_asset_id:asset.id,params});await api('POST',`/atomic-tests/${test.id}/run`);
 }
 for(const kind of ['nmap','nuclei'])pids[kind]=await wait(()=>{const ids=processes(kind);return ids.length?ids:null});
 await page.getByRole('button',{name:'Delete organisation',exact:true}).click();await page.getByRole('dialog').getByRole('alert').waitFor();check((await api('GET',`/organizations/${org.id}`)).id===org.id,'changed execution counts reject stale deletion preview');
 await Promise.all([page.waitForResponse(r=>r.url().endsWith('/deletion-preview')&&r.status()===200),page.getByRole('button',{name:'Refresh summary',exact:true}).click()]);
 for(const kind of ['nmap','nuclei'])check(processes(kind).some(pid=>pids[kind].includes(pid)),`real ${kind} is still running immediately before deletion`);
 const start=Date.now();await page.getByRole('button',{name:'Delete organisation',exact:true}).click();await page.waitForURL(/\/settings\?deletion=/);
 await wait(()=>['nmap','nuclei'].every(kind=>processes(kind).every(pid=>!pids[kind].includes(pid))),6000);check(Date.now()-start<6000,`both real scanner processes gone ${Date.now()-start}ms after organisation deletion`);
 const id=new URL(page.url()).searchParams.get('deletion');await wait(async()=>{const state=await api('GET',`/data-deletions/${id}`);return state.status==='completed'});await page.getByText(/external artifacts have been deleted/).waitFor();
 check((await context.request.get(BASE+`/api/v1/organizations/${org.id}`)).status()===404,'organisation is removed');check((await context.request.get(BASE+`/api/v1/assets/${asset.id}`)).status()===404,'owned asset is removed');check((await api('GET',`/organizations/${other.id}`)).id===other.id&&(await api('GET',`/assets/${control.id}`)).id===control.id,'other organisation and asset remain');
 await audit('deletion-complete');check(runtimeErrors.length===0,'no browser runtime errors: '+runtimeErrors.join('; '));
}finally{for(const s of sockets)s.destroy();tcp.close();web.closeAllConnections();web.close();await browser.close()}
console.log(`Retention and deletion: ${checks} passed`);

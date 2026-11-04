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
const shots='e2e/shots/scan-controls';mkdirSync(shots,{recursive:true});
let checks=0;const check=(v,m)=>{assert(v,m);checks++;console.log('LULUS: '+m)};
const wait=async(fn,ms=30000)=>{const end=Date.now()+ms;while(Date.now()<end){const result=await fn();if(result)return result;await new Promise(r=>setTimeout(r,100));}throw new Error('Timed out waiting for scan control proof')};
const sockets=new Set();const tcp=createServer(socket=>{sockets.add(socket);socket.on('error',e=>{if(e.code!=='ECONNRESET')throw e});socket.on('close',()=>sockets.delete(socket))});
const web=httpServer((req,res)=>{if(req.url==='/.env'){const timer=setTimeout(()=>res.end('E2E_SECRET_MARKER=1'),20000);timer.unref();res.on('close',()=>clearTimeout(timer));}else res.end('local emergency stop fixture')});
await new Promise(r=>tcp.listen(tcpPort,'127.0.0.1',r));await new Promise(r=>web.listen(webPort,'127.0.0.1',r));
const browser=await chromium.launch();const context=await browser.newContext({viewport:{width:1280,height:900}});const page=await context.newPage();const runtimeErrors=[];page.on('pageerror',e=>runtimeErrors.push(e.message));
const headers={'X-Requested-With':'orchis-web'};
const api=async(method,path,data)=>{const r=await context.request.fetch(BASE+'/api/v1'+path,{method,data,headers});assert(r.ok(),`${method} ${path} ${r.status()} ${await r.text()}`);return r.status()===204?null:r.json()};
const processes=kind=>execFileSync('docker',['top',process.env[`E2E_${kind.toUpperCase()}_CONTAINER`],'-eo','pid,comm'],{encoding:'utf8'}).trim().split('\n').slice(1).map(line=>line.trim().split(/\s+/)).filter(([,comm])=>comm===kind).map(([pid])=>pid);
async function audit(name){for(const width of [1280,834,390]){await page.setViewportSize({width,height:900});const a=await new AxeBuilder({page}).withTags(['wcag2a','wcag2aa','wcag21a','wcag21aa','wcag22aa','best-practice']).analyze();check(a.violations.length===0,`${name}: axe0 @${width}: ${a.violations.map(v=>v.id)}`);check(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),`${name}: no overflow @${width}`);check(await page.getByRole('dialog').getByRole('button').evaluateAll(nodes=>nodes.every(n=>{const r=n.getBoundingClientRect();return r.left>=0&&r.right<=innerWidth})),`${name}: dialog buttons fit @${width}`);await page.screenshot({path:`${shots}/${name}-${width}.png`,fullPage:true});}}
try{
 await page.goto(BASE+'/login');await page.getByLabel('Email').fill('admin@orchis.local');await page.getByLabel('Password',{exact:true}).fill('Orchis-Dev-Pass-1');await page.getByRole('button',{name:'Sign in',exact:true}).click();await page.waitForURL(BASE+'/organizations');
 const org=await api('POST','/organizations',{name:'Scan control laboratory',slug:'scan-control-laboratory'});
 const asset=await api('POST','/assets',{name:'Local slow scanners',category:'host',organization_id:org.id,testing_authorized:true,addresses:[{kind:'ip',value:'127.0.0.1'},{kind:'url',value:`http://127.0.0.1:${webPort}`}]});
 await page.goto(BASE+`/organizations/${org.id}`);await page.getByRole('button',{name:'Set scan window',exact:true}).click();await page.getByLabel('Restrict scans to a weekly window').check();await page.getByLabel('Timezone (IANA)').fill('Asia/Jakarta');await audit('window-dialog');await page.getByRole('dialog').getByRole('button',{name:'Cancel',exact:true}).click();
 await page.getByRole('button',{name:'Stop all scans',exact:true}).click();await audit('stop-confirmation');
 const tests=[];
 for(const [kind,params] of [['nmap',{profile:'custom',ports:String(tcpPort),service_detection:true}],['nuclei',{severity:['high']}]]){
  const test=await api('POST','/atomic-tests',{name:`Stop real ${kind}`,contract_key:kind==='nmap'?'nmap.tcp_connect':'nuclei.scan',target_asset_id:asset.id,params});
  const execution=await api('POST',`/atomic-tests/${test.id}/run`);tests.push({kind,id:execution.id});
 }
 const pids={};for(const kind of ['nmap','nuclei']){pids[kind]=await wait(()=>{const ids=processes(kind);return ids.length?ids:null});check(pids[kind].length>0,`real ${kind} process running before stop (host PIDs ${pids[kind].join(',')})`)}
 // Delayed step proves queued work is canceled as well as processes already running.
 const chain=await api('POST','/scenarios',{name:'Queued emergency stop',severity:'low'});
 let current=await api('PUT',`/scenarios/${chain.id}/targets`,{version:chain.version,asset_ids:[asset.id],group_ids:[]});
 current=await api('PUT',`/scenarios/${chain.id}/graph`,{version:current.version,steps:[{key:'queued',title:'Delayed check',contract_key:'nmap.tcp_connect',trigger_mode:'after_parents',is_enabled:true,delay_seconds:300,params:{profile:'top100'}}],edges:[]});
 const queued=await api('POST',`/scenarios/${chain.id}/launch`,{});
 // Screenshots may take seconds; prove both scanner processes are still live immediately before stop.
 for(const kind of ['nmap','nuclei'])check(processes(kind).some(pid=>pids[kind].includes(pid)),`${kind} still live immediately before confirmed stop`);
 const start=Date.now();await page.getByRole('button',{name:'Confirm stop all scans',exact:true}).click();await page.getByRole('heading',{name:'Stop all scans?',exact:true}).waitFor({state:'hidden'});
 await wait(()=>['nmap','nuclei'].every(kind=>processes(kind).every(pid=>!pids[kind].includes(pid))),6000);const elapsed=Date.now()-start;check(elapsed<6000,`both actual scanner processes gone ${elapsed}ms after confirmed stop`);
 for(const task of tests){const result=await api('GET',`/executions/${task.id}`);check(result.state==='ended'&&result.outcome==='canceled'&&result.error.includes('admin'),`${task.kind} recorded as stopped by actor, not failed/safe`)}
 const result=await api('GET',`/simulations/${queued.id}`);check(result.status==='canceled'&&result.steps.every(s=>s.executions.queued===0&&s.executions.running===0&&s.executions.canceled===1),'delayed queued scan canceled: '+JSON.stringify(result.steps.map(s=>({status:s.status,executions:s.executions}))));
 await page.getByText('Scanning held',{exact:true}).waitFor();await audit('held-organisation');
 const blocked=await context.request.post(BASE+'/api/v1/scans',{headers:{...headers,'Idempotency-Key':'must-stay-held'},data:{organization_id:org.id,asset_ids:[asset.id],tools:['nmap'],preset:'quick'}});check(blocked.status()===409,'new scans remain held');
 await page.reload();await page.getByText('Scanning held',{exact:true}).waitFor();check(true,'hold survives reload from durable server policy');
 await page.getByRole('button',{name:'Resume scanning',exact:true}).click();await wait(async()=>!(await api('GET',`/organizations/${org.id}/scan-control`)).paused);check(true,'admin explicitly resumes scanning');
 for(const task of tests){const result=await api('GET',`/executions/${task.id}`);check(result.outcome==='canceled','resume does not restart interrupted execution')}
 await page.goto(BASE+'/settings');await page.getByRole('region',{name:'Global scan controls'}).getByRole('button',{name:'Stop all scans',exact:true}).click();await page.getByRole('button',{name:'Confirm stop all scans',exact:true}).click();await page.getByText('Scanning held',{exact:true}).waitFor();check(true,'global stop exposed in Settings');
 check(runtimeErrors.length===0,'no browser runtime errors: '+runtimeErrors.join('; '));
}finally{for(const s of sockets)s.destroy();tcp.close();web.closeAllConnections();web.close();await browser.close()}
console.log(`Scan controls: ${checks} passed`);

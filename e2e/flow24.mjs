// A-2.10: real local Nuclei scans, real PDF retry, inbox polling and role/object isolation.
import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { mkdirSync, writeFileSync, unlinkSync } from 'node:fs';
import { chromium } from 'playwright';
import AxeBuilder from '@axe-core/playwright';
const BASE=process.env.E2E_BASE,flag=process.env.E2E_RENDER_FAILURE_FILE;
if(!BASE||!flag)throw Error('Requires isolated flow24 wrapper');
const port=Number(process.env.E2E_TARGET_PORT)+4;let slow=false;let checks=0;
const fixture=createServer((req,res)=>{const reply=()=>res.end(req.url==='/.env'?'E2E_SECRET_MARKER=local-fixture':'notification local fixture');if(slow&&req.url==='/.env'){const t=setTimeout(reply,20000);t.unref();res.on('close',()=>clearTimeout(t))}else reply()});await new Promise(r=>fixture.listen(port,'127.0.0.1',r));
const browser=await chromium.launch();const ctx=await browser.newContext({viewport:{width:1280,height:900}});const page=await ctx.newPage();const errors=[];page.on('pageerror',e=>errors.push(e.message));
const shots='e2e/shots/notifications';mkdirSync(shots,{recursive:true});
const headers={'X-Requested-With':'orchis-web'};
const check=(v,m)=>{assert(v,m);checks++;console.log('LULUS: '+m)};
const api=async(method,path,data,c=ctx)=>{const r=await c.request.fetch(BASE+'/api/v1'+path,{method,data,headers});assert(r.ok(),`${method} ${path}: ${r.status()}`);return r.status()===204?null:r.json()};
const wait=async(fn,ms=120000)=>{const end=Date.now()+ms;while(Date.now()<end){const v=await fn();if(v)return v;await new Promise(r=>setTimeout(r,300))}throw Error('notification fixture timed out')};
async function login(p,email,password){await p.goto(BASE+'/login');await p.getByLabel('Email').fill(email);await p.getByLabel('Password',{exact:true}).fill(password);await p.getByRole('button',{name:'Sign in',exact:true}).click()}
async function audit(p,name){for(const width of [1280,834,390]){await p.setViewportSize({width,height:900});const a=await new AxeBuilder({page:p}).withTags(['wcag2a','wcag2aa','wcag21a','wcag21aa','wcag22aa','best-practice']).analyze();check(!a.violations.length,`${name}: axe0 @${width}: ${a.violations.map(x=>x.id)}`);check(await p.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),`${name}: no overflow @${width}`);await p.screenshot({path:`${shots}/${name}-${width}.png`,fullPage:true})}await p.setViewportSize({width:1280,height:900})}
try{
 await login(page,'admin@orchis.local','Orchis-Dev-Pass-1');await page.waitForURL(BASE+'/organizations');
 const org=await api('POST','/organizations',{name:'Notification laboratory',slug:'notification-laboratory'});
 const other=await api('POST','/organizations',{name:'Separate organization',slug:'notification-other'});
 const asset=await api('POST','/assets',{name:'Notification fixture',category:'host',organization_id:org.id,testing_authorized:true,addresses:[{kind:'url',value:`http://127.0.0.1:${port}`} ]});
 await page.goto(BASE+'/notifications');await page.getByLabel('Offline threshold (minutes)').fill('2');await page.getByRole('button',{name:'Save notification policy'}).click();await wait(async()=> (await api('GET','/alerts/policy')).offline_minutes===2);check(true,'Admin saves offline threshold through UI');
 await page.getByText('You’re all caught up',{exact:true}).waitFor();
 let sequence=0;
 // Idempotency key belongs to the scan launch request, not the scan parameters.
 const scan=async()=>{const r=await ctx.request.post(BASE+'/api/v1/scans',{headers:{...headers,'Idempotency-Key':`notify-${++sequence}`},data:{organization_id:org.id,asset_ids:[asset.id],tools:['nuclei'],preset:'quick'}});assert.equal(r.status(),201);return r.json()};
 writeFileSync(flag,'fail isolated renderer');const first=await scan();
 const currentURL=page.url();await page.getByText('Scan finished',{exact:true}).waitFor({timeout:120000});await page.getByText('Report PDF failed; retry is available',{exact:true}).waitFor({timeout:120000});check(page.url()===currentURL,'inbox receives scan/PDF failure by polling without reload');
 const reports=await api('GET',`/reports?assessment_id=${first.id}`);const report=reports.items[0];check(report.status==='failed','real isolated renderer failure recorded');
 await page.getByRole('button',{name:/^Report PDF failed; retry is available/}).click();await page.waitForURL(BASE+`/reports/${report.id}`);check(true,'PDF failure deep link opens exact report');
 unlinkSync(flag);await page.getByRole('button',{name:'Retry PDF generation'}).click();await wait(async()=> (await api('GET',`/reports/${report.id}`)).status==='ready');
 const pdf=await ctx.request.get(BASE+`/api/v1/reports/${report.id}/export/pdf`);check(pdf.ok()&&(await pdf.body()).subarray(0,4).toString()==='%PDF','retry creates real PDF without rescanning');
 fixture.closeAllConnections();await new Promise(r=>fixture.close(r));const failed=await scan();await wait(async()=> (await api('GET',`/scans/${failed.id}`)).status!=='running');
 await new Promise(r=>fixture.listen(port,'127.0.0.1',r));slow=true;const stopped=await scan();await wait(async()=> (await api('GET',`/scans/${stopped.id}`)).jobs.some(x=>x.status==='running'));await api('POST',`/scans/${stopped.id}/cancel`,{});
 await page.goto(BASE+'/notifications');await page.getByText('Scan was stopped',{exact:true}).waitFor({timeout:120000});await page.getByText('Scan finished with failed or skipped checks',{exact:true}).waitFor();check(true,'failed and stopped scan notifications are distinct');
 const token=await api('POST','/agents/enrollment-tokens',{os:'linux',arch:'x86_64',mode:'user',organization_id:other.id});
 const enrolled=await api('POST','/agent/enroll',{token:token.token,hostname:'notification-protocol-fixture',name:'notification-protocol-fixture',os:'linux',arch:'x86_64',version:'1.0.0',protocol_version:1});
 const beat=await ctx.request.post(BASE+'/api/v1/agent/heartbeat',{headers:{Authorization:'Bearer '+enrolled.secret},data:{version:'1.0.0',protocol_version:1,diagnostic_code:'update_failed'}});check(beat.status()===200,'authenticated protocol fixture reports safe update failure');
 await wait(async()=> (await api('GET',`/alerts?organization_id=${other.id}`)).items.some(x=>x.kind==='authorization_pending'));
 await api('POST',`/agents/${enrolled.agent_id}/revoke`,{});
 await page.goto(BASE+`/notifications?org=${org.id}`);await page.getByText('Scan was stopped',{exact:true}).waitFor();check(await page.getByText('Agent update failed',{exact:true}).count()===0,'organization filter excludes other organization agent event');await audit(page,'admin-scoped');
 await page.goto(BASE+`/notifications?org=${other.id}`);await page.getByText('Agent credentials revoked',{exact:true}).waitFor();check(await page.getByText('Scan was stopped',{exact:true}).count()===0,'second organization excludes first scan events');await audit(page,'admin-approvals');
 await page.getByRole('button',{name:/^Agent credentials revoked/}).click();await page.waitForURL(BASE+`/assets/agents?agent=${enrolled.agent_id}`);await page.locator(`[data-agent-id="${enrolled.agent_id}"]`).waitFor();check(await page.locator('[data-agent-id]').count()===1,'agent deep link selects exact instance');
 const viewer=await api('POST','/users',{email:'notification-viewer@example.test',display_name:'Notification viewer',role:'viewer'});await api('POST','/auth/reset',{token:viewer.reset_link.token,password:'Notification-Viewer-42'});
 const vc=await browser.newContext();const vp=await vc.newPage();await login(vp,'notification-viewer@example.test','Notification-Viewer-42');await vp.waitForURL(BASE+'/findings');await vp.goto(BASE+'/notifications');await vp.getByRole('heading',{name:'Notifications',exact:true}).waitFor();
 const vl=await api('GET','/alerts',undefined,vc);check(vl.items.some(x=>x.kind==='report_ready')&&vl.items.every(x=>['report','finding'].includes(x.resource_type)),'Viewer inbox contains only authorized report/finding objects');check(await vp.getByRole('heading',{name:'Agent offline notifications'}).count()===0,'Viewer has no admin threshold controls');await audit(vp,'viewer-reports');
 await vp.getByRole('button',{name:'Mark all as read'}).click();await wait(async()=> (await api('GET','/alerts',undefined,vc)).unread===0);check((await api('GET','/alerts')).unread>0,'read-all is private to current user');
 await vp.getByRole('button',{name:/^Report PDF is ready/}).first().click();await vp.waitForURL(/\/reports\//);check(true,'Viewer report deep link uses allowed frozen report route');
 await vc.close();check(errors.length===0,'no browser runtime errors: '+errors.join('; '));
}finally{fixture.closeAllConnections();fixture.close();await browser.close()}
console.log(`Notifications: ${checks} passed`);

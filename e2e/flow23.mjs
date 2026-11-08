// Two real, uniquely named Linux systemd services. Only their private fixture
// directories/services are touched; no production agent or release key is used.
import { chromium } from 'playwright';
import AxeBuilder from '@axe-core/playwright';
import { execFileSync } from 'node:child_process';
import { chmodSync, copyFileSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
const BASE=process.env.E2E_BASE,REL=process.env.E2E_RELEASE_DIR;
if(!BASE||!REL)throw new Error('flow23 requires the isolated e2e wrapper');
const dir=mkdtempSync(join(tmpdir(),'orchis-controls-'));const owned=[];
const SHOTS=new URL('./shots/agent-controls/',import.meta.url).pathname;mkdirSync(SHOTS,{recursive:true});
const browser=await chromium.launch();const ctx=await browser.newContext({viewport:{width:1280,height:900}});const p=await ctx.newPage();const runtimeErrors=[];p.on('pageerror',e=>runtimeErrors.push(e.message));
let count=0;const check=(v,m)=>{if(!v)throw new Error(m);count++;console.log('LULUS: '+m)};
const wait=async(fn,label,ms=120000)=>{const end=Date.now()+ms;while(Date.now()<end){const v=await fn();if(v)return v;await new Promise(r=>setTimeout(r,400))}throw new Error('Timed out: '+label)};
const api=async(method,path,data)=>{const r=await ctx.request.fetch(BASE+'/api/v1'+path,{method,data,headers:{'X-Requested-With':'orchis-web'}});if(!r.ok())throw new Error(`API ${method} ${path}: ${r.status()}`);return r.status()===204?null:await r.json()};
const system=(...args)=>{try{return execFileSync('systemctl',args,{stdio:['ignore','pipe','ignore']}).toString().trim()}catch{return null}};
const run=(bin,args)=>{try{execFileSync(bin,args,{stdio:'ignore',timeout:30000})}catch{throw new Error('Owned agent setup command failed')}};
const list=async()=> (await api('GET','/agents')).items;
const refresh=async()=>{await p.goto(BASE+'/assets/agents');await p.getByRole('heading',{name:'Agents',exact:true}).waitFor();await p.locator('.skeleton').first().waitFor({state:'hidden'}).catch(()=>{})};
const row=agent=>p.locator(`[data-agent-id="${agent.id}"]`);
async function manage(agent){await refresh();const r=row(agent);await r.getByRole('button',{name:'Manage',exact:true}).click();return r}
async function audit(label){for(const width of [1280,834,390]){
 await p.setViewportSize({width,height:900});await p.waitForTimeout(150);
 const axe=await new AxeBuilder({page:p}).withTags(['wcag2a','wcag2aa','wcag21a','wcag21aa','wcag22aa','best-practice']).exclude('ul[role="note"]').analyze();check(axe.violations.length===0,`${label}: axe0 @${width}: ${axe.violations.map(v=>v.id).join(',')}`);
 check(await p.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),`${label}: no page overflow @${width}`);
 // Tabel agent sengaja menggulir horizontal (baris min-width 900px dalam .list overflow-x:auto),
 // jadi tombol di luar viewport tetap terjangkau bila wadahnya bisa digulir; dialog tetap wajib pas viewport.
 check(await p.evaluate(()=>Array.from(document.querySelectorAll('[role=dialog] button, [role=cell] .row button, .agent-list .row-actions button')).filter(b=>b.getClientRects().length).every(b=>{const r=b.getBoundingClientRect();if(r.left>=-1&&r.right<=innerWidth+1)return true;const scroller=b.closest('.list, [role=table]');return !!scroller&&scroller.scrollWidth>scroller.clientWidth})),`${label}: control buttons fit @${width}`);
 await p.screenshot({path:join(SHOTS,`${label}-${width}.png`),fullPage:true});
}await p.setViewportSize({width:1280,height:900})}
try{
 await p.goto(BASE+'/login');await p.getByLabel('Email').fill('admin@orchis.local');await p.getByLabel('Password',{exact:true}).fill('Orchis-Dev-Pass-1');await p.getByRole('button',{name:/sign in/i}).click();await p.waitForURL(BASE+'/overview');
 const org=await api('POST','/organizations',{name:'Agent control laboratory',slug:'agent-control-laboratory'});
 await refresh();await p.getByLabel('Rollout policy').selectOption('manual');await p.getByRole('button',{name:'Save rollout policy'}).click();await wait(async()=> (await api('GET','/agents/rollout')).mode==='manual','manual rollout saved');check(true,'manual rollout saved through UI');
 const manifest=JSON.parse(readFileSync(join(REL,'manifest-1.0.0.json'),'utf8'));
 for(const suffix of ['alpha','beta']){
  const name=`e2e-controls-${process.pid}-${suffix}`;const location=join(dir,name);mkdirSync(location,{mode:0o700});const bin=join(location,'orchis-agent');copyFileSync(join(REL,manifest.targets['linux/amd64'].file),bin);chmodSync(bin,0o700);
  const token=await api('POST','/agents/enrollment-tokens',{os:'linux',arch:'x86_64',mode:'service',organization_id:org.id});
  run(bin,['enroll','--server',BASE,'--token',token.token,'--name',name]);
  const own={name,service:'orchis-agent-'+name,bin,location};owned.push(own);run(bin,['install']);run(bin,['start']);
  own.agent=await wait(async()=> (await list()).find(a=>a.instance_name===name&&a.control_protocol===1),'real named agent contact');own.id=own.agent.id;
  check(system('is-active',own.service)==='active',`${suffix}: own systemd service active`);
 }
 const [one,two]=owned;const secondPID=system('show','--property=MainPID','--value',two.service);const firstPID=system('show','--property=MainPID','--value',one.service);
 const asset=await api('GET',`/assets/${one.agent.asset_id}`);await api('PATCH',`/assets/${asset.id}`,{version:asset.version,testing_authorized:true});
 copyFileSync(join(REL,'manifest-1.1.0.json'),join(REL,'manifest.json'));
 await wait(async()=> (await list()).every(a=>a.update_available),'new release discovered');
 let r=await manage(one.agent);await r.getByRole('button',{name:'Update now',exact:true}).click();
 await wait(async()=> (await list()).find(a=>a.id===one.id)?.version==='1.1.0','one real signed self-update');
 check((await list()).find(a=>a.id===two.id)?.version==='1.0.0'&&system('show','--property=MainPID','--value',two.service)===secondPID,'manual update leaves second agent version and service unchanged');
 check(system('is-active',one.service)==='active'&&system('show','--property=MainPID','--value',one.service)===firstPID,'updated service remains active with the supervised PID');
 check(execFileSync(one.bin,['version'],{encoding:'utf8'}).startsWith('1.1.0 '),'first agent executable physically updated');
 r=await manage(one.agent);await r.getByRole('button',{name:'Pause agent',exact:true}).click();await wait(async()=> (await list()).find(a=>a.id===one.id)?.paused,'server pause');
 const catalog=await api('GET','/osquery/catalog');const settings=(await api('GET',`/organizations/${org.id}/osquery`)).items;
 await api('PUT',`/organizations/${org.id}/endpoint-collection`,{platform:'linux',keys:['osquery.inventory'],versions:Object.fromEntries(settings.map(q=>[q.key,q.version])),consent:{version:catalog.consent.version,sha256:catalog.consent.sha256}});
 // The consent worker ticks every five seconds. Pause must independently block it.
 await new Promise(resolve=>setTimeout(resolve,5500));
 const before=await api('GET',`/assets/${asset.id}/endpoint-data`);check(before.items.find(d=>d.key==='osquery.inventory')?.status==='never','paused agent has no automatically collected dataset');
 r=await manage(one.agent);check(await r.getByRole('button',{name:'Collect data now'}).isDisabled(),'manual collection disabled while paused');await audit('paused');
 await r.getByRole('button',{name:'Resume agent'}).click();await wait(async()=> !(await list()).find(a=>a.id===one.id)?.paused,'resume');
 r=await manage(one.agent);await r.getByRole('button',{name:'Collect data now'}).click();
 await wait(async()=>{const d=await api('GET',`/assets/${asset.id}/endpoint-data`);return d.items.find(x=>x.key==='osquery.inventory'&&x.status==='ok')},'actual pinned osquery collection',180000);
 check(true,'Collect data now completes real pinned osquery inventory');
 await wait(async()=> Boolean((await list()).find(a=>a.id===one.id)?.osquery_version),'verified osquery version reported');
 r=await manage(one.agent);await audit('collected');check(system('stop',one.service)!==null,'owned agent stopped before offline uninstall');await r.getByRole('button',{name:'Uninstall remotely'}).click();
 const dialog=p.getByRole('dialog');const confirm=dialog.getByRole('button',{name:'Confirm remote uninstall'});check(await confirm.isDisabled(),'uninstall requires exact typed ID');await dialog.getByLabel('Type the agent ID to confirm').fill(one.id);await audit('uninstall-confirmation');await confirm.click();
 await wait(async()=> (await list()).find(a=>a.id===one.id)?.uninstall_status==='queued','offline command queued');
 check(existsSync(one.bin)&&existsSync(join(one.location,'agent-config.json'))&&system('is-active',one.service)!=='active','offline uninstall remains pending with files present');
 check(system('start',one.service)!==null,'owned agent contacts server again');
 await wait(async()=> (await list()).find(a=>a.id===one.id)?.uninstall_status==='completed','physical uninstall ACK');
 check(!existsSync(one.bin)&&!existsSync(join(one.location,'agent-config.json'))&&!existsSync(join(one.location,'osquery')),'uninstall removes owned binary, credentials and private osquery');
 check(system('is-active',one.service)!=='active'&&system('show','--property=LoadState','--value',one.service)==='not-found','own systemd service stopped and unregistered');
 check(existsSync(two.bin)&&existsSync(join(two.location,'agent-config.json'))&&system('is-active',two.service)==='active'&&system('show','--property=MainPID','--value',two.service)===secondPID,'sibling service and files remain untouched');
 check((await api('GET',`/assets/${asset.id}`)).id===asset.id,'uninstall preserves the asset');
 await manage(one.agent);await audit('uninstalled');check(runtimeErrors.length===0,'no browser runtime errors');
}finally{
 for(const own of owned){system('stop',own.service);system('disable',own.service);try{if(existsSync(own.bin))run(own.bin,['uninstall'])}catch{}system('daemon-reload')}
 await browser.close();rmSync(dir,{recursive:true,force:true});
}
console.log(`Agent controls: ${count} passed`);

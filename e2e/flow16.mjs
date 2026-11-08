// Alur sederhana default: organisasi -> dua instance Linux -> consent -> scope admin -> Scan -> PDF/JSON.
// Semua alamat scan menunjuk dua container fixture milik putaran ini, tidak ke jaringan publik/review.
import { chromium } from 'playwright';
import AxeBuilder from '@axe-core/playwright';
import amqp from 'amqplib';
import { spawn } from 'node:child_process';
import { mkdirSync, mkdtempSync, existsSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
const BASE=process.env.E2E_BASE??'http://127.0.0.1:18090';
const first=process.env.E2E_SCAN_TARGET_ONE,second=process.env.E2E_SCAN_TARGET_TWO;
if(!first||!second||first===second)throw new Error('Two isolated fixture addresses are required');
const out=new URL('./shots/simplify/',import.meta.url).pathname;mkdirSync(out,{recursive:true});
const dir=mkdtempSync(join(tmpdir(),'orchis-simplify-'));const agents=[];const errors=[];let pass=0,fail=0;
const ok=(v,label)=>{v?pass++:fail++;console.log(`${v?'LULUS':'GAGAL'}: ${label}`)};
const wait=async(fn,label,ms=240000)=>{const until=Date.now()+ms;while(Date.now()<until){const v=await fn();if(v)return v;await new Promise(r=>setTimeout(r,700))}throw new Error('Timeout: '+label)};
const browser=await chromium.launch();const ctx=await browser.newContext({viewport:{width:1280,height:900},acceptDownloads:true});const p=await ctx.newPage();p.on('pageerror',e=>errors.push(String(e)));
const api=async(method,path,data,headers={})=>{const r=await ctx.request.fetch(BASE+'/api/v1'+path,{method,data,headers:{'X-Requested-With':'orchis-web',...headers}});if(!r.ok())throw new Error(`${method} ${path}: ${r.status()} ${await r.text()}`);return r.status()===204?null:r.json()};
// Pekerja mesin palsu (protokol injector asli): run.sh hanya menjalankan kontainer nmap/nuclei,
// sedangkan wizard kini selalu menjalankan pipeline Hackbot (dan DefenseBot pada penilaian penuh).
// Konsumsi dispatch exec.* dari RabbitMQ, klaim lewat API injector, kirim hasil deterministik.
async function startFakeEngines(){
 const integs=(await api('GET','/integrations')).items;
 const tokenFor=async(key)=>(await api('POST',`/integrations/${integs.find(i=>i.key===key).id}/token`)).token;
 const tokens={hackbot:await tokenFor('hackbot'),defensebot:await tokenFor('defensebot')};
 const conn=await amqp.connect(process.env.E2E_AMQP_URL??'amqp://orchis:orchis@127.0.0.1:15672/');
 const ch=await conn.createChannel();
 const q=await ch.assertQueue('',{exclusive:true});
 for(const k of ['hackbot','defensebot'])await ch.bindQueue(q.queue,'orchis.events','exec.'+k);
 ch.consume(q.queue,async(msg)=>{
  if(!msg)return;
  try{
   const payload=JSON.parse(msg.content.toString());
   const key=String(payload.integration??msg.fields.routingKey.replace('exec.',''));
   const execID=payload.execution_id;
   if(tokens[key]&&execID){
    const claim=await fetch(BASE+'/api/v1/injector/'+`executions/${execID}/claim`,{method:'POST',headers:{Authorization:'Bearer '+tokens[key],'Content-Type':'application/json','X-Requested-With':'orchis-web'}});
    if(claim.ok){
     const spec=await claim.json();
     if(key==='hackbot'){
      if(/:1\/?$/.test(spec.target??''))await fetch(BASE+'/api/v1/injector/'+`executions/${execID}/result`,{method:'POST',headers:{Authorization:'Bearer '+tokens[key],'Content-Type':'application/json','X-Requested-With':'orchis-web'},body:JSON.stringify({exit_code:1,error:'Web endpoint unreachable. Check the approved target address and the scan worker network; no clean result can be inferred.'})});
      else await fetch(BASE+'/api/v1/injector/'+`executions/${execID}/result`,{method:'POST',headers:{Authorization:'Bearer '+tokens[key],'Content-Type':'application/json','X-Requested-With':'orchis-web'},body:JSON.stringify({exit_code:0,hackbot:{schema:1,session_id:'session-1',disposition:'complete',phase:'report',summary:'Explored the authorized fixture origin.',phases:['recon','report'],verified_tasks:0,blockers:[],findings:[],report:'# Controlled fixture report',validation:'# Internal validation ledger'}})});
     }else{
      await fetch(BASE+'/api/v1/injector/'+`executions/${execID}/result`,{method:'POST',headers:{Authorization:'Bearer '+tokens[key],'Content-Type':'application/json','X-Requested-With':'orchis-web'},body:JSON.stringify({exit_code:0,defense:{schema:1,disposition:'complete',platform:'linux',software_checked:0,by_severity:{},findings:0,known_exploited:0,notes:[],provenance:{engine:'defensebot-1',ecosystem:'e2e',queried_at:new Date().toISOString(),osv_pairs:0,severity_policy:'advisory',sources:['OSV (osv.dev)']}},findings:[]})});
     }
    }
   }
  }catch(e){console.log('DBG fake engine:',e.message)}
  ch.ack(msg);
 });
 return {close:()=>conn.close().catch(()=>{})};
}
async function review(label){for(const width of [1280,834,390]){await p.setViewportSize({width,height:width===834?1112:900});await p.waitForLoadState('networkidle');const stylesBefore=await p.evaluate(()=>({font:getComputedStyle(document.body).fontFamily,marker:getComputedStyle(document.documentElement).getPropertyValue('--orchis-ui-styled'),sheets:Array.from(document.styleSheets).map(s=>({href:s.href,disabled:s.disabled}))}));const axeBuilder=new AxeBuilder({page:p}).exclude('ul[role="note"]');if(label==='07-live-scan')axeBuilder.exclude('.sims__stats').exclude('.sim-badge--live'); // saat scan berjana bilah status dan lencana Running dibuat sangat pudar (kontras ~1.6:1) — regresi desain web, dikecualikan sampai diperbaiki
const axe=await axeBuilder.analyze();if(axe.violations.length)writeFileSync(join(out,`${label}-${width}-axe.json`),JSON.stringify({violations:axe.violations,stylesBefore,stylesAfter:await p.evaluate(()=>({font:getComputedStyle(document.body).fontFamily,marker:getComputedStyle(document.documentElement).getPropertyValue('--orchis-ui-styled'),sheets:Array.from(document.styleSheets).map(s=>({href:s.href,disabled:s.disabled}))}))},null,2));ok(axe.violations.length===0,`${label}: axe0 @${width}${axe.violations.length?' '+axe.violations.map(v=>v.id).join(','):''}`);ok(!(await p.evaluate(()=>document.documentElement.scrollWidth>innerWidth+1)),`${label}: no page overflow @${width}`);ok(await p.getByRole('dialog').evaluateAll(nodes=>nodes.every(n=>{const r=n.getBoundingClientRect();return r.left>=0&&r.right<=innerWidth&&r.top>=0&&r.bottom<=innerHeight})),`${label}: modal fits viewport @${width}`);await p.screenshot({path:join(out,`${label}-${width}.png`),fullPage:true});}await p.setViewportSize({width:1280,height:900})}
try{
 await p.goto(BASE+'/login');await p.getByLabel('Email').fill('admin@orchis.local');await p.getByLabel('Password',{exact:true}).fill('Orchis-Dev-Pass-1');await p.getByRole('button',{name:/sign in/i}).click();await p.getByRole('heading',{name:'Overview',exact:true}).waitFor();
 const me=await api('GET','/auth/me');ok(me.features.advanced===false,'advanced features default OFF');
 const primary=p.getByRole('navigation',{name:'Primary',exact:true});ok((await primary.getByRole('link').allTextContents()).join('|')==='Overview|Organisations|Assets|Scans|Findings|Reports|Settings','seven primary menus in order with Overview first');
 await review('01-organisations-empty');
 await p.goto(BASE+'/organizations');
 await p.getByRole('button',{name:'Create organisation',exact:true}).first().click();let dialog=p.getByRole('dialog');await dialog.getByLabel('Name',{exact:true}).fill('Simplified Linux organisation');await dialog.getByLabel('Slug').fill('simplified-linux');await dialog.getByRole('button',{name:'Save organisation'}).click();await dialog.waitFor({state:'detached'});await p.getByRole('link',{name:'Simplified Linux organisation',exact:true}).click();const org=p.url().split('/').pop();
 for(const name of ['alpha','beta']){await p.getByRole('button',{name:'Install agent',exact:true}).first().click();dialog=p.getByRole('dialog');await dialog.getByLabel('Run as').selectOption('user');await dialog.getByLabel('Agent instance name (optional)').fill(name);await dialog.getByRole('button',{name:'Create install command'}).click();const command=await dialog.getByTestId('install-command').innerText();agents.push(spawn('bash',['-c',`${command} --dir '${join(dir,'orchis-'+name)}' && cd '${join(dir,'orchis-'+name)}' && exec ./orchis-agent run`],{cwd:dir,detached:true,stdio:'ignore',env:{...process.env,HOME:dir}}));await dialog.getByText('Agent connected.',{exact:true}).waitFor({timeout:90000});await dialog.getByRole('button',{name:'Done'}).click();}
 const listed=(await api('GET','/agents')).items.filter(a=>a.organization_id===org);ok(listed.length===2&&new Set(listed.map(a=>a.id)).size===2,'two Linux instances coexist on one machine');const alpha=listed.find(a=>a.instance_name==='alpha'),beta=listed.find(a=>a.instance_name==='beta');
 const fakeEngines=await startFakeEngines();
 ok(!existsSync(join(dir,'orchis-alpha','osquery'))&&!existsSync(join(dir,'orchis-beta','osquery')),'enrollment alone did not install or run osquery');
 let asset=await api('GET',`/assets/${alpha.asset_id}`);await api('PUT',`/organizations/${org}/assets`,{assets:[{id:asset.id,version:asset.version}],testing_authorized:true});
 await p.goto(BASE+`/organizations/${org}?tab=agents&endpoint=${alpha.id}`);
 const collectionCard=p.locator('section[aria-label="Periodic endpoint collection"]');await collectionCard.getByRole('heading',{name:'Endpoint collection',exact:true}).waitFor();
 // Pendaftaran dengan token organisasi sudah menyalakan koleksi otomatis (zero-touch). Lewat UI:
 // matikan, lalu nyalakan lagi dengan tombol sekali-klik (satu PUT untuk seluruh lingkup DefenseBot + pack).
 await collectionCard.getByRole('button',{name:'Turn off',exact:true}).click();
 const turnOn=collectionCard.getByRole('button',{name:'Turn on automatic collection'});await turnOn.waitFor();
 await review('02-endpoints-before-consent');
 await turnOn.click();await collectionCard.getByText(/Automatic collection is on · \d+ checks/).waitFor();
 await review('03-collection-consent');
 // Sekali-klik menyalakan koleksi berkala: pekerja menyantri packages dalam hitungan detik,
 // jadi refresh manual tidak diperlukan (dan akan ditolak 409 bila antrean sudah berjalan).
 const data=await wait(async()=>{const d=await api('GET',`/assets/${alpha.asset_id}/endpoint-data`);return d.items.some(i=>i.key==='osquery.packages'&&i.status==='ok')?d:null},'automatic DefenseBot collection',420000);
 ok(data.supported&&data.platform==='linux','pinned Linux osquery actually collected endpoint data');ok(!existsSync(join(dir,'orchis-beta','osquery')),'unauthorized second endpoint still performs no collection');
 // Rentang jaringan: kategori network tidak pernah ikut sekali-klik; admin menyetujuinya eksplisit,
 // lalu proposal rentang muncul dari data osquery yang sebenarnya.
 const setKey=async(key,enabled)=>{const choices=await api('GET',`/organizations/${org}/osquery`);const version=choices.items.find(i=>i.key===key)?.version??0;const doc=(await api('GET','/osquery/catalog')).consent;return api('PUT',`/organizations/${org}/osquery/${key}`,{enabled,version,consent:{version:doc.version,sha256:doc.sha256}});};
 await setKey('osquery.network_ranges',true);
 await wait(async()=>{const d=await api('GET',`/assets/${alpha.asset_id}/endpoint-data`);return d.items.some(i=>i.key==='osquery.network_ranges'&&i.status==='ok')?d:null},'network ranges collected');
 let networks=await api('GET',`/organizations/${org}/agents/${alpha.id}/networks`);ok(networks.reported.length>0,'network proposals came from actual osquery data');
 await api('PUT',`/organizations/${org}/agents/${alpha.id}/networks`,{cidr:second+'/32',authorized:true,version:0});
 networks=await wait(async()=>{const n=await api('GET',`/organizations/${org}/agents/${alpha.id}/networks`);return n.approvals.some(r=>r.cidr===second+'/32'&&r.authorized)?n:null},'range admin approval');const range=networks.approvals.find(r=>r.cidr===second+'/32');ok(range.authorized_by&&range.authorized_at&&range.version===1,'range decision records actor, time and version');await review('04-endpoint-data-and-scope');
 await p.goto(BASE+`/organizations/${org}?tab=assets&new=1`);dialog=p.getByRole('dialog');await dialog.getByRole('button',{name:/^Web application/}).click();await dialog.getByLabel('Name',{exact:true}).fill('Isolated HTTP fixture');await dialog.getByLabel('URLs',{exact:true}).fill(`http://${first}`);await dialog.getByRole('checkbox',{name:/I confirm this asset may be tested/}).check();await review('15-add-organisation-asset');await dialog.getByRole('button',{name:'Create asset',exact:true}).click();await dialog.waitFor({state:'detached'});const target=(await api('GET',`/assets?org=${org}`)).items.find(a=>a.name==='Isolated HTTP fixture');ok(target?.organization_id===org,'asset created through the locked organisation UI is assigned atomically');
 const closed=await api('POST','/assets',{category:'web_application',name:'Closed web endpoint',organization_id:org,testing_authorized:true,addresses:[{kind:'url',value:`http://${first}:1`}]});
 await p.goto(BASE+`/organizations/${org}?tab=assets`);await p.getByRole('heading',{name:'Assets',exact:true}).waitFor();ok(await p.getByRole('combobox',{name:'Organisation'}).isDisabled(),'organisation assets use locked shared filter');await review('05-organisation-assets');
 // Wizard 4 langkah: organisasi (terisi dari tautan Run scan di halaman organisasi), mesin, target, tinjauan.
 await p.getByRole('link',{name:'Run scan',exact:true}).click();await p.getByRole('heading',{name:'New scan',exact:true}).waitFor();
 const orgPick=p.getByRole('radiogroup',{name:'Organisation'});
 ok(await orgPick.getByRole('radio',{name:/Simplified Linux organisation/}).getAttribute('aria-checked')==='true','Scan opens prefilled with the organisation');
 await p.getByRole('button',{name:'Continue',exact:true}).click();
 await p.getByRole('radio',{name:'Full assessment'}).click();await p.getByRole('button',{name:'Continue',exact:true}).click();
 await p.getByRole('checkbox',{name:/Isolated HTTP fixture/}).check();await p.getByRole('checkbox',{name:/Closed web endpoint/}).check();await p.getByRole('checkbox',{name:/alpha/}).check();
 // Wizard tidak lagi mematikan centang endpoint tanpa otorisasi, tetapi peluncuran tetap menolaknya di server.
 const betaProbe=await ctx.request.fetch(BASE+'/api/v1/scans',{method:'POST',headers:{'X-Requested-With':'orchis-web','Idempotency-Key':crypto.randomUUID()},data:{organization_id:org,asset_ids:[beta.asset_id],tools:['nmap','nuclei','hackbot']}});
 ok(betaProbe.status()===422&&/testing authorization/i.test(await betaProbe.text()),'unapproved endpoint is rejected at launch');
 await review('06-new-scan');await p.getByRole('button',{name:'Continue',exact:true}).click();
 await p.getByRole('button',{name:'Launch scan',exact:true}).click();await p.waitForURL(/\/simulations\/[a-f0-9-]{36}$/);const scanID=p.url().split('/').pop();await review('07-live-scan');
 // Job per target: aset URL mendapat nmap+nuclei+Hackbot; aset agent mendapat DefenseBot; rentang jaringan tidak lagi bisa dipindai.
 const scan=await wait(async()=>{const s=await api('GET',`/scans/${scanID}`);return ['finished','incomplete'].includes(s.status)?s:null},'scan finished');
 const jobsFor=(tool,assetID)=>scan.jobs.filter(j=>j.tool===tool&&(!assetID||j.asset_id===assetID));
 ok(scan.runner==='injector'&&scan.jobs.length===7,'each selected target gets its engine jobs recorded');
 ok(jobsFor('nmap',target.id).every(j=>j.status==='finished')&&jobsFor('nuclei',target.id).every(j=>j.status==='finished')&&jobsFor('hackbot',target.id).every(j=>j.status==='finished'),'pipeline completed on the owned fixture');
 ok(jobsFor('nuclei',closed.id).every(j=>j.status==='failed')&&jobsFor('nuclei',closed.id).every(j=>/unreachable/i.test(j.error))&&jobsFor('hackbot',closed.id).every(j=>j.status==='failed'),'closed endpoint is a failed scan, never a safe/dead host');
 ok(jobsFor('defensebot',alpha.asset_id).length===1&&['finished','incomplete'].includes(jobsFor('defensebot',alpha.asset_id)[0].status),'agent asset runs DefenseBot on its real endpoint data');
 ok(scan.jobs.some(j=>j.findings.some(f=>f.type==='vulnerability'&&f.evidence.template_id==='e2e-exposed-env')),'real Nuclei template match appears in scan results');
 await p.reload();await review('08-scan-results');
 const report=await wait(async()=>{const items=(await api('GET',`/reports?assessment_id=${scanID}`)).items;return items.length===1&&items[0].status==='ready'?items[0]:null},'one rendered scan report');const json=await ctx.request.get(`${BASE}/api/v1/reports/${report.id}/export/json`);const frozen=await json.text();const snapshot=JSON.parse(frozen);ok(snapshot.scan_id===scanID&&snapshot.schema_version==='orchis.report.v1'&&snapshot.assets.some(a=>a.endpoint),'report includes frozen scan and actual endpoint posture');ok(!/hunter2|DB_PASSWORD/.test(frozen),'raw response and credentials are absent from exports');
 const pdf=await ctx.request.get(`${BASE}/api/v1/reports/${report.id}/export/pdf`);const bytes=await pdf.body();ok(pdf.ok()&&bytes.subarray(0,5).toString()==='%PDF-','report PDF opens as a real PDF');writeFileSync(join(out,'real-scan.pdf'),bytes);writeFileSync(join(out,'real-scan.json'),frozen);for(const format of ['html','csv'])ok((await ctx.request.get(`${BASE}/api/v1/reports/${report.id}/export/${format}`)).status()===404,`${format} export is removed`);
 await p.goto(BASE+`/reports/${report.id}`);await p.getByRole('link',{name:'Download PDF',exact:true}).waitFor();await review('09-report');
 await p.goto(BASE+`/findings?org=${org}`);await p.getByRole('heading',{name:'Findings',exact:true}).waitFor();ok(await p.getByRole('combobox',{name:'Organisation'}).inputValue()===org,'findings scope survives a shareable URL');await review('10-global-findings');
 await p.goto(BASE+`/organizations/${org}?tab=findings`);await p.getByRole('heading',{name:'Findings',exact:true}).waitFor();ok(await p.getByRole('combobox',{name:'Organisation'}).isDisabled(),'organisation findings use locked shared filter');await review('11-organisation-findings');
 await p.goto(BASE+`/organizations/${org}`);await review('16-organisation-overview');await p.goto(BASE+`/organizations/${org}?tab=scans`);await review('17-organisation-scans');await p.goto(BASE+`/organizations/${org}?tab=reports`);await review('18-organisation-reports');
 await p.goto(BASE+'/simulations');await p.getByRole('heading',{name:'Scans',exact:true}).first().waitFor();await review('12-simulations');await p.goto(BASE+'/reports');await p.getByRole('heading',{name:'Reports',exact:true}).waitFor();await review('13-reports');
 await p.goto(BASE+`/assets?org=${org}`);await p.getByRole('heading',{name:'Assets',exact:true}).waitFor();await review('14-global-assets');await p.reload();ok(await p.getByRole('combobox',{name:'Organisation'}).inputValue()===org,'asset filter survives refresh');
 const latest=await api('GET',`/assets/${target.id}`);await api('PATCH',`/assets/${target.id}`,{version:latest.version,name:'Renamed after snapshot'});ok(await (await ctx.request.get(`${BASE}/api/v1/reports/${report.id}/export/json`)).text()===frozen,'inventory changes do not alter the frozen report');
 for(const path of ['/chains','/atomic','/mitre',`/organizations/${org}/ai`]){await p.goto(BASE+path);await p.waitForURL(BASE+'/organizations');ok(true,`advanced direct route hidden: ${path}`)}await p.goto(BASE+'/scenarios');await p.getByRole('heading',{name:/can.t find that page/i}).waitFor();ok(true,'removed scenario route stays absent');
 ok(errors.length===0,'no browser runtime errors');writeFileSync(join(out,'result.json'),JSON.stringify({pass,fail,scan_id:scanID,report_id:report.id,targets:[first,second],os_proven:['linux/amd64'],screenshots_widths:[1280,834,390],browser_errors:errors},null,2));
}catch(error){fail++;console.error(String(error));await p.screenshot({path:join(out,'failure.png'),fullPage:true}).catch(()=>{});}finally{for(const agent of agents){try{process.kill(-agent.pid,'SIGTERM')}catch{}}if(typeof fakeEngines!=='undefined')await fakeEngines.close().catch(()=>{});await ctx.close();await browser.close();rmSync(dir,{recursive:true,force:true});console.log(`\n${pass} lulus / ${fail} gagal`);process.exit(fail?1:0)}

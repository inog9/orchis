// Three-role product policy: real accounts, API refusals, navigation and accessible UI.
import assert from 'node:assert/strict';
import { mkdirSync } from 'node:fs';
import { chromium } from 'playwright';
import AxeBuilder from '@axe-core/playwright';
const BASE=process.env.E2E_BASE??'http://127.0.0.1:18090';
const SHOTS='e2e/shots/roles/';mkdirSync(SHOTS,{recursive:true});
const browser=await chromium.launch();let checks=0;
const check=(v,msg)=>{assert(v,msg);checks++;console.log('LULUS: '+msg);};
async function login(page,email,password){await page.goto(BASE+'/login');await page.getByLabel('Email').fill(email);await page.getByLabel('Password',{exact:true}).fill(password);await page.getByRole('button',{name:'Sign in',exact:true}).click();}
async function audit(page,label){for(const width of [1280,834,390]){await page.setViewportSize({width,height:900});const result=await new AxeBuilder({page}).withTags(['wcag2a','wcag2aa','wcag21a','wcag21aa','wcag22aa','best-practice']).analyze();check(!result.violations.length,`${label} axe0 @${width}: ${result.violations.map(v=>v.id)}`);check(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),`${label} no overflow @${width}`);await page.screenshot({path:SHOTS+`${label}-${width}.png`,fullPage:true});}}
try {
 const admin=await browser.newContext();const page=await admin.newPage();await login(page,'admin@orchis.local','Orchis-Dev-Pass-1');await page.waitForURL(BASE+'/organizations');
 const headers={'X-Requested-With':'orchis-web'};
 const roles=await (await admin.request.get(BASE+'/api/v1/roles')).json();assert.deepEqual(roles.items.map(r=>r.key),['admin','analyst','viewer']);checks++;
 for(const role of ['viewer','analyst']){
  const created=await admin.request.post(BASE+'/api/v1/users',{headers,data:{email:`role-${role}@example.test`,display_name:`Role ${role}`,role}});check(created.status()===201,`${role} account created`);const c=await created.json();
  const reset=await admin.request.post(BASE+'/api/v1/auth/reset',{headers,data:{token:c.reset_link.token,password:'Role-Test-Password-42'}});check(reset.status()===204,`${role} password set once`);
  const ctx=await browser.newContext();const p=await ctx.newPage();const failures=[],errors=[];p.on('response',r=>{if(r.status()===403)failures.push(r.url());});p.on('pageerror',e=>errors.push(e.message));
  await login(p,`role-${role}@example.test`,'Role-Test-Password-42');await p.waitForURL(BASE+(role==='viewer'?'/findings':'/organizations'));
  if(role==='viewer'){
   await p.getByRole('heading',{name:'Findings',exact:true}).waitFor();
   assert.deepEqual(await p.getByRole('navigation',{name:'Primary',exact:true}).getByRole('link').evaluateAll(nodes=>nodes.map(n=>new URL(n.href).pathname)),['/findings','/reports']);checks++;
   await audit(p,'viewer-findings');await p.getByRole('link',{name:'Reports',exact:true}).click();await p.getByRole('heading',{name:'Reports',exact:true}).waitFor();await p.getByText('No reports yet',{exact:true}).waitFor();check(await p.getByRole('link',{name:'Open organisations',exact:true}).count()===0,'Viewer report empty state offers no unavailable organisation action');await audit(p,'viewer-reports');
   for(const path of ['/organizations','/assets','/assets/agents','/simulations','/settings/users']){await p.goto(BASE+path);await p.getByRole('heading',{name:'Access needed',exact:true}).waitFor();check(true,'Viewer direct route denied before fetching '+path);}
  }
  check(failures.length===0,`${role} UI makes no unauthorized requests: ${failures}`);check(errors.length===0,`${role} no browser runtime errors: ${errors}`);
  // Server refusal is verified separately from presentation, using a valid authorized flag.
  const auth=await ctx.request.post(BASE+'/api/v1/assets',{headers,data:{name:'must not authorize',category:'host',testing_authorized:true}});check(auth.status()===403,`${role} cannot authorize a scan target`);
  await ctx.close();
 }
 await page.goto(BASE+'/settings/users');await page.getByRole('button',{name:'New user',exact:true}).click();await page.getByRole('dialog').waitFor();assert.deepEqual(await page.getByLabel('Role',{exact:true}).locator('option').allTextContents(),['Admin','Analyst','Viewer']);checks++;await audit(page,'admin-role-options');await admin.close();
}finally{await browser.close();}
console.log(`Three-role policy: ${checks} passed`);

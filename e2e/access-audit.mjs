// UI proof on an isolated local server/database, never a production account.
import {chromium} from 'playwright';
import AxeBuilder from '@axe-core/playwright';
import {mkdir,writeFile} from 'node:fs/promises';
const base=process.env.E2E_BASE||process.env.E2E_ACCESS_URL||'http://127.0.0.1:18190';
const parsed=new URL(base);
if(parsed.hostname!=='127.0.0.1'||parsed.protocol!=='http:')throw new Error('Local fixture only');
const out=process.env.E2E_ACCESS_SHOTS||'e2e/shots/t1-access';await mkdir(out,{recursive:true});
const browser=await chromium.launch();let checks=0;
try{
 const context=await browser.newContext({baseURL:base});
 for(let i=0;i<28;i++){
  const res=await context.request.post('/api/v1/auth/login',{headers:{'X-Requested-With':'orchis-web'},data:{email:`fixture-guess-${i}@invalid.test`,password:'Wrong-Fixture-9'}});
  if(![401,429].includes(res.status()))throw new Error('Unexpected local failure status');checks++;
 }
 const page=await context.newPage();const errors=[];page.on('pageerror',e=>errors.push(e.message));
 await page.goto('/login');await page.getByLabel('Email').fill(process.env.E2E_ACCESS_EMAIL||'admin@orchis.local');await page.getByLabel('Password',{exact:true}).fill(process.env.E2E_ACCESS_PASSWORD||'Orchis-Dev-Pass-1');await page.getByRole('button',{name:'Sign in',exact:true}).click();await page.waitForURL(base+'/organizations');checks++;
 await page.goto('/settings');await page.getByRole('link',{name:'Audit log',exact:true}).click();await page.getByRole('heading',{name:'Audit log',exact:true}).waitFor();checks++;
 const audit=page.getByRole('list',{name:'Audit events'});await audit.locator('li').first().waitFor();
 if(await audit.locator('li').count()!==25)throw new Error('Audit page size is incorrect');checks++;
 if((await audit.innerText()).includes('fixture-guess-'))throw new Error('Guessed identity leaked');checks++;
 await page.getByRole('button',{name:'Next',exact:true}).click();await page.getByText('Page 2',{exact:true}).waitFor();checks++;
 await page.getByRole('button',{name:'Previous',exact:true}).click();await page.getByText('Page 1',{exact:true}).waitFor();
 await page.getByLabel('Event type').selectOption('auth.login_failed');await page.getByText('Page 1',{exact:true}).waitFor();await audit.getByText('Sign-in failed',{exact:true}).first().waitFor();checks++;
 const summaries=[];
 for(const width of [1280,834,390]){
  await page.setViewportSize({width,height:900});await page.evaluate(()=>window.scrollTo(0,0));await page.screenshot({path:`${out}/audit-${width}-top.png`});
  const results=await new AxeBuilder({page}).analyze();if(results.violations.length)throw new Error('Audit axe violations: '+results.violations.map(v=>v.id).join(','));
  const overflow=await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth);if(overflow)throw new Error('Audit horizontal overflow');
  await page.getByRole('button',{name:'Next',exact:true}).scrollIntoViewIfNeeded();await page.screenshot({path:`${out}/audit-${width}-bottom.png`});summaries.push({width,axe:0,overflow:false});checks+=2;
 }
 if(errors.length)throw new Error('Browser errors: '+errors.join(';'));
 await writeFile(`${out}/results.json`,JSON.stringify({checks,summaries},null,2));console.log(JSON.stringify({checks,summaries}));await context.close();
}finally{await browser.close();}

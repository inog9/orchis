// Inventaris visual sebelum perbaikan: database baru, tidak memakai seed.
import {chromium} from 'playwright';
import {mkdirSync,writeFileSync} from 'node:fs';
const base=process.env.E2E_BASE; const shots=new URL('./shots/ux-before/',import.meta.url).pathname;mkdirSync(shots,{recursive:true});
const b=await chromium.launch();const p=await b.newPage();
await p.goto(base+'/login');await p.getByLabel('Email').fill('admin@orchis.local');await p.getByLabel('Password',{exact:true}).fill('Orchis-Dev-Pass-1');await p.getByRole('button',{name:/sign in/i}).click();await p.waitForURL(base+'/');
const rows=[];
for(const width of [1280,834]){await p.setViewportSize({width,height:900});for(const path of ['/','/organizations','/assets','/assets/agents','/actions','/atomic','/scenarios','/chains','/simulations','/findings','/reports','/settings/integrations']){await p.goto(base+path);await p.waitForLoadState('networkidle');rows.push({width,path,text:await p.locator('main').innerText()});await p.screenshot({path:shots+`${width}-${path.replaceAll('/','_')||'home'}.png`,fullPage:true});}}
writeFileSync(shots+'inventory.json',JSON.stringify(rows,null,2));await b.close();

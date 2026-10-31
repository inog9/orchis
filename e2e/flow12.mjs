// Standalone production smoke; owns every project/container/volume it creates.
import {spawnSync} from 'node:child_process';
import {randomBytes,randomInt} from 'node:crypto';
import {createServer} from 'node:net';
import {mkdirSync,readFileSync,writeFileSync} from 'node:fs';
import {join} from 'node:path';
const name='orchis-s-'+Date.now().toString(36)+'-'+randomBytes(2).toString('hex');
const state='/var/lib/orchis-smoke-tests/'+name;
const out=new URL('./shots/production-smoke/',import.meta.url).pathname;mkdirSync(out,{recursive:true});
async function ports(){
 for(let attempt=0;attempt<20;attempt++){
  const port=randomInt(25000,45000);const sockets=[createServer(),createServer()];
  const results=await Promise.allSettled(sockets.map((server,i)=>new Promise((resolve,reject)=>{server.once('error',reject);server.listen(port+i,'127.0.0.1',resolve)})));
  await Promise.all(sockets.filter(s=>s.listening).map(s=>new Promise(resolve=>s.close(resolve))));
  if(results.every(r=>r.status==='fulfilled'))return port;
 }
 throw new Error('No free local HTTPS port pair');
}
function command(args,options={}){const result=spawnSync(args[0],args.slice(1),{encoding:'utf8',timeout:180000,...options});if(result.status!==0)throw new Error('Local smoke command failed: '+args[0]);return result.stdout??'';}
function cleanup(project){
 // Only exact labels for this run; never prune or match names of unrelated stacks.
 for(const label of ['com.docker.compose.project='+project,'io.orchis.ops-owner='+project]){
  const ids=command(['docker','ps','-aq','--filter','label='+label]).trim().split(/\s+/).filter(Boolean);
  if(ids.length)command(['docker','rm','-f',...ids]);
 }
 for(const [kind,list,remove] of [['volume',['docker','volume','ls','-q'],['docker','volume','rm']],['network',['docker','network','ls','-q'],['docker','network','rm']]]){
  const ids=command([...list,'--filter','label=com.docker.compose.project='+project]).trim().split(/\s+/).filter(Boolean);
  if(ids.length)command([...remove,...ids]);
 }
}
let failure;
try{
 const port=await ports();
 command(['python3','deploy/prod/ops.py','smoke','--state',state,'--project',name,'--port',String(port)],{stdio:'inherit',timeout:1200000});
 const result=JSON.parse(readFileSync(join(state,'smoke-result.json'),'utf8'));
 if(result.restore!=='verified'||result.ai_rekey!=='verified'||result.tested_assets!==1||result.skipped_assets!==2)throw new Error('Incomplete production smoke proof');
 command(['docker','run','--rm','--label','io.orchis.ops-owner='+name,'--network',name+'_backend','--read-only','--cap-drop','ALL','--security-opt','no-new-privileges:true','--memory','96m','--pids-limit','32','--cpus','0.5','--user','65534:65534','-v',join(state,'metrics_token')+':/run/secrets/metrics_token:ro','-v',new URL('../deploy/prod/monitoring/probe.mjs',import.meta.url).pathname+':/probe.mjs:ro','--entrypoint','node','node:24.22.0-alpine3.24@sha256:44fce696f38ba8c987c4db89954051000bbd05ffd8d8659ba15ed4a8602e937f','/probe.mjs'],{stdio:'inherit'});
 writeFileSync(join(out,'result.json'),JSON.stringify(result,null,2));console.log('Production smoke: real scans, frozen snapshot/PDF, encrypted backup and isolated restore verified');
}catch(error){failure=error;}
finally{
 for(const project of [name,name+'-restored']){try{cleanup(project);}catch(error){failure??=error;}}
}
if(failure)throw failure;
// Encrypted archive + out-of-band key stay in private /var/lib state, outside git.

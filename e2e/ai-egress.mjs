// Bukti egress lokal: tidak menghubungi provider asli atau tujuan publik.
import {execFileSync} from 'node:child_process';
const tag=Date.now(),network=`orchis-ai-net-${tag}`,proxy=`orchis-ai-proxy-${tag}`,worker=`orchis-ai-worker-${tag}`;
const run=args=>execFileSync('docker',args,{encoding:'utf8',stdio:['ignore','pipe','pipe']});let count=0;
try{
 run(['network','create','--internal',network]);
 const flags=['--network',network,'--read-only','--cap-drop','ALL','--security-opt','no-new-privileges','--pids-limit','32','--memory','128m'];
 run(['run','-d','--rm','--name',proxy,...flags,'--entrypoint','/usr/local/bin/ai-proxy','orchis-ai:dev']);
 run(['run','-d','--rm','--name',worker,...flags,'-e',`ORCHIS_SERVER_URL=http://${proxy}:8080`,'-e','ORCHIS_AI_WORKER_TOKEN=local-only-egress-fixture-token-32','-e',`ORCHIS_AI_PROXY=http://${proxy}:8080`,'orchis-ai:dev']);
 const n=JSON.parse(run(['network','inspect',network]))[0],w=JSON.parse(run(['inspect',worker]))[0];if(!n.Internal||w.Config.User!=='65532:65532'||!w.HostConfig.ReadonlyRootfs||!w.HostConfig.CapDrop.includes('ALL')||!w.HostConfig.SecurityOpt.includes('no-new-privileges')||w.Mounts.length!==0)throw new Error('Sandbox configuration weakened');count++;console.log('LULUS: worker non-root/read-only/no capabilities/no mounts pada network internal');
 for(const host of ['example.invalid:443','169.254.169.254:80','127.0.0.1:8090','api.openai.com:80']){
  const response=run(['run','--rm','--network',network,'--read-only','--cap-drop','ALL','alpine:3.23','sh','-c',`printf 'CONNECT ${host} HTTP/1.1\\r\\nHost: ${host}\\r\\n\\r\\n' | nc -w 2 ${proxy} 8080`]);
  if(!response.startsWith('HTTP/1.1 403'))throw new Error('Proxy accepted forbidden destination: '+host);count++;console.log('LULUS: proxy menolak '+host+' tanpa dial ke tujuan');
 }
 let shell=false;try{run(['exec',worker,'/bin/sh','-c','id']);shell=true;}catch{}if(shell)throw new Error('Worker image unexpectedly contains a shell');count++;console.log('LULUS: image scratch tidak memiliki shell');
 console.log(`AI egress: ${count} lulus, 0 gagal`);
}finally{for(const name of [worker,proxy])try{run(['rm','-f',name]);}catch{}try{run(['network','rm',network]);}catch{}}

import {readFileSync} from 'node:fs';
const token=readFileSync('/run/secrets/metrics_token','utf8').trim();
const headers={Authorization:'Bearer '+token};
const anonymous=await fetch('http://server:8090/api/v1/auth/me');if(anonymous.status!==401)throw new Error('Anonymous identity request should be rejected');
for(const host of ['server','nmap','nuclei','ai-worker']){
 let res=await fetch('http://'+host+':9101/metrics');if(res.status!==401)throw new Error(host+' metrics accepts anonymous');
 res=await fetch('http://'+host+':9101/metrics',{headers});if(res.status!==200)throw new Error(host+' metrics failed');
 const body=await res.text();
 if(host==='server'){
  for(const line of ['orchis_metrics_database_up 1','orchis_metrics_queue_up 1','orchis_disk_sample_up 1','orchis_backup_status_known 1','orchis_backup_last_success 1'])if(!body.includes(line))throw new Error('Missing server gauge: '+line);
  if(!body.includes('orchis_database_bytes ')||!body.includes('orchis_http_requests_total{route="auth"'))throw new Error('HTTP/DB metrics missing');
 }else{const time=Number(body.match(/orchis_worker_last_success_timestamp_seconds ([\d.e+]+)/)?.[1]);if(!time||Date.now()/1000-time>60)throw new Error(host+' heartbeat stale');}
 console.log(host+': private metrics and current samples verified');
}
if(process.env.ORCHIS_PROBE_PROMETHEUS==='1'){
const targets=await (await fetch('http://prometheus:9090/api/v1/targets')).json();
if(targets.status!=='success'||targets.data.activeTargets.length!==4||targets.data.activeTargets.some(t=>t.health!=='up'))throw new Error('Prometheus targets not all healthy');
const rules=await (await fetch('http://prometheus:9090/api/v1/rules')).json();
if(rules.status!=='success'||rules.data.groups.flatMap(g=>g.rules).length!==8)throw new Error('Expected eight alert rules');
console.log('Prometheus: all four private targets up; eight alert rules loaded');

}

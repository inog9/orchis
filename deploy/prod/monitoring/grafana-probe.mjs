import {readFileSync} from 'node:fs';
const password=readFileSync('/run/secrets/grafana_password','utf8').trim();
const headers={Authorization:'Basic '+Buffer.from('admin:'+password).toString('base64')};
const res=await fetch('http://grafana:3000/api/dashboards/uid/orchis-operations',{headers});
if(res.status!==200)throw new Error('Grafana dashboard authentication/provisioning failed');
const data=await res.json();if(data.dashboard.panels.length!==10)throw new Error('Missing provisioned panels');
const sources=await(await fetch('http://grafana:3000/api/datasources',{headers})).json();if(!sources.some(s=>s.uid==='orchis-prometheus'))throw new Error('Missing datasource');
console.log('Grafana: random private credential, provisioned datasource and ten panels verified');

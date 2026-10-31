import {readFileSync} from 'node:fs';
import {createHash} from 'node:crypto';
const url='http://127.0.0.1:8091/render';
const token=readFileSync(process.env.ORCHIS_RENDERER_TOKEN_FILE,'utf8').trim();
const unauth=await fetch(url,{method:'POST',body:'<h1>Untrusted</h1>'});
if(unauth.status!==401)throw new Error('Missing renderer authentication');
const res=await fetch(url,{method:'POST',headers:{Authorization:'Bearer '+token,'Content-Type':'text/html'},body:'<!doctype html><html><body><h1>ORCHIS production renderer probe</h1><p>Local infrastructure fixture.</p><img src="http://198.51.100.99/egress-test" alt="Blocked external resource"></body></html>',signal:AbortSignal.timeout(60000)});
const pdf=Buffer.from(await res.arrayBuffer());
if(res.status!==200||!pdf.subarray(0,5).equals(Buffer.from('%PDF-')))throw new Error('PDF probe failed');
let blocked=false;
try{await fetch('http://1.1.1.1',{signal:AbortSignal.timeout(2000)});}catch{blocked=true;}
if(!blocked)throw new Error('Renderer network has external egress');
console.log(JSON.stringify({renderer:'ok',unauthenticated_status:unauth.status,pdf_bytes:pdf.length,pdf_sha256:createHash('sha256').update(pdf).digest('hex'),external_egress:'blocked'}));

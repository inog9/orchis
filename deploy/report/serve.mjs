// Internal renderer: one bounded child process per request; never logs document data.
import http from 'node:http';
import {readFileSync} from 'node:fs';
import {spawn} from 'node:child_process';
import {timingSafeEqual} from 'node:crypto';
import {fileURLToPath} from 'node:url';
const token = readFileSync(process.env.ORCHIS_RENDERER_TOKEN_FILE, 'utf8').trim();
if(token.length < 32) throw new Error('Renderer token is required');
let busy = false;
const server = http.createServer(async (req,res) => {
  if(req.method === 'GET' && req.url === '/healthz') { res.writeHead(200).end('ok'); return; }
  const supplied = Buffer.from(req.headers.authorization || '');
  const expected = Buffer.from('Bearer '+token);
  if(supplied.length !== expected.length || !timingSafeEqual(supplied,expected)) {res.writeHead(401).end(); return;}
  if(req.method !== 'POST' || req.url !== '/render') {res.writeHead(404).end(); return;}
  if(busy) {res.writeHead(503).end(); return;}
  busy=true;
  let child;
  const abort=()=>child?.kill('SIGKILL');
  res.on('close',abort);
  try {
    let size=0; const chunks=[];
    for await(const chunk of req) {
      size+=chunk.length;
      if(size>16*1024*1024) {res.writeHead(413).end(); return;}
      chunks.push(chunk);
    }
    const pdf = await new Promise((resolve,reject) => {
      child=spawn(process.execPath,[fileURLToPath(new URL('./render.mjs',import.meta.url))],{stdio:['pipe','pipe','ignore']});
      let bytes=0;const out=[];
      const timer=setTimeout(()=>{abort();reject(new Error('timeout'));},55000);
      child.stdout.on('data',c=>{bytes+=c.length;if(bytes>32*1024*1024){abort();reject(new Error('output limit'));}else out.push(c);});
      child.on('error',e=>{clearTimeout(timer);reject(e);});
      child.on('close',code=>{clearTimeout(timer);code===0?resolve(Buffer.concat(out)):reject(new Error('renderer failed'));});
      child.stdin.on('error',()=>{});
      child.stdin.end(Buffer.concat(chunks));
    });
    if(!pdf.subarray(0,5).equals(Buffer.from('%PDF-')))throw new Error('invalid PDF');
    res.writeHead(200,{'Content-Type':'application/pdf','Content-Length':pdf.length}).end(pdf);
  } catch {if(!res.headersSent)res.writeHead(502).end('Rendering failed');}
  finally {abort();busy=false;res.off('close',abort);}
});
server.requestTimeout=65000;server.headersTimeout=10000;server.maxHeadersCount=30;
server.listen(8091,'0.0.0.0');
for(const signal of ['SIGTERM','SIGINT'])process.on(signal,()=>{server.close();setTimeout(()=>process.exit(0),2000).unref();});

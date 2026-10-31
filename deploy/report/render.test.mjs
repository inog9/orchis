// Uji egress memakai listener lokal sungguhan; renderer tidak boleh menghubunginya.
import { createServer } from 'node:http';
import { spawn } from 'node:child_process';
let requests=0;
const server=createServer((req,res)=>{requests++;res.end('should not be read');});
await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
const port=server.address().port;
try{
 const worker=spawn(process.execPath,[new URL('./render.mjs',import.meta.url).pathname],{stdio:['pipe','pipe','pipe']});
 const chunks=[];let errors='';worker.stdout.on('data',d=>chunks.push(d));worker.stderr.on('data',d=>errors+=d);
 worker.stdin.end(`<html><head><style>@import url(http://127.0.0.1:${port}/style);</style></head><body><h1>Egress guard</h1><img src="http://127.0.0.1:${port}/image"><script>fetch('http://127.0.0.1:${port}/script')</script></body></html>`);
 const code=await new Promise(resolve=>worker.on('close',resolve));
 if(code!==0||!Buffer.concat(chunks).subarray(0,5).equals(Buffer.from('%PDF-')))throw new Error(errors||'Renderer failed');
 if(requests!==0)throw new Error(`Renderer made ${requests} external requests`);
 console.log('LULUS: renderer membuat PDF tanpa akses HTTP gambar/CSS/JavaScript');
}finally{server.close();}

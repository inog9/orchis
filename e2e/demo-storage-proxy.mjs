// Owned E2E fixture only. Fail DELETE requests without stopping shared MinIO.
import http from 'node:http';
import {existsSync,writeFileSync} from 'node:fs';
const [portFile,faultFile]=process.argv.slice(2);
if(!portFile||!faultFile)throw new Error('Missing private fixture state paths');
http.createServer((req,res)=>{
 if(req.method==='DELETE'&&existsSync(faultFile)){
  res.writeHead(503,{'Content-Type':'application/xml'});
  res.end('<Error><Code>ServiceUnavailable</Code><Message>LOCAL synthetic cleanup fixture</Message></Error>');return;
 }
 const upstream=http.request({hostname:'127.0.0.1',port:19000,method:req.method,path:req.url,headers:req.headers},reply=>{res.writeHead(reply.statusCode??502,reply.headers);reply.pipe(res)});
 upstream.on('error',()=>{if(!res.headersSent)res.writeHead(502);res.end()});
 req.on('aborted',()=>upstream.destroy());req.pipe(upstream);
}).listen(0,'127.0.0.1',function(){writeFileSync(portFile,String(this.address().port),{mode:0o600})});

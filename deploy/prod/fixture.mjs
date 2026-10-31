// Synthetic local service used only by the explicit production smoke recipe.
// Scanner findings must come from requests to this service, never fabricated rows.
import {createServer} from 'node:http';
createServer((req,res)=>{
 if(req.url==='/.env'){
  res.setHeader('Content-Type','text/plain');
  res.end('APP_NAME=ORCHIS_LOCAL_FIXTURE\nAPP_ENV=local\nAPP_DEBUG=true\nDB_HOST=fixture\nDB_DATABASE=fixture\nDB_USERNAME=fixture\nDB_PASSWORD=synthetic-local-fixture-only\n');return;
 }
 if(req.url!=='/') {res.writeHead(404).end('Not found');return;}
 res.setHeader('Content-Type','text/html');res.end('<h1>ORCHIS LOCAL SMOKE FIXTURE</h1>');
}).listen(8081,'0.0.0.0',()=>console.log(JSON.stringify({level:'info',event:'local_fixture_ready',port:8081})));

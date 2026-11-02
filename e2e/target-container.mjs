// Fixture milik e2e pada network namespace container; tidak membuka port host.
import { createServer } from 'node:http';
createServer((req,res)=>{res.setHeader('Content-Type','text/plain');res.end(req.url==='/.env'?'DB_PASSWORD=hunter2\nE2E_SECRET_MARKER=1\n':'Orchis isolated scan fixture\n');}).listen(80,'0.0.0.0');

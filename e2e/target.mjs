// Sasaran scan untuk e2e: server HTTP kecil di 127.0.0.1. Port terbuka untuk nmap; /.env bocor untuk template nuclei e2e.
import { createServer } from 'node:http';
const port = Number(process.argv[2] ?? 18081);
createServer((req, res) => {
  if (req.url === '/.env') { res.end('DB_PASSWORD=hunter2\nE2E_SECRET_MARKER=1\n'); return; }
  res.end('<h1>e2e target</h1>');
}).listen(port, '127.0.0.1', () => console.log('target siap di', port));

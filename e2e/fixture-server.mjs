// Menyajikan bundel ATT&CK kecil untuk e2e (tanpa internet): node fixture-server.mjs <port>
import { createServer } from 'node:http';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
const body = readFileSync(fileURLToPath(new URL('./fixtures/attack-bundle.json', import.meta.url)));
const port = Number(process.argv[2] ?? 18082);
createServer((req, res) => { res.setHeader('content-type', 'application/json'); res.end(body); }).listen(port, '127.0.0.1', () => console.log('fixture siap di', port));

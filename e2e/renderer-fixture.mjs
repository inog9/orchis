// Fault injection hanya untuk server disposable flow11; renderer produk tidak mengenal flag ini.
import {existsSync} from 'node:fs';
const flag=process.env.ORCHIS_E2E_RENDER_FAILURE_FILE;
if(!flag||!/^\/tmp\/orchis-e2e-render-[a-zA-Z0-9]+$/.test(flag))throw new Error('Missing isolated render fixture');
if(existsSync(flag)){process.stderr.write('LOCAL FIXTURE: renderer process unavailable\n');process.exit(2);}
await import('../deploy/report/render.mjs');

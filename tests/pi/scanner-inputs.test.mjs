import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import crypto from 'node:crypto';
import {importScannerInputs} from '../../docker/scanner-inputs.mjs';
function fixture(t){
 const root=fs.mkdtempSync(path.join(os.tmpdir(),'scanner-handoff-'));t.after(()=>fs.rmSync(root,{recursive:true,force:true}));
 const source=path.join(root,'source'),workspace=path.join(root,'workspace');fs.mkdirSync(source);fs.mkdirSync(workspace);
 const manifest={schema:1,sources:[]};
 for(const tool of ['nmap','nuclei']){
  const data=Buffer.from(JSON.stringify({tool,status:'finished',target:'https://fixture.test',findings:[{title:tool+' seed'}]}));
  const name=tool+'.json';fs.writeFileSync(path.join(source,name),data);manifest.sources.push({name,bytes:data.length,sha256:crypto.createHash('sha256').update(data).digest('hex')});
 }
 fs.writeFileSync(path.join(source,'manifest.json'),JSON.stringify(manifest));return {root,source,workspace,manifest};
}
test('imports both scanner findings next to Hackbot work in its private engagement workspace',t=>{
 const f=fixture(t),folder=importScannerInputs(f.source,f.workspace);
 for(const tool of ['nmap','nuclei']){const file=path.join(folder,tool+'.json');assert.equal(JSON.parse(fs.readFileSync(file)).findings[0].title,tool+' seed');assert.equal(fs.statSync(file).mode&0o777,0o600)}
 assert.equal(folder,path.join(fs.realpathSync(f.workspace),'inputs','scanners'));assert.equal(importScannerInputs(f.source,f.workspace),folder);
});
test('refuses a modified scanner result rather than silently replacing evidence',t=>{
 const f=fixture(t);fs.appendFileSync(path.join(f.source,'nuclei.json'),' ');assert.throws(()=>importScannerInputs(f.source,f.workspace),/scanner_input_changed/);
});
test('refuses unsafe names, input symlinks and destination symlinks',t=>{
 const f=fixture(t);f.manifest.sources[0].name='../escape';fs.writeFileSync(path.join(f.source,'manifest.json'),JSON.stringify(f.manifest));assert.throws(()=>importScannerInputs(f.source,f.workspace),/invalid_scanner_input_name/);
 const g=fixture(t);fs.unlinkSync(path.join(g.source,'nmap.json'));fs.symlinkSync(path.join(g.source,'nuclei.json'),path.join(g.source,'nmap.json'));assert.throws(()=>importScannerInputs(g.source,g.workspace),/invalid_scanner_input_file/);
 const h=fixture(t);fs.symlinkSync(h.source,path.join(h.workspace,'inputs'));assert.throws(()=>importScannerInputs(h.source,h.workspace),/scanner_destination/);
});
test('retains failure metadata and refuses corrupt existing handoffs on resume',t=>{
 const f=fixture(t);const file=path.join(f.source,'nmap.json');const data=Buffer.from(JSON.stringify({tool:'nmap',status:'failed',target:'https://fixture.test',error:'scanner unavailable',findings:[]}));fs.writeFileSync(file,data);f.manifest.sources[0]={name:'nmap.json',bytes:data.length,sha256:crypto.createHash('sha256').update(data).digest('hex')};fs.writeFileSync(path.join(f.source,'manifest.json'),JSON.stringify(f.manifest));
 const folder=importScannerInputs(f.source,f.workspace);assert.equal(JSON.parse(fs.readFileSync(path.join(folder,'nmap.json'))).error,'scanner unavailable');
 fs.writeFileSync(path.join(folder,'nmap.json'),'changed');assert.throws(()=>importScannerInputs(f.source,f.workspace),/existing_scanner_inputs_changed/);
});

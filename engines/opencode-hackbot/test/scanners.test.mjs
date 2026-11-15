import {test} from 'node:test';
import assert from 'node:assert/strict';
import crypto from 'node:crypto';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {copyScannerInputs} from '../src/scanners.mjs';

test('copies scanner inputs only when their manifest hashes match', () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'orchis-scanners-'));
  try {
    const source = path.join(root,'source'), workspace = path.join(root,'workspace');
    fs.mkdirSync(source); fs.mkdirSync(workspace);
    const sources = ['nmap','nuclei'].map(tool => {
      const data = Buffer.from(JSON.stringify({tool,status:'finished',findings:[]}));
      const name = tool + '.json'; fs.writeFileSync(path.join(source,name),data);
      return {name,bytes:data.length,sha256:crypto.createHash('sha256').update(data).digest('hex')};
    });
    fs.writeFileSync(path.join(source,'manifest.json'), JSON.stringify({schema:1,sources}));
    copyScannerInputs(source,workspace);
    assert.ok(fs.existsSync(path.join(workspace,'inputs/scanners/nmap.json')));
    fs.writeFileSync(path.join(source,'nmap.json'),'{}');
    assert.throws(()=>copyScannerInputs(source,workspace), /scanner_input_changed/);
  } finally { fs.rmSync(root,{recursive:true,force:true}); }
});

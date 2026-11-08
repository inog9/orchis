import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {prepareResult} from '../src/result.mjs';

test('only imports findings with bounded private evidence', () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(),'orchis-result-'));
  try {
    fs.mkdirSync(path.join(root,'private/evidence'),{recursive:true});
    fs.writeFileSync(path.join(root,'private/evidence/proof.txt'),'request and response');
    const finding = {id:'one',title:'Example',severity:'low',value:'https://example.com/',evidence_file:'private/evidence/proof.txt',details:{verdict:'VALIDATED',evidence:'observed response'}};
    const draft = {summary:'summary',report:'report',validation:'validation',blockers:[],findings:[finding,{...finding,id:'two',evidence_file:'../outside'}]};
    const result = prepareResult(draft,root,'session');
    assert.equal(result.disposition,'limited');
    assert.deepEqual(result.findings.map(f=>f.id),['one']);
    assert.equal(result.findings[0].evidence_file,undefined);
    assert.equal(result.blockers.length,1);
    assert.throws(()=>prepareResult({...draft,report:''},root,'session'));
  } finally { fs.rmSync(root,{recursive:true,force:true}); }
});

import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import crypto from 'node:crypto';
import { exportResult } from '../../docker/export-result.mjs';

function fixture(t, mode='complete') {
 const root=fs.mkdtempSync(path.join(os.tmpdir(),'export-fixture-'));
 t.after(()=>fs.rmSync(root,{recursive:true,force:true}));
 const workspace=path.join(root,'workspace');fs.mkdirSync(workspace);
 const artifact=(name,content='fixture evidence')=>{fs.writeFileSync(path.join(workspace,name),content);return {path:name,sha256:crypto.createHash('sha256').update(content).digest('hex')}};
 const outputs=[artifact('test_PENTEST_REPORT.md','# Client report'),artifact('test_VALIDATION_REASON.md','# Validation ledger')];
 const proof=Object.fromEntries(['request','response','control','identity','impact'].map(key=>[key,key+'.txt']));
 const proofEvidence=Object.values(proof).map(name=>artifact(name));
 const candidate={id:'issue',title:'Verified issue',url:'https://fixture.test/path',verdict:'VALIDATED',reproduced:true,replay:'flow-1',reason:'fixture proof',vector:'CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:L/I:N/A:N',score:5.3,proof,proofEvidence,evidence:proofEvidence};
 const state={mode,phase:'report',phases:{recon:{completedAt:'now'},report:{completedAt:'now',outputs}},tasks:{recon:{id:'recon',status:'verified'}},candidates:{issue:candidate,unproven:{id:'unproven',verdict:'NEEDS_MORE_EVIDENCE'}},checklist:[]};
 const save=()=>fs.writeFileSync(path.join(root,'state.json'),JSON.stringify(state));save();
 return {root,workspace,sessionId:'fixture',state,save};
}
test('exports verified structured findings and both immutable reports',t=>{
 const f=fixture(t);const output=exportResult(f,'complete');const result=JSON.parse(fs.readFileSync(output));
 assert.equal(result.findings.length,1);assert.equal(result.findings[0].severity,'medium');assert.equal(result.report,'# Client report');assert.ok(result.summary.split(/\s+/).length<=300);
 assert.equal(fs.statSync(output).mode&0o777,0o600);
});
test('refuses reports changed after the delivery gate',t=>{
 const f=fixture(t);fs.writeFileSync(path.join(f.workspace,'test_PENTEST_REPORT.md'),'changed');assert.throws(()=>exportResult(f,'complete'),/artifact_changed/);
});
test('refuses missing proof and symlink escapes',t=>{
 const f=fixture(t);f.state.candidates.issue.proofEvidence=[];f.save();assert.throws(()=>exportResult(f,'complete'),/missing_candidate_proof/);
 f.state.candidates={};fs.unlinkSync(path.join(f.workspace,'test_PENTEST_REPORT.md'));fs.symlinkSync(path.join(f.root,'state.json'),path.join(f.workspace,'test_PENTEST_REPORT.md'));f.save();assert.throws(()=>exportResult(f,'complete'),/artifact_outside_workspace/);
});
test('partial delivery synthesizes a clearly labeled report without advancing controller gates',t=>{
 const f=fixture(t,'paused');delete f.state.phases.report;f.save();const result=JSON.parse(fs.readFileSync(exportResult(f,'input_required','interactive_request')));
 assert.match(result.report,/integration-generated report is incomplete/);assert.equal(result.disposition,'input_required');assert.equal(JSON.parse(fs.readFileSync(path.join(f.root,'state.json'))).mode,'paused');
});
test('different validated findings on the same URL keep distinct identities',t=>{
 const f=fixture(t);f.state.candidates.second={...f.state.candidates.issue,id:'second'};f.save();const result=JSON.parse(fs.readFileSync(exportResult(f,'complete')));assert.notEqual(result.findings[0].value,result.findings[1].value);
});
test('terminal delivery requires the controller reports and matching disposition',t=>{
 const f=fixture(t);assert.throws(()=>exportResult(f,'limited'),/terminal_state_mismatch/);f.state.phases.report.outputs=[];f.save();assert.throws(()=>exportResult(f,'complete'),/missing_delivery_reports/);
});

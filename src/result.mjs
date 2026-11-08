import fs from 'node:fs';
import path from 'node:path';

export function prepareResult(draft, root, session) {
  if (!draft || typeof draft.summary !== 'string' || !draft.summary.trim() || typeof draft.report !== 'string' || !draft.report.trim() || typeof draft.validation !== 'string' || !draft.validation.trim() || !Array.isArray(draft.findings) || !Array.isArray(draft.blockers)) throw Error('invalid result draft');
  const blockers = draft.blockers.filter(x => typeof x === 'string' && x.trim()).slice(0,99);
  const evidenceRoot = path.join(fs.realpathSync(root), 'private', 'evidence');
  const findings = [];
  for (const f of draft.findings.slice(0,2000)) {
    try {
      if (!f || typeof f.id !== 'string' || !f.id || typeof f.title !== 'string' || !f.title || Buffer.byteLength(f.title)>300 || typeof f.value !== 'string' || !f.value || Buffer.byteLength(f.value)>500 || !['info','low','medium','high','critical'].includes(f.severity) || f.details?.verdict !== 'VALIDATED' || typeof f.details.evidence !== 'string' || !f.details.evidence.trim() || typeof f.evidence_file !== 'string') throw Error();
      const evidence = fs.realpathSync(path.resolve(root, f.evidence_file));
      if (!evidence.startsWith(evidenceRoot + path.sep)) throw Error();
      const stat = fs.statSync(evidence);
      if (!stat.isFile() || stat.size === 0 || stat.size > 5*1024*1024 || Buffer.byteLength(JSON.stringify(f.details)) > 8192) throw Error();
      findings.push({id:f.id,title:f.title,severity:f.severity,value:f.value,details:f.details});
    } catch { /* Unsupported claims are not imported as findings. */ }
  }
  if (findings.length !== draft.findings.length) blockers.push('Some findings lacked valid private evidence or fields and were excluded.');
  const disposition = blockers.length ? 'limited' : 'complete';
  return {schema:1,session_id:session,disposition,phase:'report',summary:draft.summary.slice(0,16000),phases:['exploration','report'],verified_tasks:0,blockers,findings,report:draft.report.slice(0,500000),validation:draft.validation.slice(0,500000)};
}

// Versioned service result. Markdown is an artifact, never a finding parser.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';

const maxReport = 1024 * 1024;
function bounded(value, bytes) {
 let text = String(value);
 while (Buffer.byteLength(text) > bytes) text = text.slice(0, -1);
 return text;
}
function verifiedText(workspace, entry, maxBytes = maxReport) {
  if (!entry || typeof entry.path !== 'string' || path.isAbsolute(entry.path)) throw new Error('invalid_artifact');
  const root = fs.realpathSync(workspace);
  const file = fs.realpathSync(path.resolve(root, entry.path));
  if (!file.startsWith(root + path.sep)) throw new Error('artifact_outside_workspace');
  const stat = fs.statSync(file);
  if (!stat.isFile() || !stat.size || stat.size > maxBytes) throw new Error('invalid_artifact_size');
  const data = fs.readFileSync(file);
  if (crypto.createHash('sha256').update(data).digest('hex') !== entry.sha256) throw new Error('artifact_changed');
  return data.toString('utf8');
}
export function exportResult(metadata, event, reason = '') {
  const stateFile = path.join(metadata.root, 'state.json');
  const state = fs.existsSync(stateFile) ? JSON.parse(fs.readFileSync(stateFile, 'utf8')) : {};
  const terminal = ['complete', 'limited'].includes(event);
  if (terminal && state.mode !== event) throw new Error('terminal_state_mismatch');
  const phases = Object.keys(state.phases ?? {}).filter(key => state.phases[key].completedAt);
  const verified = Object.values(state.tasks ?? {}).filter(task => task.status === 'verified');
  const findings = [];
  for (const candidate of Object.values(state.candidates ?? {})) {
    if (candidate.verdict !== 'VALIDATED') continue;
    for (const entry of [...(candidate.evidence ?? []), ...(candidate.proofEvidence ?? [])]) verifiedText(metadata.workspace, entry, 64 * 1024 * 1024);
    const proofs = candidate.proofEvidence ?? [];
    if (!candidate.reproduced || !candidate.replay || !['request','response','control','identity','impact'].every(key => proofs.some(p => p.path === candidate.proof?.[key]))) throw new Error('missing_candidate_proof');
    if (typeof candidate.score !== 'number' || candidate.score < 0 || candidate.score > 10) throw new Error('invalid_candidate_score');
    const severity = candidate.score >= 9 ? 'critical' : candidate.score >= 7 ? 'high' : candidate.score >= 4 ? 'medium' : candidate.score > 0 ? 'low' : 'info';
    findings.push({ id: candidate.id, title: bounded(candidate.title || candidate.id, 300), severity,
      value: bounded(candidate.url || candidate.target || '', 400) + '#hackbot:' + crypto.createHash('sha256').update(String(candidate.id)).digest('hex'),
      details: { verdict: 'VALIDATED', reason: bounded(candidate.reason || '', 1200), vector: candidate.vector, score: candidate.score,
        reproduction: bounded(candidate.reproduction || candidate.steps || '', 1500),
        impact: bounded(candidate.impactDescription || '', 1000),
        replay: bounded(candidate.replay, 200), remediation: bounded(candidate.remediation || '', 1200), evidence: proofs.map(({path,sha256}) => ({path,sha256})) } });
  }
  const blockers = [...new Set([
    ...Object.values(state.tasks ?? {}).filter(t => ['blocked','interrupted','failed'].includes(t.status)).map(t => `${t.id}: ${t.status}. ${bounded(t.reason || t.error || "", 500)}`),
    ...(state.checklist ?? []).filter(item => item.status === 'blocked').map(item => `${item.id}: blocked. ${bounded(item.reason || "", 500)}`),
    ...(!terminal ? [reason || event] : []),
  ])].slice(0,100);
  const summary = `Hackbot Lite completed ${verified.length} verified specialist tasks. Completed phases: ${phases.join(', ') || 'none'}. It validated ${findings.length} findings. Delivery: ${event}. ${blockers.length ? `${blockers.length} coverage limitations were recorded; consult the report.` : 'Consult the report for tested coverage and evidence.'}`;
  const outputs = state.phases?.report?.outputs ?? [];
  const reportEntry = outputs.find(entry => entry.path.endsWith('_PENTEST_REPORT.md'));
  const validationEntry = outputs.find(entry => entry.path.endsWith('_VALIDATION_REASON.md'));
  if (terminal && (!reportEntry || !validationEntry)) throw new Error('missing_delivery_reports');
  const report = terminal ? verifiedText(metadata.workspace, reportEntry) : `# Partial pentest report\n\nThis integration-generated report is incomplete.\n\n${summary}\n\n## Coverage limitations\n\n${blockers.map(b => `- ${b}`).join('\n')}\n\n## Validated findings\n\n${findings.map(f => `### ${f.title}\n\nSeverity: ${f.severity}\n\n${f.details.reason}`).join('\n\n') || 'No validated findings available.'}\n`;
  const validation = terminal ? verifiedText(metadata.workspace, validationEntry) : `# Partial validation ledger\n\n${Object.values(state.candidates ?? {}).map(c => `- ${c.id}: ${c.verdict}`).join('\n') || 'No candidates recorded.'}\n`;
  const result = { schema: 1, session_id: metadata.sessionId, disposition: event, phase: state.phase || 'setup', summary, phases, verified_tasks: verified.length, blockers, findings, report, validation };
  const file = path.join(metadata.root, 'private', 'service-result.json');
  fs.mkdirSync(path.dirname(file), { recursive:true, mode:0o700 });
  const temp = `${file}.${process.pid}.tmp`;
  fs.writeFileSync(temp, JSON.stringify(result), { mode:0o600 });
  fs.renameSync(temp, file);
  return file;
}

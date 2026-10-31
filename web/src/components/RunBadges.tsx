import type { ExecutionSummary, Finding } from '../api/types';

const OUTCOME_LABEL: Record<string, string> = { success: 'Succeeded', error: 'Failed', canceled: 'Canceled', timeout: 'Timed out', skipped: 'Skipped' };

export function runLabel(e: ExecutionSummary | null | undefined): string {
  if (!e) return 'Never run';
  if (e.state === 'ready') return 'Queued';
  if (e.state === 'running') return 'Running';
  return OUTCOME_LABEL[e.outcome ?? ''] ?? 'Finished';
}

export function RunBadge({ run }: { run: ExecutionSummary | null | undefined }) {
  const tone = !run ? '' : run.state !== 'ended' ? 'status--live' : run.outcome === 'success' ? 'status--ok' : 'status--bad';
  return <span className={`status ${tone}`}>{runLabel(run)}</span>;
}

const SEV_LABEL: Record<Finding['severity'], string> = { info: 'Info', low: 'Low', medium: 'Medium', high: 'High', critical: 'Critical' };

export function SeverityBadge({ severity }: { severity: Finding['severity'] }) {
  return <span className={`sev sev--${severity}`}>{SEV_LABEL[severity]}</span>;
}

export function when(iso: string | null | undefined): string {
  if (!iso) return '—';
  return new Date(iso).toLocaleString('en-US', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
}

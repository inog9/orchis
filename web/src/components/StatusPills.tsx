import type { Simulation, StepStatus } from '../api/types';

const SIM_LABEL: Record<Simulation['status'], string> = {
  draft: 'Draft', scheduled: 'Scheduled', running: 'Running', paused: 'Paused', finished: 'Finished', canceled: 'Canceled', failed: 'Failed',
};

export function SimBadge({ status }: { status: Simulation['status'] }) {
  const tone = status === 'running' ? 'status--live' : status === 'finished' ? 'status--ok' : status === 'failed' || status === 'canceled' ? 'status--bad' : '';
  return <span className={`status ${tone}`}>{SIM_LABEL[status]}</span>;
}

const STEP_LABEL: Record<StepStatus, string> = {
  waiting: 'Waiting', queued: 'Queued', running: 'Running', done: 'Done', failed: 'Failed', skipped: 'Skipped', blocked: 'Blocked', canceled: 'Canceled',
};

export function StepBadge({ status }: { status: StepStatus }) {
  const tone = status === 'running' ? 'status--live' : status === 'done' ? 'status--ok' : status === 'failed' || status === 'blocked' || status === 'canceled' ? 'status--bad' : '';
  return <span className={`status ${tone}`}>{STEP_LABEL[status] ?? status}</span>;
}

export function ScenarioSeverity({ severity }: { severity: string }) {
  return <span className={`crit crit--${severity}`}>{severity[0]!.toUpperCase() + severity.slice(1)}</span>;
}

export function elapsed(start: string | null, end: string | null): string {
  if (!start) return '—';
  const s = Math.max(0, Math.floor(((end ? new Date(end) : new Date()).getTime() - new Date(start).getTime()) / 1000));
  const h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60), r = s % 60;
  return h > 0 ? `${h}h ${String(m).padStart(2, '0')}m` : m > 0 ? `${m}m ${String(r).padStart(2, '0')}s` : `${r}s`;
}

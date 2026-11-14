import { completionStatus } from '../../lib/scanView';
import { t as textUI, useLocale, fv as formatUI } from '../../i18n/runtime';
import { useHome } from '../../api/hooks';
import { useScans } from '../../api/scans';
import type { AgentRow } from '../../api/types';
import { relativeTime } from '../../lib/relativeTime';
import { StatCard } from './StatCard';
import { SeverityBar } from './OrgCard';
import { StatusBadge } from '../sims/StatusBadge';

export function agentCounts(agents: Pick<AgentRow, 'status'>[]) {
    const live = agents.filter((a) => a.status !== 'revoked');
    return { total: live.length, online: live.filter((a) => a.status === 'online').length, offline: live.filter((a) => a.status === 'offline').length };
}
// Total temuan = semua tingkat keparahan (sama dengan KPI Findings dan kartu organisasi).
export const severityTotal = (by: Record<string, number> | undefined) => Object.values(by ?? {}).reduce((n, v) => n + (v || 0), 0);

export function OrgOverviewStats({ orgID, agents, bySeverity, canAssets, canScans, canFindings }: {
    orgID: string; agents: AgentRow[]; bySeverity: Record<string, number> | undefined; canAssets: boolean; canScans: boolean; canFindings: boolean;
}) {
    useLocale();
    const home = useHome(canAssets);
    const entry = home.data?.organizations?.find((o) => o.id === orgID);
    const scans = useScans(orgID, '', canScans);
    const last = scans.data?.items[0];
    const ac = agentCounts(agents);
    return (<div className="stats orgd__stats" aria-label={textUI("m_orgd_overview")}>
      {canAssets && <StatCard label={textUI("m_orgs_m_assets")} value={entry?.assets} loading={home.isPending}><span className="stat__sub">{entry ? textUI("m_orgd_authorized", [entry.authorized]) : ''}</span></StatCard>}
      <StatCard label={textUI("m_orgs_m_agents")} value={ac.total}><span className="stat__sub">{textUI("m_orgd_online_offline", [ac.online, ac.offline])}</span></StatCard>
      {canFindings && <StatCard label={textUI("m_orgs_m_findings")} value={bySeverity ? severityTotal(bySeverity) : undefined} text={bySeverity ? undefined : '—'}>{bySeverity && <div className="stat__sev"><SeverityBar by={bySeverity}/></div>}</StatCard>}
      {canScans && <StatCard label={textUI("m_orgs_stat_last")} text={last ? relativeTime(last.created_at) : '—'} loading={scans.isPending && scans.fetchStatus !== 'idle'}>{last && <span className="stat__sub orgd__lastscan"><StatusBadge status={completionStatus(last)}/></span>}</StatCard>}
    </div>);
}

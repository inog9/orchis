import { Link } from 'react-router-dom';
import { t as textUI, useLocale, fv as formatUI } from '../../i18n/runtime';
import { useReadiness, type ReadinessItem } from '../../api/readiness';

const RECENT_MS = 30 * 24 * 3600 * 1000; // 30 hari

// Dashboard cakupan: aset yang belum siap dinilai penuh, dengan perbaikan untuk tiap baris.
export function CoveragePanel({ orgID }: { orgID: string }) {
    useLocale();
    const q = useReadiness(orgID, true);
    if (q.isPending || q.error || !q.data) return null;
    const rows = q.data.items.flatMap((item) => {
        const issues: string[] = [];
        const fixes: { label: string; to: string }[] = [];
        if (item.agent_status === 'none') {
            issues.push(textUI('flow_cov_no_agent'));
            fixes.push({ label: textUI('flow_hint_agent'), to: '/assets/agents' });
        } else if (item.agent_status === 'offline') {
            issues.push(textUI('flow_cov_agent_offline'));
            fixes.push({ label: textUI('flow_hint_agent'), to: '/assets/agents' });
        } else if (item.defense_reason.startsWith('Collection')) {
            issues.push(textUI('flow_cov_no_consent'));
            fixes.push({ label: textUI('flow_hint_consent'), to: `/organizations/${orgID}?tab=agents` });
        }
        const recent = item.last_scan_at && Date.now() - Date.parse(item.last_scan_at) < RECENT_MS;
        if (!recent) {
            issues.push(textUI('flow_cov_no_recent'));
        } else if (item.last_scan_status && ['incomplete', 'failed'].includes(item.last_scan_status)) {
            issues.push(textUI('flow_cov_incomplete'));
        }
        if (issues.length === 0) return [];
        return [{ item, issues, fixes }];
    });
    return (<section className="panel" aria-labelledby="cov-h">
      <h2 id="cov-h" className="panel__title">{textUI('flow_cov_title')}</h2>
      <p className="field__hint">{textUI('flow_cov_sub')}</p>
      {rows.length === 0 ? <p role="status">{textUI('flow_cov_none')}</p> : (<ul className="cov-list">
        {rows.map(({ item, issues, fixes }) => <CoverageRow key={item.id} orgID={orgID} item={item} issues={issues} fixes={fixes}/>)}
      </ul>)}
    </section>);
}

function CoverageRow({ orgID, item, issues, fixes }: { orgID: string; item: ReadinessItem; issues: string[]; fixes: { label: string; to: string }[] }) {
    return (<li className="cov-row">
      <span className="cov-row__name">{formatUI(item.name)}</span>
      {issues.map((i) => <span key={i} className="cov-row__why">{formatUI(i)}</span>)}
      <span className="cov-row__fix">
        {fixes.slice(0, 1).map((f) => <Link key={f.to} className="link-quiet" to={f.to}>{formatUI(f.label)}</Link>)}
        {!fixes.length && <Link className="link-quiet" to={`/simulations/new?org=${encodeURIComponent(orgID)}&asset=${encodeURIComponent(item.id)}`}>{textUI('flow_hint_scan')}</Link>}
      </span>
    </li>);
}

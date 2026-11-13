import { t as textUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { api } from '../api/client';
import type { Finding, FindingPage } from '../api/types';
import { describeError } from './ErrorNote';
import { SeverityBadge, when } from './RunBadges';
import './monitoring.css';

const typeLabel = (type: string): string => {
    switch (type) {
        case 'c2_suspect_connection': return textUI("m_mon_t_c2_suspect_connection");
        case 'new_listening_port': return textUI("m_mon_t_new_listening_port");
        case 'new_software_installed': return textUI("m_mon_t_new_software_installed");
        case 'new_local_account': return textUI("m_mon_t_new_local_account");
        case 'agent_tamper': return textUI("m_mon_t_agent_tamper");
        case 'agent_unresponsive': return textUI("m_mon_t_agent_unresponsive");
        default: return type;
    }
};
const text = (v: unknown): string => (Array.isArray(v) ? v.join(', ') : v == null ? '' : String(v));
// Only whitelisted, already-redacted fields are shown: where it connected, on which ports, by which process.
function facts(f: Finding): Array<[string, string]> {
    const d = f.details ?? {};
    const row = d.row as Record<string, unknown> | undefined;
    const observed = row ? ['port', 'address', 'process', 'name', 'version', 'username'].filter((k) => row[k] != null && row[k] !== '').map((k) => `${k}=${row[k]}`).join(' ') : '';
    const out: Array<[string, string]> = [];
    if (d.dst) out.push([textUI("m_mon_f_dst"), text(d.dst)]);
    if (d.ports) out.push([textUI("m_mon_f_ports"), text(d.ports)]);
    if (d.processes) out.push([textUI("m_mon_f_processes"), text(d.processes)]);
    if (d.connections) out.push([textUI("m_mon_f_connections"), text(d.connections)]);
    if (observed) out.push([textUI("m_mon_f_observed"), observed]);
    return out;
}

// Standing detections from the endpoint monitor (blue team). They are not part of one scan.
export function MonitoringPanel({ assetID, allowed }: { assetID: string; allowed: boolean }) {
    useLocale();
    const q = useQuery({ queryKey: ['findings', 'monitoring', assetID], enabled: allowed && !!assetID, queryFn: () => api.get<FindingPage>(`/api/v1/findings?asset_id=${encodeURIComponent(assetID)}&type=monitoring&limit=50`), refetchInterval: 15000, retry: false });
    const items = q.data?.items ?? [];
    if (!allowed)
        return null;
    return (<section className="panel mon" aria-labelledby="mon-h">
      <div className="mon__head"><div><h2 id="mon-h" className="panel__title">{textUI("m_mon_title")}</h2><p className="muted small">{textUI("m_mon_sub")}</p></div>
        {formatUI(allowed && items.length > 0 && <span className="status">{textUI("m_mon_count", [String(items.length)])}</span>)}</div>
      {formatUI(q.isPending ? <p role="status">{textUI("m_mon_loading")}</p> : q.isError ? <p role="alert">{formatUI(describeError(q.error).message)}</p> : items.length === 0 ? <p className="muted">{textUI("m_mon_none")}</p> : (<ul className="mon__list" aria-live="polite">
        {items.map((f) => (<li key={f.id} className="mon__row">
          <div className="mon__top"><SeverityBadge severity={f.severity}/><strong>{formatUI(f.title)}</strong></div>
          <p className="muted small mon__meta">{formatUI(typeLabel(f.type))} · {textUI("m_mon_seen", [when(f.last_seen_at), String(f.occurrence_count)])}</p>
          {formatUI(facts(f).length > 0 && <dl className="mon__facts">{facts(f).map(([k, v]) => <div key={k}><dt>{formatUI(k)}</dt><dd className="mono">{formatUI(v)}</dd></div>)}</dl>)}
        </li>))}
      </ul>))}
      {formatUI(allowed && <Link className="link-quiet" to={`/findings?asset_id=${encodeURIComponent(assetID)}`}>{textUI("m_mon_all")}</Link>)}
    </section>);
}

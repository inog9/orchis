import { t as textUI, useLocale, locale, fv as formatUI } from "../i18n/runtime";
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { api } from '../api/client';
import type { Scan } from '../api/scans';
import { describeError } from './ErrorNote';
import { StatusBadge } from './sims/StatusBadge';
import { durationMs, formatDuration } from '../lib/scanView';
import { engineSetLabel } from '../lib/engines';

// Riwayat scan aset: /scans hanya bisa disaring per organisasi, jadi halaman pertama (25 scan terbaru) disaring di sini menurut options.asset_ids.
export function scansForAsset(items: Scan[], assetID: string): Scan[] {
    return items.filter((s) => (s.options?.asset_ids ?? []).includes(assetID));
}
export function AssetScanHistory({ assetID, organizationID, allowed }: { assetID: string; organizationID: string | null | undefined; allowed: boolean }) {
    useLocale();
    const q = useQuery({ queryKey: ['scans', 'asset', assetID], enabled: allowed && !!organizationID, retry: false, queryFn: () => api.get<{ items: Scan[]; next_cursor: string | null }>(`/api/v1/scans?org=${encodeURIComponent(organizationID!)}`) });
    if (!allowed || !organizationID) return null;
    const items = scansForAsset(q.data?.items ?? [], assetID);
    const u = { h: textUI("m_sim_u_h"), m: textUI("m_sim_u_m"), s: textUI("m_sim_u_s") };
    const started = (iso: string) => { const d = new Date(iso); return Number.isNaN(d.getTime()) ? '—' : d.toLocaleString(locale(), { dateStyle: 'medium', timeStyle: 'short' }); };
    return <section className="panel" aria-labelledby="scan-hist-h"><h2 id="scan-hist-h" className="panel__title">{textUI("m_ast_scan_history")}</h2>
      {q.isPending ? <p role="status">{textUI("m_sim_loading")}</p>
        : q.isError ? <div role="alert"><p>{formatUI(describeError(q.error).message)}</p><button type="button" className="btn btn--secondary btn--auto" onClick={() => q.refetch()}>{textUI("m_sim_retry")}</button></div>
        : items.length === 0 ? <p className="muted">{textUI("m_ast_scan_none")}</p>
        : <ul className="ast-scans">{items.map((s) => <li key={s.id}>
            <Link to={`/simulations/${s.id}`} className="mono">{s.id.slice(0, 13)}</Link>
            <StatusBadge status={s.status}/>
            <span>{engineSetLabel(s.options.tools)} · {formatUI(s.options.preset)}</span>
            <span className="muted small">{started(s.created_at)} · {formatDuration(durationMs(s), u)}</span>
          </li>)}</ul>}
      <Link className="link-quiet" to={`/simulations?org=${encodeURIComponent(organizationID)}`}>{textUI("m_ast_scan_all")}</Link>
    </section>;
}

import { t as textUI, tr as richUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { api } from '../api/client';
import type { FindingPage } from '../api/types';
import { describeError } from './ErrorNote';
import { SeverityBadge, when } from './RunBadges';
export function AssetFindings({ assetID, allowed }: {
    assetID: string;
    allowed: boolean;
}) {
    useLocale();
    const q = useQuery({ queryKey: ['findings', 'asset', assetID], enabled: allowed && !!assetID, queryFn: () => api.get<FindingPage>(`/api/v1/findings?asset_id=${encodeURIComponent(assetID)}&limit=5`), refetchInterval: 5000, retry: false });
    const items = q.data?.items ?? [];
    return <section className="panel" aria-labelledby="find-h"><h2 id="find-h" className="panel__title">{textUI("m_e171c2ff25b5")}</h2>
 {formatUI(!allowed ? <p className="muted">{textUI("m_c06a30a9b145")}</p> : q.isPending ? <p role="status">{textUI("m_bc8d3b8863e0")}</p> : q.isError ? <div role="alert"><p>{richUI("m_764ccf456c1e", [formatUI(describeError(q.error).message)])}</p><button className="btn btn--secondary btn--auto" onClick={() => q.refetch()}>{textUI("m_bc7d813166ad")}</button></div> : items.length === 0 ? <p className="muted">{textUI("m_cca407010ef1")}</p> : <><p className="muted small">{textUI("m_0c9d5ffde251")}</p><ul className="mini-list" aria-label={textUI("m_5342efb84509")}>{formatUI(items.map(f => <li key={f.id}><div className="mini-list__main"><strong>{formatUI(f.title)}</strong><span className="muted small">{richUI("m_77ac00d18fc7", [formatUI(when(f.last_seen_at))])}</span></div><SeverityBadge severity={f.severity}/></li>))}</ul></>)}
 {formatUI(allowed && <Link className="link-quiet" to={`/findings?asset_id=${encodeURIComponent(assetID)}`}>{textUI("m_9c6ec0c24bde")}</Link>)}
 </section>;
}

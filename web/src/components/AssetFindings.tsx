import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { api } from '../api/client';
import type { FindingPage } from '../api/types';
import { describeError } from './ErrorNote';
import { SeverityBadge, when } from './RunBadges';

export function AssetFindings({assetID,allowed}:{assetID:string;allowed:boolean}) {
 const q=useQuery({queryKey:['findings','asset',assetID],enabled:allowed&&!!assetID,queryFn:()=>api.get<FindingPage>(`/api/v1/findings?asset_id=${encodeURIComponent(assetID)}&limit=5`),refetchInterval:5000,retry:false});
 const items=q.data?.items??[];
 return <section className="panel" aria-labelledby="find-h"><h2 id="find-h" className="panel__title">Findings</h2>
 {!allowed?<p className="muted">Your role cannot read findings. Ask an administrator to review your access.</p>:q.isPending?<p role="status">Loading this asset’s findings…</p>:q.isError?<div role="alert"><p>Couldn’t load this asset’s findings. {describeError(q.error).message}</p><button className="btn btn--secondary btn--auto" onClick={()=>q.refetch()}>Retry asset findings</button></div>:items.length===0?<p className="muted">No findings recorded for this asset yet. Failed or skipped checks do not prove that it is safe.</p>:<><p className="muted small">Latest findings recorded on this asset.</p><ul className="mini-list" aria-label="Asset findings">{items.map(f=><li key={f.id}><div className="mini-list__main"><strong>{f.title}</strong><span className="muted small">Last seen {when(f.last_seen_at)}</span></div><SeverityBadge severity={f.severity}/></li>)}</ul></>}
 {allowed&&<Link className="link-quiet" to={`/findings?asset_id=${encodeURIComponent(assetID)}`}>View all findings for this asset →</Link>}
 </section>;
}

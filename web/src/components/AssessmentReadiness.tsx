import { Link } from 'react-router-dom';
import { useHome, useMe } from '../api/hooks';
import { can } from '../api/types';
import { useReadiness } from './OrganisationSetup';
import { describeError } from './ErrorNote';
export function AssessmentReadiness({id}:{id:string}) {
 const q=useReadiness(id);const h=useHome();const {data:me}=useMe();
 const v=q.data;const blocked=v?.items.filter(a=>a.reason)||[];const offline=h.data?.injectors?.filter(i=>i.status!=='online')||[];
 return <section className="prerequisites" aria-labelledby="readiness-title"><h2 id="readiness-title" className="panel__heading">Before you assess</h2><p>Nmap and Nuclei run from scan worker containers, not from endpoint agents. The worker must reach each approved target address.</p>
 {q.isPending&&<p role="status">Checking testing permission and target addresses…</p>}{q.error&&<div role="alert"><p>Could not check readiness: {describeError(q.error).message}</p><button className="link-quiet" onClick={()=>q.refetch()}>Check again</button></div>}
 {v&&<><p><strong>{v.ready} ready to scan / {v.total} assets.</strong> This preview does not run checks. Permission and scope are checked again when scans start.</p>{v.total===0&&<p>No assets yet. Install an agent above, <Link to="/assets?new=1">add an asset</Link>, or assign unassigned assets below.</p>}{v.total>1000&&<p>Preview shows the first 1000 assets. Select at most 1000 assets for an assessment.</p>}{blocked.length>0&&<details open={v.ready===0}><summary>{blocked.length} assets need attention in this preview</summary><ul>{blocked.slice(0,20).map(a=><li key={a.id}><strong>{a.name}: {a.reason}</strong>{a.authorized?<span>Enter a reachable IP address and web URL on the asset. An address may still be refused by scope policy.</span>:<span>{can(me,'asset.authorize')?'Select this asset below and authorize testing, then set its target addresses.':'Ask an administrator to authorize testing and set its target addresses.'}</span>}<Link to={`/assets/${a.id}`}>Review asset and addresses →</Link></li>)}</ul>{blocked.length>20&&<p>Review the remaining assets in the list below.</p>}</details>}</>}
 {offline.length>0&&<div className="notice"><p><strong>Scan worker unavailable: {offline.map(i=>i.name).join(', ')}.</strong> Checks will wait in the queue; they have not succeeded while waiting.</p>{can(me,'integration.read')?<Link to="/settings/integrations">Check scan workers →</Link>:<p>Ask an administrator to reconnect the scan workers.</p>}</div>}
 {!can(me,'assessment.run')&&<p>Your role can review results. Ask an administrator or analyst to start an assessment.</p>}
 </section>;
}

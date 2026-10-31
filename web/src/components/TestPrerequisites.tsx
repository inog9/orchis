import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { api } from '../api/client';
import { useHome, useMe } from '../api/hooks';
import { can, type Asset } from '../api/types';
export function TestPrerequisites({targetID,needsAgent}:{targetID?:string|null;needsAgent:boolean}) {
 const {data:me}=useMe();const h=useHome();
 const q=useQuery({queryKey:['asset',targetID],enabled:!!targetID&&can(me,'asset.read'),queryFn:()=>api.get<Asset>(`/api/v1/assets/${targetID}`),refetchInterval:5000});
 const a=q.data;const offline=h.data?.injectors?.filter(i=>i.status!=='online')||[];
 return <div className="prerequisites"><p>Creating a test saves its settings. Press Run on the test page to start a check.</p>
 {!targetID&&<p>Choose an authorized asset. <Link to="/assets">Review assets and testing permission →</Link></p>}
 {a&&<><Link to={`/assets/${a.id}`}>Review target: {a.name} →</Link>{!a.testing_authorized&&<p>Testing permission is missing. Ask an administrator to authorize this asset before running it.</p>}{!needsAgent&&!a.addresses?.length&&<p>No target address. Edit the asset to enter approved addresses before running a network scan.</p>}{needsAgent&&a.agent_state!=='online'&&<p>The endpoint agent is offline or revoked. <Link to="/assets/agents">Check or reinstall the agent →</Link></p>}</>}
 {!needsAgent&&offline.length>0&&<p>Scan worker unavailable: {offline.map(i=>i.name).join(', ')}. A run will wait in the queue. {can(me,'integration.read')?<Link to="/settings/integrations">Check scan workers →</Link>:'Ask an administrator to reconnect the scan worker.'}</p>}
 </div>;
}

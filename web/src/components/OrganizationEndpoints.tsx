import { Link, useSearchParams } from 'react-router-dom';
import { useMe } from '../api/hooks';
import { can, type AgentRow } from '../api/types';
import { EndpointCollection } from './EndpointCollection';
import { EndpointCatalog } from './EndpointCatalog';
import { EndpointDataPanel } from './EndpointDataPanel';
import { AgentNetworks } from './AgentNetworks';

export function OrganizationEndpoints({org,agents}:{org:string;agents:AgentRow[]}){
 const {data:me}=useMe();const [params,setParams]=useSearchParams();const chosen=params.get('endpoint');const fromAsset=params.get('asset');const selected=chosen?agents.find(a=>a.id===chosen):fromAsset?agents.find(a=>a.asset_id===fromAsset):agents[0];
 const manage=can(me,'settings.manage')&&can(me,'organization.write');
 return <><div className="org-grid">{(['linux','darwin','windows'] as const).map(platform=><article className="org-card" key={platform}><h2>{platform==='darwin'?'macOS':platform==='linux'?'Linux':'Windows'}</h2><p className="muted">{platform==='linux'?'Only amd64 has a pinned osquery artifact.':'Endpoint collection not supported yet; no pinned artifact or runtime proof.'}</p>{agents.filter(a=>(a.os==='macos'?'darwin':a.os)===platform).map(a=><p key={a.id}><button className="btn btn--secondary btn--auto" aria-pressed={selected?.id===a.id} onClick={()=>{const next=new URLSearchParams(params);next.set('endpoint',a.id);next.delete('asset');setParams(next)}}>{a.instance_name||a.asset_name}</button> · {a.status}</p>)}</article>)}</div>{selected?<section className="panel" key={selected.id} aria-label="Selected endpoint"><h2>{selected.instance_name||selected.asset_name}</h2><p>{selected.os==='macos'?'darwin':selected.os} / {selected.arch} · {selected.status} · <Link to={`/assets/${selected.asset_id}`}>Asset addresses and testing permission</Link></p><EndpointCollection org={org} platform={selected.os} arch={selected.arch} canManage={manage}/><EndpointDataPanel assetID={selected.asset_id} allowed={can(me,'asset.read')} canRun={can(me,'atomic.run')} canDelete={can(me,'asset.write')&&can(me,'settings.manage')}/><details><summary>Collection catalogue and individual approvals</summary><EndpointCatalog orgId={org} platform={selected.os==='macos'?'darwin':selected.os} arch={selected.arch} canManage={manage}/></details><AgentNetworks org={org} agent={selected.id}/></section>:<p>{agents.length?'Choose an endpoint from this organisation.':'No agents/endpoints yet. Install an agent for this organisation, then authorize its asset and review collection consent.'}</p>}</>;
}

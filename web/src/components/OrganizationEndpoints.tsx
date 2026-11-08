import { collectionSupported } from '../lib/collectionSupport';
import { t as textUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { Link, useSearchParams } from 'react-router-dom';
import { useMe } from '../api/hooks';
import { can, type AgentRow } from '../api/types';
import { EndpointCollection } from './EndpointCollection';
import { EndpointDataPanel } from './EndpointDataPanel';
import { AgentControls, AgentDiagnostics } from './AgentControls';
export function OrganizationEndpoints({ org, agents }: {
    org: string;
    agents: AgentRow[];
}) {
    useLocale();
    const { data: me } = useMe();
    const [params, setParams] = useSearchParams();
    const chosen = params.get('endpoint');
    const fromAsset = params.get('asset');
    const selected = chosen ? agents.find(a => a.id === chosen) : fromAsset ? agents.find(a => a.asset_id === fromAsset) : agents[0];
    const manage = can(me, 'settings.manage') && can(me, 'organization.write');
    return <><div className="org-grid">{formatUI((['linux', 'darwin', 'windows'] as const).map(platform => <article className="org-card" key={platform}><h2>{formatUI(platform === 'darwin' ? 'macOS' : platform === 'linux' ? textUI("m_4828e60247c1") : textUI("m_d598026a9cbc"))}</h2><p className="muted">{formatUI(platform === 'linux' ? textUI("m_cf2f91907061") : platform === 'darwin' ? textUI("m_ep_macos_note") : textUI("m_8ef208d9b356"))}</p>{formatUI(agents.filter(a => (a.os === 'macos' ? 'darwin' : a.os) === platform).map(a => <p key={a.id}><button className="btn btn--secondary btn--auto" aria-pressed={selected?.id === a.id} onClick={() => { const next = new URLSearchParams(params); next.set('endpoint', a.id); next.delete('asset'); setParams(next); }}>{formatUI(a.instance_name || a.asset_name)}</button> · {formatUI(a.status)}</p>))}</article>))}</div>{formatUI(selected ? <section className="panel" key={selected.id} aria-label={textUI("m_a94db660e5d0")}><h2>{formatUI(selected.instance_name || selected.asset_name)}</h2><p>{formatUI(selected.os === 'macos' ? 'darwin' : selected.os)} / {formatUI(selected.arch)} · {formatUI(selected.status)} · <Link to={`/assets/${selected.asset_id}`}>{textUI("m_cb2b4c5d7e5f")}</Link></p>{formatUI(can(me, 'agent.manage') && can(me, 'settings.manage') ? <AgentControls agent={selected}/> : <AgentDiagnostics agent={selected}/>)}{formatUI(collectionSupported(selected.os, selected.arch) ? <><EndpointCollection org={org} platform={selected.os} arch={selected.arch} canManage={manage}/><EndpointDataPanel assetID={selected.asset_id} allowed={can(me, 'asset.read')} canRun={can(me, 'atomic.run')} canDelete={can(me, 'asset.write') && can(me, 'settings.manage')}/></> : <p className="notice" role="note">{textUI("m_ep_unsupported")}</p>)}</section> : <p>{formatUI(agents.length ? textUI("m_9ae4278f394a") : textUI("m_fb30a3ebf4b1"))}</p>)}</>;
}

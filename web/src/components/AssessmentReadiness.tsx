import { t as textUI, tr as richUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { Link } from 'react-router-dom';
import { useHome, useMe } from '../api/hooks';
import { can } from '../api/types';
import { useReadiness } from './OrganisationSetup';
import { describeError } from './ErrorNote';
import { localizeReason } from '../lib/readinessReason';
import { engineNames } from '../lib/engines';
export function AssessmentReadiness({ id }: {
    id: string;
}) {
    useLocale();
    const q = useReadiness(id);
    const h = useHome();
    const { data: me } = useMe();
    const v = q.data;
    const blocked = v?.items.filter(a => a.reason) || [];
    const offline = h.data?.injectors?.filter(i => i.status !== 'online') || [];
    return <section className="prerequisites" aria-label={textUI("m_0948f8f37f92", [v?.ready ?? 0, v?.total ?? 0])}>
 {formatUI(q.isPending && <p role="status">{textUI("m_6470b2b671b5")}</p>)}{formatUI(q.error && <div role="alert"><p>{richUI("m_86020aac288e", [formatUI(describeError(q.error).message)])}</p><button className="link-quiet" onClick={() => q.refetch()}>{textUI("m_fb7099ad8e81")}</button></div>)}
 {formatUI(v && <><p className="prerequisites__line"><strong>{richUI("m_0948f8f37f92", [formatUI(v.ready), formatUI(v.total)])}</strong> <Link to={`/simulations/new?org=${id}`}>{textUI("m_sim_new")}</Link></p>{formatUI(v.total === 0 && <p>{richUI("m_2f5530eb6440", [<Link to={`/organizations/${id}?tab=assets&new=1`}>{textUI("m_cd72469a0b15")}</Link>])}</p>)}{formatUI(v.total > 1000 && <p>{textUI("m_8b8eded21875")}</p>)}{formatUI(blocked.length > 0 && <details><summary>{richUI("m_a48db6ac3189", [formatUI(blocked.length)])}</summary><ul>{formatUI(blocked.slice(0, 20).map(a => <li key={a.id}><strong>{formatUI(a.name)}: {formatUI(localizeReason(a.reason))}</strong>{formatUI(a.authorized ? <span>{textUI("m_abb1f09fa621")}</span> : <span>{formatUI(can(me, 'asset.authorize') ? textUI("m_8cf4bd3d02e5") : textUI("m_8c4590d01525"))}</span>)}<Link to={`/assets/${a.id}`}>{textUI("m_068a03233a3b")}</Link></li>))}</ul>{formatUI(blocked.length > 20 && <p>{textUI("m_5c15abb8be89")}</p>)}</details>)}</>)}
 {formatUI(offline.length > 0 && <div className="notice"><p>{richUI("m_5f695bf84d4a", [<strong>{richUI("m_bb60e1dfc8f7", [formatUI(engineNames(offline.map(i => i.key)).join(', '))])}</strong>])}</p>{formatUI(can(me, 'integration.read') ? <Link to="/settings/integrations">{textUI("m_a95106196bce")}</Link> : <p>{textUI("m_545a0ca386d8")}</p>)}</div>)}
 {formatUI(!can(me, 'assessment.run') && <p>{textUI("m_9cc6ceaeb689")}</p>)}
 </section>;
}

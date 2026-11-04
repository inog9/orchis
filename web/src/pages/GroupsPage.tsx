import { t as textUI, tr as richUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { ChevronRight, Plus } from 'lucide-react';
import { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { useGroups, useMe } from '../api/hooks';
import { can } from '../api/types';
import { AssetTabs } from '../components/AssetTabs';
import { describeError } from '../components/ErrorNote';
import { describeRule } from '../lib/rule';
import { GroupDialog } from './GroupDialog';
export function GroupsPage() {
    useLocale();
    const { data: me } = useMe();
    const navigate = useNavigate();
    const q = useGroups();
    const [creating, setCreating] = useState(false);
    const canWrite = can(me, 'asset.write');
    const items = q.data ?? [];
    return (<section className="page" aria-labelledby="groups-title">
      <div className="page__head">
        <div>
          <nav className="crumbs" aria-label={textUI("m_2bd873d6c734")}><span>{textUI("m_bd12731d7bc9")}</span><ChevronRight size={12} aria-hidden="true"/><span className="crumbs__current">{textUI("m_39dd12de3b02")}</span></nav>
          <div className="page__title-row">
            <h1 id="groups-title" className="page__title">{textUI("m_39dd12de3b02")}</h1>
            {formatUI(q.data && <span className="badge">{formatUI(items.length)}</span>)}
          </div>
          <p className="page__sub">{textUI("m_5152fb13d2e2")}</p>
        </div>
        {formatUI(canWrite && <button type="button" className="btn btn--primary btn--auto" onClick={() => setCreating(true)}>{richUI("m_9735a6ab0c00", [<Plus size={16} aria-hidden="true"/>])}</button>)}
      </div>
      <AssetTabs />
      {formatUI(q.isPending ? (<ul className="cards" aria-label={textUI("m_e1bc2ca02237")}>{formatUI([0, 1, 2].map((i) => <li key={i} className="card"><span className="skeleton"/></li>))}</ul>) : q.isError ? (<div className="state state--error" role="alert">
          <h2>{textUI("m_a2cb3eaf3a2b")}</h2><p>{richUI("m_19636dc199da", [formatUI(describeError(q.error).message)])}</p>
          <button type="button" className="btn btn--secondary btn--auto" onClick={() => q.refetch()}>{textUI("m_d8b8392e2c54")}</button>
        </div>) : (<ul className="cards">
          {formatUI(items.map((g) => (<li key={g.id} className="card">
              <div className="card__top">
                <Link className="card__name" to={`/assets/groups/${g.id}`}>{formatUI(g.name)}</Link>
                {formatUI(g.is_system && <span className="status">{textUI("m_1f43948106d1")}</span>)}
              </div>
              <p className="card__rule">{formatUI(describeRule(g.rule))}</p>
              <p className="card__count"><strong>{formatUI(g.member_count)}</strong> {formatUI(g.member_count === 1 ? 'asset' : 'assets')}</p>
            </li>)))}
        </ul>))}
      {formatUI(creating && <GroupDialog onClose={() => setCreating(false)} onSaved={(g) => { setCreating(false); navigate(`/assets/groups/${g.id}`); }}/>)}
    </section>);
}

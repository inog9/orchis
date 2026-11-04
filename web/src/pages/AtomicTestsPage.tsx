import { t as textUI, tr as richUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { Plus } from 'lucide-react';
import { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { useAtomicTests, useContracts, useMe } from '../api/hooks';
import { can } from '../api/types';
import { describeError } from '../components/ErrorNote';
import { RunBadge, when } from '../components/RunBadges';
import { AtomicTestDialog } from './AtomicTestDialog';
export function AtomicTestsPage() {
    useLocale();
    const { data: me } = useMe();
    const navigate = useNavigate();
    const q = useAtomicTests();
    const contracts = useContracts();
    const [creating, setCreating] = useState(false);
    // Aksi agent bergantung pada ada-tidaknya agent di aset sasaran, bukan pada satu injector global: tidak ikut peringatan ini.
    const offline = (contracts.data ?? []).filter((c) => !c.needs_agent && c.integration_status !== 'online');
    const items = q.data ?? [];
    return (<section className="page" aria-labelledby="atomic-page-title">
      <div className="page__head">
        <div>
          <div className="page__title-row">
            <h1 id="atomic-page-title" className="page__title">{textUI("m_ad1451c7094d")}</h1>
            {formatUI(q.data && <span className="badge">{formatUI(items.length)}</span>)}
          </div>
          <p className="page__sub">{textUI("m_eb24a6b6694e")}</p>
        </div>
        {formatUI(can(me, 'atomic.write') && (<button type="button" className="btn btn--primary btn--auto" onClick={() => setCreating(true)}>{richUI("m_dc6ae7b7b982", [<Plus size={16} aria-hidden="true"/>])}</button>))}
      </div>
      {formatUI(offline.length > 0 && (<div className="notice" role="status">
          <strong>{richUI("m_4845a5715da9", [formatUI([...new Set(offline.map((c) => c.integration))].join(' and ')), formatUI(new Set(offline.map((c) => c.integration)).size === 1 ? textUI("m_8a27f2fcccaa") : textUI("m_ba5bff2b4cdf"))])}</strong>
          <span>{richUI("m_5de9b27a20d5", [formatUI(offline.length === 1 ? 'it' : 'them')])}</span>
        </div>))}
      {formatUI(q.isPending ? (<ul className="cards" aria-label={textUI("m_5d49a9b62709")}>{formatUI([0, 1, 2].map((i) => <li key={i} className="card"><span className="skeleton"/></li>))}</ul>) : q.isError ? (<div className="state state--error" role="alert">
          <h2>{textUI("m_6da3b0ad42e7")}</h2><p>{richUI("m_19636dc199da", [formatUI(describeError(q.error).message)])}</p>
          <button type="button" className="btn btn--secondary btn--auto" onClick={() => q.refetch()}>{textUI("m_d8b8392e2c54")}</button>
        </div>) : items.length === 0 ? (<div className="state">
          <h2>{textUI("m_87c3d4944952")}</h2>
          <p>{textUI("m_e74e994dcb0a")}</p>
          {formatUI(can(me, 'atomic.write') && <button type="button" className="btn btn--primary btn--auto" onClick={() => setCreating(true)}>{textUI("m_9597e83f181c")}</button>)}
        </div>) : (<ul className="cards">
          {formatUI(items.map((t) => (<li key={t.id} className="card">
              <div className="card__top">
                <Link className="card__name" to={`/atomic/${t.id}`}>{formatUI(t.name)}</Link>
                <RunBadge run={t.last_execution}/>
              </div>
              <p className="card__rule">{formatUI(t.contract_name)} · {formatUI(t.target_asset_name ?? textUI("m_4b33d955f9a8"))}</p>
              <p className="card__count">
                {formatUI(t.last_execution ? <><strong>{formatUI(t.last_execution.finding_count)}</strong> {formatUI(t.last_execution.finding_count === 1 ? 'finding' : 'findings')} · {formatUI(when(t.last_execution.created_at))}</> : <span className="muted">{textUI("m_b9906c384bf3")}</span>)}
              </p>
            </li>)))}
        </ul>))}
      {formatUI(creating && <AtomicTestDialog onClose={() => setCreating(false)} onCreated={(t) => { setCreating(false); navigate(`/atomic/${t.id}`); }}/>)}
    </section>);
}

import { t as textUI, tr as richUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { Plus } from 'lucide-react';
import { useState } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import { useMe, useScenarios } from '../api/hooks';
import { can } from '../api/types';
import { describeError } from '../components/ErrorNote';
import { NewScenarioDialog } from './NewChainDialog';
export function ChainsPage() {
    useLocale();
    const { data: me } = useMe();
    const navigate = useNavigate();
    const q = useScenarios();
    const [params, setParams] = useSearchParams();
    const [creating, setCreating] = useState(params.get('new') === '1' && can(me, 'scenario.write'));
    const items = q.data ?? [];
    return (<section className="page" aria-labelledby="chains-title">
      <div className="page__head">
        <div>
          <div className="page__title-row"><h1 id="chains-title" className="page__title">{textUI("m_ccd8711a2343")}</h1>{formatUI(q.data && <span className="badge">{formatUI(items.length)}</span>)}</div>
          <p className="page__sub">{textUI("m_a1f0f2240ab3")}</p>
        </div>
        {formatUI(can(me, 'scenario.write') && <button type="button" className="btn btn--primary btn--auto" onClick={() => setCreating(true)}>{richUI("m_df0e12051c66", [<Plus size={16} aria-hidden="true"/>])}</button>)}
      </div>
      {formatUI(q.isPending ? <span className="skeleton"/> : q.isError ? (<div className="state state--error" role="alert"><h2>{textUI("m_1ddc3fcb8be5")}</h2><p>{formatUI(describeError(q.error).message)}</p>
          <button type="button" className="btn btn--secondary btn--auto" onClick={() => q.refetch()}>{textUI("m_d8b8392e2c54")}</button></div>) : items.length === 0 ? (<div className="state"><h2>{textUI("m_eb7630cf8058")}</h2><p>{textUI("m_6d89119eda1f")}</p></div>) : (<div className="list" role="table" aria-label={textUI("m_ccd8711a2343")} tabIndex={0}>
          <div className="list__head chain-row" role="row"><span role="columnheader">{textUI("m_945c72078d9c")}</span><span role="columnheader">{textUI("m_1ba13aa632ae")}</span><span role="columnheader">{textUI("m_61bf096785ee")}</span><span role="columnheader">{textUI("m_8c2e4a035f5f")}</span></div>
          {formatUI(items.map((s) => (<div key={s.id} className="asset-row chain-row" role="row">
              <span role="cell"><Link className="asset-row__name" to={`/chains/${s.id}`}>{formatUI(s.name)}</Link></span>
              <span role="cell">{formatUI(s.step_count)}</span><span role="cell">{formatUI(s.edge_count)}</span>
              <span role="cell"><span className={`status ${s.status === 'ready' ? 'status--ok' : ''}`}>{formatUI(s.status === 'ready' ? textUI("m_5fa7aac5375c") : textUI("m_ebf12ef47cf5"))}</span></span>
            </div>)))}
        </div>))}
      {formatUI(creating && <NewScenarioDialog title={textUI("m_5c23ea1f7c64")} onClose={() => { setCreating(false); params.delete('new'); setParams(params, { replace: true }); }} onCreated={(id) => navigate(`/chains/${id}`)}/>)}
    </section>);
}

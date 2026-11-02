import { Plus } from 'lucide-react';
import { useState } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import { useMe, useScenarios } from '../api/hooks';
import { can } from '../api/types';
import { describeError } from '../components/ErrorNote';
import { NewScenarioDialog } from './NewChainDialog';

export function ChainsPage() {
  const { data: me } = useMe();
  const navigate = useNavigate();
  const q = useScenarios();
  const [params,setParams]=useSearchParams();
  const [creating, setCreating] = useState(params.get('new')==='1'&&can(me,'scenario.write'));
  const items = q.data ?? [];
  return (
    <section className="page" aria-labelledby="chains-title">
      <div className="page__head">
        <div>
          <div className="page__title-row"><h1 id="chains-title" className="page__title">Scenario chains</h1>{q.data && <span className="badge">{items.length}</span>}</div>
          <p className="page__sub">Design the actions and dependencies of each chain. A step starts once all of its parents finish, or when a parent finds something.</p>
        </div>
        {can(me, 'scenario.write') && <button type="button" className="btn btn--primary btn--auto" onClick={() => setCreating(true)}><Plus size={16} aria-hidden="true" /> New chain</button>}
      </div>
      {q.isPending ? <span className="skeleton" /> : q.isError ? (
        <div className="state state--error" role="alert"><h2>Couldn’t load chains</h2><p>{describeError(q.error).message}</p>
          <button type="button" className="btn btn--secondary btn--auto" onClick={() => q.refetch()}>Try again</button></div>
      ) : items.length === 0 ? (
        <div className="state"><h2>No scenario chains yet</h2><p>A chain is a draft until you add actions; creating one does not run anything.</p></div>
      ) : (
        <div className="list" role="table" aria-label="Scenario chains" tabIndex={0}>
          <div className="list__head chain-row" role="row"><span role="columnheader">CHAIN</span><span role="columnheader">ACTIONS</span><span role="columnheader">DEPENDENCIES</span><span role="columnheader">STATUS</span></div>
          {items.map((s) => (
            <div key={s.id} className="asset-row chain-row" role="row">
              <span role="cell"><Link className="asset-row__name" to={`/chains/${s.id}`}>{s.name}</Link></span>
              <span role="cell">{s.step_count}</span><span role="cell">{s.edge_count}</span>
              <span role="cell"><span className={`status ${s.status === 'ready' ? 'status--ok' : ''}`}>{s.status === 'ready' ? 'Ready' : 'Draft'}</span></span>
            </div>
          ))}
        </div>
      )}
      {creating && <NewScenarioDialog title="New scenario chain" onClose={() => {setCreating(false);params.delete('new');setParams(params,{replace:true});}} onCreated={(id) => navigate(`/chains/${id}`)} />}
    </section>
  );
}

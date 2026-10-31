import { Link } from 'react-router-dom';
import { useState } from 'react';
import { useSimulations } from '../api/hooks';
import { describeError } from '../components/ErrorNote';
import { SimBadge, elapsed } from '../components/StatusPills';
import { when } from '../components/RunBadges';

const FILTERS = [['', 'All'], ['running', 'Running'], ['paused', 'Paused'], ['finished', 'Finished'], ['canceled', 'Canceled'], ['failed', 'Failed']] as const;

export function SimulationsPage() {
  const [status, setStatus] = useState('');
  const q = useSimulations(status);
  const items = q.data ?? [];
  return (
    <section className="page" aria-labelledby="sims-title">
      <div className="page__head">
        <div>
          <div className="page__title-row"><h1 id="sims-title" className="page__title">Simulations</h1>{q.data && <span className="badge">{items.length}</span>}</div>
          <p className="page__sub">Every launch of a scenario. Open one to follow its steps, findings and attack path.</p>
        </div>
      </div>
      <div className="chips" role="group" aria-label="Filter by status">
        {FILTERS.map(([v, l]) => <button key={v} type="button" className={`chip ${status === v ? 'is-on' : ''}`} aria-pressed={status === v} onClick={() => setStatus(v)}>{l}</button>)}
      </div>
      {q.isPending ? <span className="skeleton" /> : q.isError ? (
        <div className="state state--error" role="alert"><h2>Couldn’t load simulations</h2><p>{describeError(q.error).message} Your data is safe.</p>
          <button type="button" className="btn btn--secondary btn--auto" onClick={() => q.refetch()}>Try again</button></div>
      ) : items.length === 0 ? (
        <div className="state"><h2>{status ? 'No simulations with this status' : 'No simulations yet'}</h2>
          <p>{status ? 'Try another filter.' : 'Launch a scenario to start one. Nothing runs until you do.'}</p>
          {!status && <Link className="btn btn--secondary btn--auto" to="/scenarios">Go to scenarios</Link>}</div>
      ) : (
        <div className="list" role="table" aria-label="Simulations" tabIndex={0}>
          <div className="list__head sim-row" role="row"><span role="columnheader">SIMULATION</span><span role="columnheader">STATUS</span><span role="columnheader">PROGRESS</span><span role="columnheader">FINDINGS</span><span role="columnheader">STARTED</span></div>
          {items.map((s) => (
            <div key={s.id} className="asset-row sim-row" role="row">
              <span role="cell" className="asset-row__text"><Link className="asset-row__name" to={`/simulations/${s.id}`}>{s.name}</Link><span className="muted small">{s.code} · {s.scenario_name ?? 'scenario removed'}</span></span>
              <span role="cell"><SimBadge status={s.status} /></span>
              <span role="cell">{s.steps_done} of {s.step_count} steps</span>
              <span role="cell">{s.finding_count}</span>
              <span role="cell" className="muted small">{when(s.started_at)}{s.started_at ? ` · ${elapsed(s.started_at, s.ended_at)}` : ''}</span>
            </div>
          ))}
        </div>
      )}
    </section>
  );
}

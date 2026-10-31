import { Plus } from 'lucide-react';
import { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { useAtomicTests, useContracts, useMe } from '../api/hooks';
import { can } from '../api/types';
import { describeError } from '../components/ErrorNote';
import { RunBadge, when } from '../components/RunBadges';
import { AtomicTestDialog } from './AtomicTestDialog';

export function AtomicTestsPage() {
  const { data: me } = useMe();
  const navigate = useNavigate();
  const q = useAtomicTests();
  const contracts = useContracts();
  const [creating, setCreating] = useState(false);
  // Aksi agent bergantung pada ada-tidaknya agent di aset sasaran, bukan pada satu injector global: tidak ikut peringatan ini.
  const offline = (contracts.data ?? []).filter((c) => !c.needs_agent && c.integration_status !== 'online');
  const items = q.data ?? [];

  return (
    <section className="page" aria-labelledby="atomic-page-title">
      <div className="page__head">
        <div>
          <div className="page__title-row">
            <h1 id="atomic-page-title" className="page__title">Single tests</h1>
            {q.data && <span className="badge">{items.length}</span>}
          </div>
          <p className="page__sub">Run a single action against one authorized asset and see what it finds.</p>
        </div>
        {can(me, 'atomic.write') && (
          <button type="button" className="btn btn--primary btn--auto" onClick={() => setCreating(true)}><Plus size={16} aria-hidden="true" /> New single test</button>
        )}
      </div>
      {offline.length > 0 && (
        <div className="notice" role="status">
          <strong>{[...new Set(offline.map((c) => c.integration))].join(' and ')} {new Set(offline.map((c) => c.integration)).size === 1 ? 'injector is' : 'injectors are'} not connected.</strong>
          <span> Tests for {offline.length === 1 ? 'it' : 'them'} will wait in the queue until it starts.</span>
        </div>
      )}
      {q.isPending ? (
        <ul className="cards" aria-label="Loading tests">{[0, 1, 2].map((i) => <li key={i} className="card"><span className="skeleton" /></li>)}</ul>
      ) : q.isError ? (
        <div className="state state--error" role="alert">
          <h2>Couldn’t load single tests</h2><p>{describeError(q.error).message} Your data is safe.</p>
          <button type="button" className="btn btn--secondary btn--auto" onClick={() => q.refetch()}>Try again</button>
        </div>
      ) : items.length === 0 ? (
        <div className="state">
          <h2>No single tests yet</h2>
          <p>An single test runs one scan, such as an Nmap port scan, against an asset you have authorized for testing.</p>
          {can(me, 'atomic.write') && <button type="button" className="btn btn--primary btn--auto" onClick={() => setCreating(true)}>Create the first test</button>}
        </div>
      ) : (
        <ul className="cards">
          {items.map((t) => (
            <li key={t.id} className="card">
              <div className="card__top">
                <Link className="card__name" to={`/atomic/${t.id}`}>{t.name}</Link>
                <RunBadge run={t.last_execution} />
              </div>
              <p className="card__rule">{t.contract_name} · {t.target_asset_name ?? 'No target'}</p>
              <p className="card__count">
                {t.last_execution ? <><strong>{t.last_execution.finding_count}</strong> {t.last_execution.finding_count === 1 ? 'finding' : 'findings'} · {when(t.last_execution.created_at)}</> : <span className="muted">Not run yet</span>}
              </p>
            </li>
          ))}
        </ul>
      )}
      {creating && <AtomicTestDialog onClose={() => setCreating(false)} onCreated={(t) => { setCreating(false); navigate(`/atomic/${t.id}`); }} />}
    </section>
  );
}



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
  const { data: me } = useMe();
  const navigate = useNavigate();
  const q = useGroups();
  const [creating, setCreating] = useState(false);
  const canWrite = can(me, 'asset.write');
  const items = q.data ?? [];

  return (
    <section className="page" aria-labelledby="groups-title">
      <div className="page__head">
        <div>
          <nav className="crumbs" aria-label="Breadcrumb"><span>Assets</span><ChevronRight size={12} aria-hidden="true" /><span className="crumbs__current">Asset groups</span></nav>
          <div className="page__title-row">
            <h1 id="groups-title" className="page__title">Asset groups</h1>
            {q.data && <span className="badge">{items.length}</span>}
          </div>
          <p className="page__sub">Dynamic groups. Assets join automatically when they match the rules.</p>
        </div>
        {canWrite && <button type="button" className="btn btn--primary btn--auto" onClick={() => setCreating(true)}><Plus size={16} aria-hidden="true" /> New group</button>}
      </div>
      <AssetTabs />
      {q.isPending ? (
        <ul className="cards" aria-label="Loading groups">{[0, 1, 2].map((i) => <li key={i} className="card"><span className="skeleton" /></li>)}</ul>
      ) : q.isError ? (
        <div className="state state--error" role="alert">
          <h2>Couldn’t load asset groups</h2><p>{describeError(q.error).message} Your data is safe.</p>
          <button type="button" className="btn btn--secondary btn--auto" onClick={() => q.refetch()}>Try again</button>
        </div>
      ) : (
        <ul className="cards">
          {items.map((g) => (
            <li key={g.id} className="card">
              <div className="card__top">
                <Link className="card__name" to={`/assets/groups/${g.id}`}>{g.name}</Link>
                {g.is_system && <span className="status">Built-in</span>}
              </div>
              <p className="card__rule">{describeRule(g.rule)}</p>
              <p className="card__count"><strong>{g.member_count}</strong> {g.member_count === 1 ? 'asset' : 'assets'}</p>
            </li>
          ))}
        </ul>
      )}
      {creating && <GroupDialog onClose={() => setCreating(false)} onSaved={(g) => { setCreating(false); navigate(`/assets/groups/${g.id}`); }} />}
    </section>
  );
}

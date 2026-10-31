import { ChevronRight, Pencil, Trash2 } from 'lucide-react';
import { useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { ApiError } from '../api/client';
import { useDeleteGroup, useGroup, useMe } from '../api/hooks';
import { can } from '../api/types';
import { describeError } from '../components/ErrorNote';
import { describeFilter } from '../lib/rule';
import { ConfirmDialog } from './ConfirmDialog';
import { GroupDialog } from './GroupDialog';
import { categoryOf, PLATFORM_LABEL } from './categories';

export function GroupDetailPage() {
  const { id = '' } = useParams();
  const navigate = useNavigate();
  const { data: me } = useMe();
  const q = useGroup(id);
  const del = useDeleteGroup(id);
  const [editing, setEditing] = useState(false);
  const [confirming, setConfirming] = useState(false);

  if (q.isPending) return <div className="page" aria-busy="true"><span className="skeleton" style={{ height: 120 }} /></div>;
  if (q.isError) {
    const notFound = q.error instanceof ApiError && q.error.status === 404;
    return (
      <div className="state state--error" role="alert">
        <h2>{notFound ? 'Group not found' : 'Couldn’t load this group'}</h2>
        <p>{notFound ? 'It may have been deleted.' : `${describeError(q.error).message} Your data is safe.`}</p>
        <Link className="btn btn--secondary btn--auto" to="/assets/groups">Back to groups</Link>
      </div>
    );
  }
  const g = q.data;

  return (
    <section className="page" aria-labelledby="group-name">
      <div className="page__head">
        <div>
          <nav className="crumbs" aria-label="Breadcrumb">
            <Link to="/assets/groups">Asset groups</Link><ChevronRight size={12} aria-hidden="true" /><span className="crumbs__current">{g.name}</span>
          </nav>
          <div className="page__title-row">
            <h1 id="group-name" className="page__title">{g.name}</h1>
            <span className="badge">{g.member_count} of {g.total_assets}</span>
            {g.is_system && <span className="status">Built-in</span>}
          </div>
          {g.description && <p className="page__sub">{g.description}</p>}
        </div>
        {!g.is_system && can(me, 'asset.write') && (
          <button type="button" className="btn btn--secondary btn--auto" onClick={() => setEditing(true)}><Pencil size={15} aria-hidden="true" /> Edit</button>
        )}
      </div>

      <section className="panel" aria-labelledby="rules-h">
        <h2 id="rules-h" className="panel__title">Rules</h2>
        {g.rule.filters.length === 0 ? <p className="muted">Every asset matches.</p> : (
          <>
            <p className="muted small">Matches {g.rule.mode === 'or' ? 'any' : 'all'} of:</p>
            <ul className="plain-list">{g.rule.filters.map((f, i) => <li key={i}>{describeFilter(f)}</li>)}</ul>
          </>
        )}
      </section>

      <section className="list" aria-labelledby="members-h" tabIndex={0}>
        <h2 id="members-h" className="panel__title list__title">Members</h2>
        {g.members.length === 0 ? (
          <div className="state"><h2>No assets match yet</h2><p>Assets appear here automatically as they match the rules above.</p></div>
        ) : (
          <ul className="list__body">
            {g.members.map((m) => {
              const Icon = categoryOf(m.category).icon;
              return (
                <li key={m.id} className="member">
                  <span className="asset-row__icon"><Icon size={16} /></span>
                  <Link className="member__name mono" to={`/assets/${m.id}`}>{m.name}</Link>
                  <span className="muted small">{m.platform ? (PLATFORM_LABEL[m.platform] ?? m.platform) : '—'}{m.arch ? ` · ${m.arch}` : ''}</span>
                  <span className={`crit crit--${m.criticality}`}>{m.criticality[0]!.toUpperCase() + m.criticality.slice(1)}</span>
                </li>
              );
            })}
          </ul>
        )}
        {g.members_capped && <p className="muted small list__note">Showing the first 50 of {g.member_count} members.</p>}
      </section>

      {!g.is_system && can(me, 'asset.delete') && (
        <section className="danger" aria-labelledby="gdanger-h">
          <div><h2 id="gdanger-h" className="panel__title">Danger zone</h2><p className="muted">Deleting a group does not delete its assets.</p></div>
          <button type="button" className="btn btn--danger btn--auto" onClick={() => setConfirming(true)}><Trash2 size={15} aria-hidden="true" /> Delete group</button>
        </section>
      )}
      {editing && <GroupDialog group={g} onClose={() => setEditing(false)} onSaved={() => setEditing(false)} onStale={() => { setEditing(false); q.refetch(); }} />}
      {confirming && (
        <ConfirmDialog title={`Delete ${g.name}?`} body="The group will be removed. Its assets stay untouched." confirmLabel="Delete group" busy={del.isPending}
          error={del.isError ? describeError(del.error).message : undefined} onClose={() => setConfirming(false)}
          onConfirm={() => del.mutate(undefined, { onSuccess: () => navigate('/assets/groups', { replace: true }) })} />
      )}
    </section>
  );
}

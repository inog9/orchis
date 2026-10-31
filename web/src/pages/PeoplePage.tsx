import { Plus, Search, X } from 'lucide-react';
import { useEffect, useState, type FormEvent } from 'react';
import { NavLink, useParams, useSearchParams } from 'react-router-dom';
import { ApiError } from '../api/client';
import { useDeletePerson, useDeleteTeam, useMe, usePeople, useSavePerson, useSaveTeam, useSetMembers, useTeam, useTeams } from '../api/hooks';
import { can, type PersonRow, type TeamRow } from '../api/types';
import { describeError } from '../components/ErrorNote';
import { Field } from '../components/Field';
import { Modal } from '../components/Modal';
import { ConfirmDialog } from './ConfirmDialog';

function useDebounced<T>(value: T, ms: number) {
  const [v, setV] = useState(value);
  useEffect(() => { const t = setTimeout(() => setV(value), ms); return () => clearTimeout(t); }, [value, ms]);
  return v;
}

function PersonDialog({ person, onClose }: { person: PersonRow | null; onClose: () => void }) {
  const save = useSavePerson();
  const [first, setFirst] = useState(person?.first_name ?? '');
  const [last, setLast] = useState(person?.last_name ?? '');
  const [email, setEmail] = useState(person?.email ?? '');
  const [org, setOrg] = useState(person?.organization ?? '');
  const err = save.error;
  const fieldErr = (n: string) => (err instanceof ApiError ? err.fields.find((f) => f.name === n)?.message : undefined);
  const touch = () => save.isError && save.reset();
  const submit = (e: FormEvent) => {
    e.preventDefault();
    save.mutate({ id: person?.id, version: person?.version, first_name: first, last_name: last, email: email.trim() || null, organization: org.trim() || null }, { onSuccess: onClose });
  };
  return (
    <Modal labelledBy="person-title" onClose={onClose} width={520}>
      <>
        <header className="dialog__head"><h2 id="person-title">{person ? 'Edit person' : 'Add person'}</h2><button type="button" className="icon-btn" aria-label="Close" onClick={onClose}><X size={18} /></button></header>
        <form className="dialog__form" onSubmit={submit} noValidate>
          {err && !(err instanceof ApiError && err.status === 422) && <div className="alert" role="alert"><strong>We couldn’t save</strong><span>{describeError(err).message}</span></div>}
          <div className="row row--2">
            <Field label="First name" value={first} onChange={(e) => { touch(); setFirst(e.target.value); }} error={fieldErr('first_name')} autoFocus />
            <Field label="Last name" value={last} onChange={(e) => { touch(); setLast(e.target.value); }} error={fieldErr('last_name')} />
          </div>
          <Field label="Email (optional)" value={email} onChange={(e) => { touch(); setEmail(e.target.value); }} error={fieldErr('email')} />
          <Field label="Organization (optional)" value={org} onChange={(e) => { touch(); setOrg(e.target.value); }} error={fieldErr('organization')} />
          <footer className="dialog__foot"><button type="button" className="btn btn--secondary btn--auto" onClick={onClose}>Cancel</button>
            <button type="submit" className="btn btn--primary btn--auto" disabled={save.isPending}>{save.isPending ? 'Saving…' : 'Save'}</button></footer>
        </form>
      </>
    </Modal>
  );
}

function PeopleTab() {
  const { data: me } = useMe();
  const [text, setText] = useState('');
  const q = usePeople(useDebounced(text.trim(), 250));
  const del = useDeletePerson();
  const [params, setParams] = useSearchParams();
  const write = can(me, 'people.write');
  const [editing, setEditingState] = useState<PersonRow | 'new' | null>(params.get('new') === '1' && write ? 'new' : null);
  const setEditing = (v: PersonRow | 'new' | null) => { setEditingState(v); if (v === null && params.has('new')) setParams({}, { replace: true }); };
  const [deleting, setDeleting] = useState<PersonRow | null>(null);
  const items = q.data ?? [];
  return (
    <div className="stack">
      <div className="toolbar">
        <label className="search-input"><Search size={15} aria-hidden="true" /><input type="search" aria-label="Search people" placeholder="Search people" value={text} onChange={(e) => setText(e.target.value)} /></label>
        {write && <button type="button" className="btn btn--primary btn--auto" onClick={() => setEditing('new')}><Plus size={16} aria-hidden="true" /> Add person</button>}
      </div>
      {q.isPending ? <span className="skeleton" /> : q.isError ? (
        <div className="state state--error" role="alert"><h2>Couldn’t load people</h2><p>{describeError(q.error).message}</p><button type="button" className="btn btn--secondary btn--auto" onClick={() => q.refetch()}>Try again</button></div>
      ) : items.length === 0 ? (
        <div className="state"><h2>{text ? 'No one matches' : 'No people yet'}</h2><p>{text ? 'Try a different search.' : 'People are the participants in an exercise. They don’t sign in to the platform.'}</p>
          {!text && write && <button type="button" className="btn btn--primary btn--auto" onClick={() => setEditing('new')}>Add the first person</button>}</div>
      ) : (
        <div className="list" role="table" aria-label="People" tabIndex={0}>
          <div className="list__head user-row" role="row"><span role="columnheader">PERSON</span><span role="columnheader">ORGANIZATION</span><span role="columnheader">TEAMS</span><span role="columnheader"><span className="sr-only">Actions</span></span></div>
          {items.map((p) => (
            <div key={p.id} className="asset-row people-row" role="row">
              <span role="cell" className="asset-row__text"><strong>{p.first_name} {p.last_name}</strong><span className="muted small">{p.email ?? 'No email'}</span></span>
              <span role="cell">{p.organization ?? '—'}</span>
              <span role="cell">{p.teams.length === 0 ? <span className="muted">—</span> : p.teams.map((t) => <span key={t.id} className="tag-chip">{t.name}</span>)}</span>
              <span role="cell" className="row-actions">{write && <>
                <button type="button" className="btn btn--secondary btn--sm" aria-label={`Edit ${p.first_name} ${p.last_name}`} onClick={() => setEditing(p)}>Edit</button>
                <button type="button" className="btn btn--danger btn--sm" aria-label={`Delete ${p.first_name} ${p.last_name}`} onClick={() => setDeleting(p)}>Delete</button></>}</span>
            </div>
          ))}
        </div>
      )}
      {editing && <PersonDialog person={editing === 'new' ? null : editing} onClose={() => setEditing(null)} />}
      {deleting && <ConfirmDialog title={`Delete ${deleting.first_name} ${deleting.last_name}?`} body="They are removed from every team." confirmLabel="Delete person" busy={del.isPending}
        error={del.isError ? describeError(del.error).message : undefined} onConfirm={() => del.mutate(deleting.id, { onSuccess: () => setDeleting(null) })} onClose={() => { del.reset(); setDeleting(null); }} />}
    </div>
  );
}

function TeamDialog({ team, onClose }: { team: TeamRow | null; onClose: () => void }) {
  const save = useSaveTeam();
  const [name, setName] = useState(team?.name ?? '');
  const [desc, setDesc] = useState(team?.description ?? '');
  const [kind, setKind] = useState(team?.kind ?? 'internal');
  const err = save.error;
  const fieldErr = (n: string) => (err instanceof ApiError ? err.fields.find((f) => f.name === n)?.message : undefined);
  return (
    <Modal labelledBy="team-title" onClose={onClose} width={480}>
      <>
        <header className="dialog__head"><h2 id="team-title">{team ? 'Edit team' : 'New team'}</h2><button type="button" className="icon-btn" aria-label="Close" onClick={onClose}><X size={18} /></button></header>
        <form className="dialog__form" noValidate onSubmit={(e: FormEvent) => { e.preventDefault(); save.mutate({ id: team?.id, version: team?.version, name, description: desc.trim() || null, kind }, { onSuccess: onClose }); }}>
          {err && !(err instanceof ApiError && err.status === 422) && <div className="alert" role="alert"><strong>We couldn’t save</strong><span>{describeError(err).message}</span></div>}
          <Field label="Name" value={name} onChange={(e) => { save.reset(); setName(e.target.value); }} error={fieldErr('name')} autoFocus />
          <div className="textarea"><label htmlFor="team-desc">Description (optional)</label><textarea id="team-desc" rows={3} value={desc} onChange={(e) => setDesc(e.target.value)} /></div>
          <div className="select"><label htmlFor="team-kind">Kind</label><select id="team-kind" value={kind} onChange={(e) => setKind(e.target.value as 'internal' | 'external')}><option value="internal">Internal</option><option value="external">External</option></select></div>
          <footer className="dialog__foot"><button type="button" className="btn btn--secondary btn--auto" onClick={onClose}>Cancel</button>
            <button type="submit" className="btn btn--primary btn--auto" disabled={save.isPending}>{save.isPending ? 'Saving…' : 'Save'}</button></footer>
        </form>
      </>
    </Modal>
  );
}

function MembersDialog({ team, onClose }: { team: TeamRow; onClose: () => void }) {
  const detail = useTeam(team.id);
  const people = usePeople('');
  const set = useSetMembers(team.id);
  const [picked, setPicked] = useState<Set<string> | null>(null);
  const chosen = picked ?? new Set(detail.data?.members.map((m) => m.id) ?? []);
  const toggle = (id: string) => { const n = new Set(chosen); if (n.has(id)) n.delete(id); else n.add(id); setPicked(n); };
  return (
    <Modal labelledBy="members-title" onClose={onClose} width={520}>
      <>
        <header className="dialog__head"><h2 id="members-title">Members of {team.name}</h2><button type="button" className="icon-btn" aria-label="Close" onClick={onClose}><X size={18} /></button></header>
        {detail.isPending || people.isPending ? <span className="skeleton" /> : (people.data ?? []).length === 0 ? <p className="muted">Add people first, then put them in this team.</p> : (
          <div className="checklist">
            {people.data!.map((p) => (
              <label key={p.id} className="check"><input type="checkbox" checked={chosen.has(p.id)} onChange={() => toggle(p.id)} /><span className="check__box" aria-hidden="true" /><span>{p.first_name} {p.last_name}{p.email && <small className="check__sub">{p.email}</small>}</span></label>
            ))}
          </div>
        )}
        {set.isError && <div className="alert" role="alert"><strong>Couldn’t save members</strong><span>{describeError(set.error).message}</span></div>}
        <footer className="dialog__foot"><button type="button" className="btn btn--secondary btn--auto" onClick={onClose}>Cancel</button>
          <button type="button" className="btn btn--primary btn--auto" disabled={set.isPending || !detail.data} onClick={() => set.mutate({ version: detail.data!.version, person_ids: [...chosen] }, { onSuccess: onClose })}>{set.isPending ? 'Saving…' : 'Save members'}</button></footer>
      </>
    </Modal>
  );
}

function TeamsTab() {
  const { data: me } = useMe();
  const q = useTeams();
  const del = useDeleteTeam();
  const [editing, setEditing] = useState<TeamRow | 'new' | null>(null);
  const [members, setMembers] = useState<TeamRow | null>(null);
  const [deleting, setDeleting] = useState<TeamRow | null>(null);
  const write = can(me, 'people.write');
  return (
    <div className="stack">
      <div className="posture__head"><p className="muted">Teams group people for an exercise.</p>{write && <button type="button" className="btn btn--primary btn--auto" onClick={() => setEditing('new')}><Plus size={16} aria-hidden="true" /> New team</button>}</div>
      {q.isPending ? <span className="skeleton" /> : q.isError ? <div className="state state--error" role="alert"><h2>Couldn’t load teams</h2><p>{describeError(q.error).message}</p></div>
        : q.data!.length === 0 ? <div className="state"><h2>No teams yet</h2><p>Create a team, then add people to it.</p></div> : (
        <ul className="cards">
          {q.data!.map((t) => (
            <li key={t.id} className="card">
              <div className="card__top"><strong className="card__name">{t.name}</strong><span className="status">{t.kind === 'internal' ? 'Internal' : 'External'}</span></div>
              {t.description && <p className="card__rule">{t.description}</p>}
              {t.member_count === 0 && <p className="notice small">No members yet.</p>}
              <p className="card__count"><strong>{t.member_count}</strong> {t.member_count === 1 ? 'member' : 'members'}</p>
              {write && <div className="row-actions"><button type="button" className="btn btn--secondary btn--sm" onClick={() => setMembers(t)}>Members</button>
                <button type="button" className="btn btn--secondary btn--sm" aria-label={`Edit ${t.name}`} onClick={() => setEditing(t)}>Edit</button>
                <button type="button" className="btn btn--danger btn--sm" aria-label={`Delete ${t.name}`} onClick={() => setDeleting(t)}>Delete</button></div>}
            </li>
          ))}
        </ul>
      )}
      {editing && <TeamDialog team={editing === 'new' ? null : editing} onClose={() => setEditing(null)} />}
      {members && <MembersDialog team={members} onClose={() => setMembers(null)} />}
      {deleting && <ConfirmDialog title={`Delete ${deleting.name}?`} body="The people in it are kept." confirmLabel="Delete team" busy={del.isPending}
        error={del.isError ? describeError(del.error).message : undefined} onConfirm={() => del.mutate(deleting.id, { onSuccess: () => setDeleting(null) })} onClose={() => { del.reset(); setDeleting(null); }} />}
    </div>
  );
}

export function PeoplePage() {
  const { tab } = useParams();
  const teams = tab === 'teams';
  return (
    <section className="page" aria-labelledby="people-title">
      <div className="page__head"><div><h1 id="people-title" className="page__title">{teams ? 'Teams' : 'People'}</h1><p className="page__sub">Participants and teams for exercises. They are not platform accounts.</p></div></div>
      <nav className="tabs" aria-label="People sections">
        <NavLink to="/people" end className={({ isActive }) => `tab ${isActive ? 'is-active' : ''}`}>People</NavLink>
        <NavLink to="/people/teams" className={({ isActive }) => `tab ${isActive ? 'is-active' : ''}`}>Teams</NavLink>
      </nav>
      {teams ? <TeamsTab /> : <PeopleTab />}
    </section>
  );
}

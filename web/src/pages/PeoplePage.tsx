import { t as textUI, tr as richUI, useLocale, fv as formatUI } from "../i18n/runtime";
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
function PersonDialog({ person, onClose }: {
    person: PersonRow | null;
    onClose: () => void;
}) {
    useLocale();
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
    return (<Modal labelledBy="person-title" onClose={onClose} width={520}>
      <>
        <header className="dialog__head"><h2 id="person-title">{formatUI(person ? textUI("m_183d634af90a") : textUI("m_66848b5361f4"))}</h2><button type="button" className="icon-btn" aria-label={textUI("m_7d9eb7acb13e")} onClick={onClose}><X size={18}/></button></header>
        <form className="dialog__form" onSubmit={submit} noValidate>
          {formatUI(err && !(err instanceof ApiError && err.status === 422) && <div className="alert" role="alert"><strong>{textUI("m_e924a6d48f91")}</strong><span>{formatUI(describeError(err).message)}</span></div>)}
          <div className="row row--2">
            <Field label={textUI("m_702ef921ed1d")} value={first} onChange={(e) => { touch(); setFirst(e.target.value); }} error={fieldErr('first_name')} autoFocus/>
            <Field label={textUI("m_7b4888049459")} value={last} onChange={(e) => { touch(); setLast(e.target.value); }} error={fieldErr('last_name')}/>
          </div>
          <Field label={textUI("m_6eea8608c452")} value={email} onChange={(e) => { touch(); setEmail(e.target.value); }} error={fieldErr('email')}/>
          <Field label={textUI("m_56e22c741d2b")} value={org} onChange={(e) => { touch(); setOrg(e.target.value); }} error={fieldErr('organization')}/>
          <footer className="dialog__foot"><button type="button" className="btn btn--secondary btn--auto" onClick={onClose}>{textUI("m_19766ed6ccb2")}</button>
            <button type="submit" className="btn btn--primary btn--auto" disabled={save.isPending}>{formatUI(save.isPending ? textUI("m_23e39291d613") : textUI("m_1509f561f241"))}</button></footer>
        </form>
      </>
    </Modal>);
}
function PeopleTab() {
    useLocale();
    const { data: me } = useMe();
    const [text, setText] = useState('');
    const q = usePeople(useDebounced(text.trim(), 250));
    const del = useDeletePerson();
    const [params, setParams] = useSearchParams();
    const write = can(me, 'people.write');
    const [editing, setEditingState] = useState<PersonRow | 'new' | null>(params.get('new') === '1' && write ? 'new' : null);
    const setEditing = (v: PersonRow | 'new' | null) => { setEditingState(v); if (v === null && params.has('new'))
        setParams({}, { replace: true }); };
    const [deleting, setDeleting] = useState<PersonRow | null>(null);
    const items = q.data ?? [];
    return (<div className="stack">
      <div className="toolbar">
        <label className="search-input"><Search size={15} aria-hidden="true"/><input type="search" aria-label={textUI("m_8a7e7f1bc563")} placeholder={textUI("m_8a7e7f1bc563")} value={text} onChange={(e) => setText(e.target.value)}/></label>
        {formatUI(write && <button type="button" className="btn btn--primary btn--auto" onClick={() => setEditing('new')}>{richUI("m_a2e21fcd69fc", [<Plus size={16} aria-hidden="true"/>])}</button>)}
      </div>
      {formatUI(q.isPending ? <span className="skeleton"/> : q.isError ? (<div className="state state--error" role="alert"><h2>{textUI("m_b185c2339570")}</h2><p>{formatUI(describeError(q.error).message)}</p><button type="button" className="btn btn--secondary btn--auto" onClick={() => q.refetch()}>{textUI("m_d8b8392e2c54")}</button></div>) : items.length === 0 ? (<div className="state"><h2>{formatUI(text ? textUI("m_e05c583a6aff") : textUI("m_4ea16875623d"))}</h2><p>{formatUI(text ? textUI("m_2e6d79de50dc") : textUI("m_aa68df68d79b"))}</p>
          {formatUI(!text && write && <button type="button" className="btn btn--primary btn--auto" onClick={() => setEditing('new')}>{textUI("m_c705085accb3")}</button>)}</div>) : (<div className="list" role="table" aria-label={textUI("m_7db20897053b")} tabIndex={0}>
          <div className="list__head user-row" role="row"><span role="columnheader">{textUI("m_e47fb7cbdad3")}</span><span role="columnheader">{textUI("m_43747dc4ca90")}</span><span role="columnheader">{textUI("m_a3b0e8567eae")}</span><span role="columnheader"><span className="sr-only">{textUI("m_ff8059dc6752")}</span></span></div>
          {formatUI(items.map((p) => (<div key={p.id} className="asset-row people-row" role="row">
              <span role="cell" className="asset-row__text"><strong>{formatUI(p.first_name)} {formatUI(p.last_name)}</strong><span className="muted small">{formatUI(p.email ?? textUI("m_2f44d9d0a44f"))}</span></span>
              <span role="cell">{formatUI(p.organization ?? '—')}</span>
              <span role="cell">{formatUI(p.teams.length === 0 ? <span className="muted">—</span> : p.teams.map((t) => <span key={t.id} className="tag-chip">{formatUI(t.name)}</span>))}</span>
              <span role="cell" className="row-actions">{formatUI(write && <>
                <button type="button" className="btn btn--secondary btn--sm" aria-label={textUI("m_a00c0dc58c1d", [p.first_name, p.last_name])} onClick={() => setEditing(p)}>{textUI("m_464c4ffd019e")}</button>
                <button type="button" className="btn btn--danger btn--sm" aria-label={textUI("m_8c3e3b13c9ef", [p.first_name, p.last_name])} onClick={() => setDeleting(p)}>{textUI("m_e2d0a54968ea")}</button></>)}</span>
            </div>)))}
        </div>))}
      {formatUI(editing && <PersonDialog person={editing === 'new' ? null : editing} onClose={() => setEditing(null)}/>)}
      {formatUI(deleting && <ConfirmDialog title={textUI("m_22a8605b6580", [deleting.first_name, deleting.last_name])} body="They are removed from every team." confirmLabel="Delete person" busy={del.isPending} error={del.isError ? describeError(del.error).message : undefined} onConfirm={() => del.mutate(deleting.id, { onSuccess: () => setDeleting(null) })} onClose={() => { del.reset(); setDeleting(null); }}/>)}
    </div>);
}
function TeamDialog({ team, onClose }: {
    team: TeamRow | null;
    onClose: () => void;
}) {
    useLocale();
    const save = useSaveTeam();
    const [name, setName] = useState(team?.name ?? '');
    const [desc, setDesc] = useState(team?.description ?? '');
    const [kind, setKind] = useState(team?.kind ?? 'internal');
    const err = save.error;
    const fieldErr = (n: string) => (err instanceof ApiError ? err.fields.find((f) => f.name === n)?.message : undefined);
    return (<Modal labelledBy="team-title" onClose={onClose} width={480}>
      <>
        <header className="dialog__head"><h2 id="team-title">{formatUI(team ? textUI("m_ced8519d92a6") : textUI("m_1b9ed8eaa17d"))}</h2><button type="button" className="icon-btn" aria-label={textUI("m_7d9eb7acb13e")} onClick={onClose}><X size={18}/></button></header>
        <form className="dialog__form" noValidate onSubmit={(e: FormEvent) => { e.preventDefault(); save.mutate({ id: team?.id, version: team?.version, name, description: desc.trim() || null, kind }, { onSuccess: onClose }); }}>
          {formatUI(err && !(err instanceof ApiError && err.status === 422) && <div className="alert" role="alert"><strong>{textUI("m_e924a6d48f91")}</strong><span>{formatUI(describeError(err).message)}</span></div>)}
          <Field label={textUI("m_dcd1d5223f73")} value={name} onChange={(e) => { save.reset(); setName(e.target.value); }} error={fieldErr('name')} autoFocus/>
          <div className="textarea"><label htmlFor="team-desc">{textUI("m_f6cbe2f0c1f8")}</label><textarea id="team-desc" rows={3} value={desc} onChange={(e) => setDesc(e.target.value)}/></div>
          <div className="select"><label htmlFor="team-kind">{textUI("m_f5387f9bb6ed")}</label><select id="team-kind" value={kind} onChange={(e) => setKind(e.target.value as 'internal' | 'external')}><option value="internal">{textUI("m_2ea1842b445b")}</option><option value="external">{textUI("m_68c114ea9c8c")}</option></select></div>
          <footer className="dialog__foot"><button type="button" className="btn btn--secondary btn--auto" onClick={onClose}>{textUI("m_19766ed6ccb2")}</button>
            <button type="submit" className="btn btn--primary btn--auto" disabled={save.isPending}>{formatUI(save.isPending ? textUI("m_23e39291d613") : textUI("m_1509f561f241"))}</button></footer>
        </form>
      </>
    </Modal>);
}
function MembersDialog({ team, onClose }: {
    team: TeamRow;
    onClose: () => void;
}) {
    useLocale();
    const detail = useTeam(team.id);
    const people = usePeople('');
    const set = useSetMembers(team.id);
    const [picked, setPicked] = useState<Set<string> | null>(null);
    const chosen = picked ?? new Set(detail.data?.members.map((m) => m.id) ?? []);
    const toggle = (id: string) => { const n = new Set(chosen); if (n.has(id))
        n.delete(id);
    else
        n.add(id); setPicked(n); };
    return (<Modal labelledBy="members-title" onClose={onClose} width={520}>
      <>
        <header className="dialog__head"><h2 id="members-title">{richUI("m_d64fc5b1518a", [formatUI(team.name)])}</h2><button type="button" className="icon-btn" aria-label={textUI("m_7d9eb7acb13e")} onClick={onClose}><X size={18}/></button></header>
        {formatUI(detail.isPending || people.isPending ? <span className="skeleton"/> : (people.data ?? []).length === 0 ? <p className="muted">{textUI("m_b6708b3c5079")}</p> : (<div className="checklist">
            {formatUI(people.data!.map((p) => (<label key={p.id} className="check"><input type="checkbox" checked={chosen.has(p.id)} onChange={() => toggle(p.id)}/><span className="check__box" aria-hidden="true"/><span>{formatUI(p.first_name)} {formatUI(p.last_name)}{formatUI(p.email && <small className="check__sub">{formatUI(p.email)}</small>)}</span></label>)))}
          </div>))}
        {formatUI(set.isError && <div className="alert" role="alert"><strong>{textUI("m_fcbc6b497ac0")}</strong><span>{formatUI(describeError(set.error).message)}</span></div>)}
        <footer className="dialog__foot"><button type="button" className="btn btn--secondary btn--auto" onClick={onClose}>{textUI("m_19766ed6ccb2")}</button>
          <button type="button" className="btn btn--primary btn--auto" disabled={set.isPending || !detail.data} onClick={() => set.mutate({ version: detail.data!.version, person_ids: [...chosen] }, { onSuccess: onClose })}>{formatUI(set.isPending ? textUI("m_23e39291d613") : textUI("m_fdb2077bea12"))}</button></footer>
      </>
    </Modal>);
}
function TeamsTab() {
    useLocale();
    const { data: me } = useMe();
    const q = useTeams();
    const del = useDeleteTeam();
    const [editing, setEditing] = useState<TeamRow | 'new' | null>(null);
    const [members, setMembers] = useState<TeamRow | null>(null);
    const [deleting, setDeleting] = useState<TeamRow | null>(null);
    const write = can(me, 'people.write');
    return (<div className="stack">
      <div className="posture__head"><p className="muted">{textUI("m_3e663ee4ed6b")}</p>{formatUI(write && <button type="button" className="btn btn--primary btn--auto" onClick={() => setEditing('new')}>{richUI("m_382360b08123", [<Plus size={16} aria-hidden="true"/>])}</button>)}</div>
      {formatUI(q.isPending ? <span className="skeleton"/> : q.isError ? <div className="state state--error" role="alert"><h2>{textUI("m_d10dc2ee9460")}</h2><p>{formatUI(describeError(q.error).message)}</p></div>
            : q.data!.length === 0 ? <div className="state"><h2>{textUI("m_7df61e45ae97")}</h2><p>{textUI("m_7d34b1221646")}</p></div> : (<ul className="cards">
          {formatUI(q.data!.map((t) => (<li key={t.id} className="card">
              <div className="card__top"><strong className="card__name">{formatUI(t.name)}</strong><span className="status">{formatUI(t.kind === 'internal' ? textUI("m_2ea1842b445b") : textUI("m_68c114ea9c8c"))}</span></div>
              {formatUI(t.description && <p className="card__rule">{formatUI(t.description)}</p>)}
              {formatUI(t.member_count === 0 && <p className="notice small">{textUI("m_12dc83c54e6c")}</p>)}
              <p className="card__count"><strong>{formatUI(t.member_count)}</strong> {formatUI(t.member_count === 1 ? 'member' : 'members')}</p>
              {formatUI(write && <div className="row-actions"><button type="button" className="btn btn--secondary btn--sm" onClick={() => setMembers(t)}>{textUI("m_1044a4c056d0")}</button>
                <button type="button" className="btn btn--secondary btn--sm" aria-label={textUI("m_fdd9d5053ccc", [t.name])} onClick={() => setEditing(t)}>{textUI("m_464c4ffd019e")}</button>
                <button type="button" className="btn btn--danger btn--sm" aria-label={textUI("m_7eab122c2b58", [t.name])} onClick={() => setDeleting(t)}>{textUI("m_e2d0a54968ea")}</button></div>)}
            </li>)))}
        </ul>))}
      {formatUI(editing && <TeamDialog team={editing === 'new' ? null : editing} onClose={() => setEditing(null)}/>)}
      {formatUI(members && <MembersDialog team={members} onClose={() => setMembers(null)}/>)}
      {formatUI(deleting && <ConfirmDialog title={textUI("m_dc6c5ae4c245", [deleting.name])} body="The people in it are kept." confirmLabel="Delete team" busy={del.isPending} error={del.isError ? describeError(del.error).message : undefined} onConfirm={() => del.mutate(deleting.id, { onSuccess: () => setDeleting(null) })} onClose={() => { del.reset(); setDeleting(null); }}/>)}
    </div>);
}
export function PeoplePage() {
    useLocale();
    const { tab } = useParams();
    const teams = tab === 'teams';
    return (<section className="page" aria-labelledby="people-title">
      <div className="page__head"><div><h1 id="people-title" className="page__title">{formatUI(teams ? textUI("m_1e1a1c078af2") : textUI("m_7db20897053b"))}</h1><p className="page__sub">{textUI("m_9a09eb0a859c")}</p></div></div>
      <nav className="tabs" aria-label={textUI("m_78224e3a894f")}>
        <NavLink to="/people" end className={({ isActive }) => `tab ${isActive ? 'is-active' : ''}`}>{textUI("m_7db20897053b")}</NavLink>
        <NavLink to="/people/teams" className={({ isActive }) => `tab ${isActive ? 'is-active' : ''}`}>{textUI("m_1e1a1c078af2")}</NavLink>
      </nav>
      {formatUI(teams ? <TeamsTab /> : <PeopleTab />)}
    </section>);
}

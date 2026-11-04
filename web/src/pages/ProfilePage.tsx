import { t as textUI, tr as richUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { Plus, X } from 'lucide-react';
import { useState, type FormEvent } from 'react';
import { ApiError } from '../api/client';
import { useChangePassword, useCreateToken, useMe, useMyTokens, useRevokeToken, useUpdateProfile } from '../api/hooks';
import type { NewApiToken } from '../api/types';
import { CopyButton } from '../components/CopyButton';
import { describeError } from '../components/ErrorNote';
import { Field } from '../components/Field';
import { Modal } from '../components/Modal';
import { when } from '../components/RunBadges';
import { ConfirmDialog } from './ConfirmDialog';
function Details() {
    useLocale();
    const { data: me } = useMe();
    const save = useUpdateProfile();
    const [name, setName] = useState(me?.display_name ?? '');
    const err = save.error;
    const fieldErr = err instanceof ApiError ? err.fields.find((f) => f.name === 'display_name')?.message : undefined;
    return (<form className="panel" noValidate aria-label={textUI("m_73949bb1bdba")} onSubmit={(e: FormEvent) => { e.preventDefault(); save.mutate(name); }}>
      <h2 className="panel__heading">{textUI("m_c0bcd4c0f8e7")}</h2>
      <Field label={textUI("m_dcd1d5223f73")} value={name} onChange={(e) => { save.reset(); setName(e.target.value); }} error={fieldErr}/>
      <dl className="kv kv--2"><div><dt>{textUI("m_969ccbd3cf63")}</dt><dd>{formatUI(me?.email)}</dd></div><div><dt>{textUI("m_14736a2eb9f4")}</dt><dd>{formatUI(me?.role)}</dd></div></dl>
      {formatUI(err && !fieldErr && <div className="alert" role="alert"><strong>{textUI("m_a018b7a8a188")}</strong><span>{formatUI(describeError(err).message)}</span></div>)}
      {formatUI(save.isSuccess && <p className="muted small" role="status">{textUI("m_fca046ccd586")}</p>)}
      <div><button type="submit" className="btn btn--primary btn--auto" disabled={save.isPending || !name.trim() || name.trim() === me?.display_name}>{formatUI(save.isPending ? textUI("m_23e39291d613") : textUI("m_1509f561f241"))}</button></div>
    </form>);
}
function Password() {
    useLocale();
    const change = useChangePassword();
    const [cur, setCur] = useState('');
    const [next, setNext] = useState('');
    const err = change.error;
    const fieldErr = (n: string) => (err instanceof ApiError ? err.fields.find((f) => f.name === n)?.message : undefined);
    const submit = (e: FormEvent) => { e.preventDefault(); change.mutate({ current_password: cur, new_password: next }, { onSuccess: () => { setCur(''); setNext(''); } }); };
    return (<form className="panel" noValidate aria-label={textUI("m_3f9c991f63a9")} onSubmit={submit}>
      <h2 className="panel__heading">{textUI("m_e7cf3ef4f17c")}</h2>
      <Field label={textUI("m_72ed2bd767ce")} type="password" autoComplete="current-password" value={cur} onChange={(e) => { change.reset(); setCur(e.target.value); }} error={fieldErr('current_password')}/>
      <Field label={textUI("m_3dd9df4441fb")} type="password" autoComplete="new-password" value={next} onChange={(e) => { change.reset(); setNext(e.target.value); }} error={fieldErr('new_password')} hint={textUI("m_b8ec1f61313b")}/>
      {formatUI(err && !(err instanceof ApiError && err.status === 422) && <div className="alert" role="alert"><strong>{textUI("m_eed73dab8440")}</strong><span>{formatUI(describeError(err).message)}</span></div>)}
      {formatUI(change.isSuccess && <p className="muted small" role="status">{textUI("m_28043045d17a")}</p>)}
      <div><button type="submit" className="btn btn--primary btn--auto" disabled={change.isPending || !cur || !next}>{formatUI(change.isPending ? textUI("m_92a0c89d7c37") : textUI("m_3f9c991f63a9"))}</button></div>
    </form>);
}
function NewTokenDialog({ onClose, onCreated }: {
    onClose: () => void;
    onCreated: (t: NewApiToken) => void;
}) {
    useLocale();
    const create = useCreateToken();
    const [name, setName] = useState('');
    const [days, setDays] = useState('90');
    const err = create.error;
    const fieldErr = (n: string) => (err instanceof ApiError ? err.fields.find((f) => f.name === n)?.message : undefined);
    return (<Modal labelledBy="ntok-title" onClose={onClose} width={480}>
      <>
        <header className="dialog__head"><h2 id="ntok-title">{textUI("m_dc58d8b7248f")}</h2><button type="button" className="icon-btn" aria-label={textUI("m_7d9eb7acb13e")} onClick={onClose}><X size={18}/></button></header>
        <form className="dialog__form" noValidate onSubmit={(e: FormEvent) => { e.preventDefault(); create.mutate({ name, expires_in_days: Number(days) || 90 }, { onSuccess: (t) => { onClose(); onCreated(t); } }); }}>
          <Field label={textUI("m_dcd1d5223f73")} value={name} onChange={(e) => { create.reset(); setName(e.target.value); }} error={fieldErr('name')} placeholder={textUI("m_79601827b68e")} autoFocus/>
          <div className="select"><label htmlFor="tok-days">{textUI("m_a5e4b9f547aa")}</label>
            <select id="tok-days" value={days} onChange={(e) => setDays(e.target.value)}><option value="30">{textUI("m_ffd7280513ce")}</option><option value="90">{textUI("m_38825b0f646b")}</option><option value="365">{textUI("m_91647badc37b")}</option></select></div>
          <p className="muted small">{textUI("m_72c1b3e9bdb0")}</p>
          {formatUI(err && !(err instanceof ApiError && err.status === 422) && <div className="alert" role="alert"><strong>{textUI("m_7fb00f9af686")}</strong><span>{formatUI(describeError(err).message)}</span></div>)}
          <footer className="dialog__foot"><button type="button" className="btn btn--secondary btn--auto" onClick={onClose}>{textUI("m_19766ed6ccb2")}</button>
            <button type="submit" className="btn btn--primary btn--auto" disabled={create.isPending}>{formatUI(create.isPending ? textUI("m_c79ed9492e3c") : textUI("m_5d8e8e30bc4f"))}</button></footer>
        </form>
      </>
    </Modal>);
}
function Tokens() {
    useLocale();
    const q = useMyTokens();
    const revoke = useRevokeToken();
    const [creating, setCreating] = useState(false);
    const [secret, setSecret] = useState<NewApiToken | null>(null);
    const [revoking, setRevoking] = useState<{
        id: string;
        name: string;
    } | null>(null);
    return (<section className="panel" aria-labelledby="tokens-title">
      <div className="posture__head"><div><h2 id="tokens-title" className="panel__heading">{textUI("m_fa884c0e7a4a")}</h2><p className="muted small">{richUI("m_f4d0132fb545", [<code className="mono">Authorization: Bearer …</code>])}</p></div>
        <button type="button" className="btn btn--secondary btn--auto" onClick={() => setCreating(true)}>{richUI("m_9ecf8ea138ec", [<Plus size={16} aria-hidden="true"/>])}</button></div>
      {formatUI(q.isPending ? <span className="skeleton"/> : q.isError ? <p role="alert">{formatUI(describeError(q.error).message)}</p> : q.data.length === 0 ? <p className="muted">{textUI("m_ebb639f0445d")}</p> : (<ul className="mini-list" aria-label={textUI("m_fa884c0e7a4a")}>
          {formatUI(q.data.map((t) => (<li key={t.id}><div className="mini-list__main"><strong>{formatUI(t.name)}</strong><span className="muted small">{richUI("m_e01055b0b766", [formatUI(t.is_demo ? textUI("m_e048ac26138f") : <code className="mono">{t.prefix}…</code>), formatUI(t.last_used_at ? textUI("m_9bad3eff6f2d", [when(t.last_used_at)]) : textUI("m_dd57c2813b0b")), formatUI(when(t.expires_at))])}</span></div>
              {formatUI(!t.is_demo && <button type="button" className="btn btn--danger btn--sm" aria-label={textUI("m_02ff89306904", [t.name])} onClick={() => setRevoking({ id: t.id, name: t.name })}>{textUI("m_87e6d00bbf53")}</button>)}</li>)))}
        </ul>))}
      {formatUI(creating && <NewTokenDialog onClose={() => setCreating(false)} onCreated={setSecret}/>)}
      {formatUI(secret && (<Modal labelledBy="stok-title" onClose={() => setSecret(null)} width={560}>
          <>
            <header className="dialog__head"><h2 id="stok-title">{textUI("m_f4d15ff00226")}</h2></header>
            <p className="subtitle">{textUI("m_080ec17bf7fe")}</p>
            <div className="secret"><code className="mono" data-testid="api-token">{secret.secret}</code><CopyButton value={secret.secret}/></div>
            <footer className="dialog__foot"><button type="button" className="btn btn--primary btn--auto" onClick={() => setSecret(null)}>{textUI("m_11a6767d5674")}</button></footer>
          </>
        </Modal>))}
      {formatUI(revoking && <ConfirmDialog title={textUI("m_fa6b2ee31f5b", [revoking.name])} body="Anything using this token stops working immediately." confirmLabel="Revoke token" busy={revoke.isPending} error={revoke.isError ? describeError(revoke.error).message : undefined} onConfirm={() => revoke.mutate(revoking.id, { onSuccess: () => setRevoking(null) })} onClose={() => setRevoking(null)}/>)}
    </section>);
}
export function ProfilePage() {
    useLocale();
    return (<section className="page" aria-labelledby="profile-title">
      <div className="page__head"><div><h1 id="profile-title" className="page__title">{textUI("m_d696a35bdd18")}</h1><p className="page__sub">{textUI("m_18e560972a53")}</p></div></div>
      <div className="stack"><Details /><Password /><Tokens /></div>
    </section>);
}

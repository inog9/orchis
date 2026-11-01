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
  const { data: me } = useMe();
  const save = useUpdateProfile();
  const [name, setName] = useState(me?.display_name ?? '');
  const err = save.error;
  const fieldErr = err instanceof ApiError ? err.fields.find((f) => f.name === 'display_name')?.message : undefined;
  return (
    <form className="panel" noValidate aria-label="Profile details" onSubmit={(e: FormEvent) => { e.preventDefault(); save.mutate(name); }}>
      <h2 className="panel__heading">Your details</h2>
      <Field label="Name" value={name} onChange={(e) => { save.reset(); setName(e.target.value); }} error={fieldErr} />
      <dl className="kv kv--2"><div><dt>Email</dt><dd>{me?.email}</dd></div><div><dt>Role</dt><dd>{me?.role}</dd></div></dl>
      {err && !fieldErr && <div className="alert" role="alert"><strong>Couldn’t save</strong><span>{describeError(err).message}</span></div>}
      {save.isSuccess && <p className="muted small" role="status">Saved.</p>}
      <div><button type="submit" className="btn btn--primary btn--auto" disabled={save.isPending || !name.trim() || name.trim() === me?.display_name}>{save.isPending ? 'Saving…' : 'Save'}</button></div>
    </form>
  );
}

function Password() {
  const change = useChangePassword();
  const [cur, setCur] = useState('');
  const [next, setNext] = useState('');
  const err = change.error;
  const fieldErr = (n: string) => (err instanceof ApiError ? err.fields.find((f) => f.name === n)?.message : undefined);
  const submit = (e: FormEvent) => { e.preventDefault(); change.mutate({ current_password: cur, new_password: next }, { onSuccess: () => { setCur(''); setNext(''); } }); };
  return (
    <form className="panel" noValidate aria-label="Change password" onSubmit={submit}>
      <h2 className="panel__heading">Password</h2>
      <Field label="Current password" type="password" autoComplete="current-password" value={cur} onChange={(e) => { change.reset(); setCur(e.target.value); }} error={fieldErr('current_password')} />
      <Field label="New password" type="password" autoComplete="new-password" value={next} onChange={(e) => { change.reset(); setNext(e.target.value); }} error={fieldErr('new_password')} hint="At least 12 characters, with upper and lower case letters and a digit or symbol." />
      {err && !(err instanceof ApiError && err.status === 422) && <div className="alert" role="alert"><strong>Couldn’t change it</strong><span>{describeError(err).message}</span></div>}
      {change.isSuccess && <p className="muted small" role="status">Password changed. Your other sessions were signed out.</p>}
      <div><button type="submit" className="btn btn--primary btn--auto" disabled={change.isPending || !cur || !next}>{change.isPending ? 'Changing…' : 'Change password'}</button></div>
    </form>
  );
}

function NewTokenDialog({ onClose, onCreated }: { onClose: () => void; onCreated: (t: NewApiToken) => void }) {
  const create = useCreateToken();
  const [name, setName] = useState('');
  const [days, setDays] = useState('90');
  const err = create.error;
  const fieldErr = (n: string) => (err instanceof ApiError ? err.fields.find((f) => f.name === n)?.message : undefined);
  return (
    <Modal labelledBy="ntok-title" onClose={onClose} width={480}>
      <>
        <header className="dialog__head"><h2 id="ntok-title">New API token</h2><button type="button" className="icon-btn" aria-label="Close" onClick={onClose}><X size={18} /></button></header>
        <form className="dialog__form" noValidate onSubmit={(e: FormEvent) => { e.preventDefault(); create.mutate({ name, expires_in_days: Number(days) || 90 }, { onSuccess: (t) => { onClose(); onCreated(t); } }); }}>
          <Field label="Name" value={name} onChange={(e) => { create.reset(); setName(e.target.value); }} error={fieldErr('name')} placeholder="CI pipeline" autoFocus />
          <div className="select"><label htmlFor="tok-days">Expires after</label>
            <select id="tok-days" value={days} onChange={(e) => setDays(e.target.value)}><option value="30">30 days</option><option value="90">90 days</option><option value="365">1 year</option></select></div>
          <p className="muted small">A token can do everything your role can, for as long as it lasts.</p>
          {err && !(err instanceof ApiError && err.status === 422) && <div className="alert" role="alert"><strong>Couldn’t create it</strong><span>{describeError(err).message}</span></div>}
          <footer className="dialog__foot"><button type="button" className="btn btn--secondary btn--auto" onClick={onClose}>Cancel</button>
            <button type="submit" className="btn btn--primary btn--auto" disabled={create.isPending}>{create.isPending ? 'Creating…' : 'Create token'}</button></footer>
        </form>
      </>
    </Modal>
  );
}

function Tokens() {
  const q = useMyTokens();
  const revoke = useRevokeToken();
  const [creating, setCreating] = useState(false);
  const [secret, setSecret] = useState<NewApiToken | null>(null);
  const [revoking, setRevoking] = useState<{ id: string; name: string } | null>(null);
  return (
    <section className="panel" aria-labelledby="tokens-title">
      <div className="posture__head"><div><h2 id="tokens-title" className="panel__heading">API tokens</h2><p className="muted small">For scripts and integrations. Send as <code className="mono">Authorization: Bearer …</code>.</p></div>
        <button type="button" className="btn btn--secondary btn--auto" onClick={() => setCreating(true)}><Plus size={16} aria-hidden="true" /> New token</button></div>
      {q.isPending ? <span className="skeleton" /> : q.isError ? <p role="alert">{describeError(q.error).message}</p> : q.data.length === 0 ? <p className="muted">You have no active tokens.</p> : (
        <ul className="mini-list" aria-label="API tokens">
          {q.data.map((t) => (
            <li key={t.id}><div className="mini-list__main"><strong>{t.name}</strong><span className="muted small">{t.is_demo ? 'Synthetic example · revoked · cannot authenticate' : <code className="mono">{t.prefix}…</code>} · {t.last_used_at ? `used ${when(t.last_used_at)}` : 'never used'} · expires {when(t.expires_at)}</span></div>
              {!t.is_demo && <button type="button" className="btn btn--danger btn--sm" aria-label={`Revoke ${t.name}`} onClick={() => setRevoking({ id: t.id, name: t.name })}>Revoke</button>}</li>
          ))}
        </ul>
      )}
      {creating && <NewTokenDialog onClose={() => setCreating(false)} onCreated={setSecret} />}
      {secret && (
        <Modal labelledBy="stok-title" onClose={() => setSecret(null)} width={560}>
          <>
            <header className="dialog__head"><h2 id="stok-title">Copy your token now</h2></header>
            <p className="subtitle">This is the only time it is shown. Store it somewhere safe.</p>
            <div className="secret"><code className="mono" data-testid="api-token">{secret.secret}</code><CopyButton value={secret.secret} /></div>
            <footer className="dialog__foot"><button type="button" className="btn btn--primary btn--auto" onClick={() => setSecret(null)}>Done</button></footer>
          </>
        </Modal>
      )}
      {revoking && <ConfirmDialog title={`Revoke “${revoking.name}”?`} body="Anything using this token stops working immediately." confirmLabel="Revoke token" busy={revoke.isPending}
        error={revoke.isError ? describeError(revoke.error).message : undefined} onConfirm={() => revoke.mutate(revoking.id, { onSuccess: () => setRevoking(null) })} onClose={() => setRevoking(null)} />}
    </section>
  );
}

export function ProfilePage() {
  return (
    <section className="page" aria-labelledby="profile-title">
      <div className="page__head"><div><h1 id="profile-title" className="page__title">Profile</h1><p className="page__sub">Your account, password and API tokens.</p></div></div>
      <div className="stack"><Details /><Password /><Tokens /></div>
    </section>
  );
}

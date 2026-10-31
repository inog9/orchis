import { Plus, X } from 'lucide-react';
import { useState, type FormEvent } from 'react';
import { ApiError } from '../api/client';
import { useCreateUser, useMe, usePatchUser, useResetLink, useRoles, useUsers } from '../api/hooks';
import type { ResetLink, UserRow } from '../api/types';
import { CopyButton } from '../components/CopyButton';
import { describeError } from '../components/ErrorNote';
import { Field } from '../components/Field';
import { Modal } from '../components/Modal';
import { when } from '../components/RunBadges';

export function resetUrl(token: string) {
  return `${window.location.origin}/reset?token=${encodeURIComponent(token)}`;
}

// Link reset tampil SEKALI. Setelah dialog ditutup, admin harus membuat link baru.
export function ResetLinkDialog({ who, link, onClose }: { who: string; link: ResetLink; onClose: () => void }) {
  const url = resetUrl(link.token);
  return (
    <Modal labelledBy="reset-title" onClose={onClose} width={560}>
      <>
        <header className="dialog__head"><h2 id="reset-title">Link for {who}</h2><button type="button" className="icon-btn" aria-label="Close" onClick={onClose}><X size={18} /></button></header>
        <p className="subtitle">Send this link to {who}. They use it once to choose their password. It expires {when(link.expires_at)} (30 minutes). It won’t be shown again.</p>
        <div className="secret"><code className="mono" data-testid="reset-url">{url}</code><CopyButton value={url} /></div>
        <footer className="dialog__foot"><button type="button" className="btn btn--primary btn--auto" onClick={onClose}>Done</button></footer>
      </>
    </Modal>
  );
}

function UserDialog({ user, onClose, onLink }: { user: UserRow | null; onClose: () => void; onLink: (who: string, l: ResetLink) => void }) {
  const roles = useRoles();
  const create = useCreateUser();
  const patch = usePatchUser();
  const [email, setEmail] = useState(user?.email ?? '');
  const [name, setName] = useState(user?.display_name ?? '');
  const [role, setRole] = useState(user?.role ?? 'analyst');
  const [status, setStatus] = useState(user?.status ?? 'active');
  const m = user ? patch : create;
  const err = m.error;
  const fieldErr = (n: string) => (err instanceof ApiError ? err.fields.find((f) => f.name === n)?.message : undefined);
  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (user) patch.mutate({ id: user.id, version: user.version, display_name: name, role, status }, { onSuccess: onClose });
    else create.mutate({ email, display_name: name, role }, { onSuccess: (c) => { onClose(); onLink(c.user.display_name, c.reset_link); } });
  };
  return (
    <Modal labelledBy="user-title" onClose={onClose} width={520}>
      <>
        <header className="dialog__head"><h2 id="user-title">{user ? 'Edit account' : 'New user'}</h2><button type="button" className="icon-btn" aria-label="Close" onClick={onClose}><X size={18} /></button></header>
        <form className="dialog__form" onSubmit={submit} noValidate>
          {err && !(err instanceof ApiError && err.status === 422) && <div className="alert" role="alert"><strong>We couldn’t save</strong><span>{describeError(err).message}</span></div>}
          <Field label="Email" value={email} disabled={!!user} onChange={(e) => { m.reset(); setEmail(e.target.value); }} error={fieldErr('email')} placeholder="name@company.com" autoFocus={!user} />
          <Field label="Name" value={name} onChange={(e) => { m.reset(); setName(e.target.value); }} error={fieldErr('display_name')} />
          <div className="select"><label htmlFor="u-role">Role</label>
            <select id="u-role" value={role} onChange={(e) => { m.reset(); setRole(e.target.value); }}>
              {(roles.data ?? []).map((r) => <option key={r.key} value={r.key}>{r.name}</option>)}
            </select>
            {fieldErr('role') && <small className="field__error" role="alert">{fieldErr('role')}</small>}
          </div>
          {user && <div className="select"><label htmlFor="u-status">Status</label>
            <select id="u-status" value={status} onChange={(e) => setStatus(e.target.value as 'active' | 'disabled')}><option value="active">Active</option><option value="disabled">Disabled (can’t sign in)</option></select></div>}
          {!user && <p className="muted small">No email is sent. After you create the account you get a one-time link to give to the person.</p>}
          <footer className="dialog__foot">
            <button type="button" className="btn btn--secondary btn--auto" onClick={onClose}>Cancel</button>
            <button type="submit" className="btn btn--primary btn--auto" disabled={m.isPending}>{m.isPending ? 'Saving…' : user ? 'Save' : 'Create user'}</button>
          </footer>
        </form>
      </>
    </Modal>
  );
}

export function UsersPanel() {
  const { data: me } = useMe();
  const q = useUsers();
  const link = useResetLink();
  const [editing, setEditing] = useState<UserRow | 'new' | null>(null);
  const [shown, setShown] = useState<{ who: string; link: ResetLink } | null>(null);
  const items = q.data ?? [];
  return (
    <div className="stack">
      <div className="posture__head"><p className="muted">People who can sign in to this platform. Passwords are set through one-time links you hand over; no email is sent.</p>
        <button type="button" className="btn btn--primary btn--auto" onClick={() => setEditing('new')}><Plus size={16} aria-hidden="true" /> New user</button></div>
      {link.isError && <div className="alert" role="alert"><strong>Couldn’t create the link</strong><span>{describeError(link.error).message}</span></div>}
      {q.isPending ? <span className="skeleton" /> : q.isError ? (
        <div className="state state--error" role="alert"><h2>Couldn’t load users</h2><p>{describeError(q.error).message}</p><button type="button" className="btn btn--secondary btn--auto" onClick={() => q.refetch()}>Try again</button></div>
      ) : (
        <div className="list" role="table" aria-label="Users" tabIndex={0}>
          <div className="list__head user-row" role="row"><span role="columnheader">USER</span><span role="columnheader">ROLE</span><span role="columnheader">STATUS</span><span role="columnheader">LAST SIGN-IN</span><span role="columnheader"><span className="sr-only">Actions</span></span></div>
          {items.map((u) => (
            <div key={u.id} className="asset-row user-row" role="row">
              <span role="cell" className="asset-row__text"><strong>{u.display_name}{u.id === me?.id ? ' (you)' : ''}</strong><span className="muted small">{u.email}</span></span>
              <span role="cell">{u.role_name}</span>
              <span role="cell"><span className={`status ${u.status === 'active' ? (u.has_password ? 'status--ok' : '') : 'status--bad'}`}>{u.status === 'disabled' ? 'Disabled' : u.has_password ? 'Active' : 'Invited'}</span></span>
              <span role="cell" className="muted small">{u.last_login_at ? when(u.last_login_at) : 'Never'}</span>
              <span role="cell" className="row-actions">
                <button type="button" className="btn btn--secondary btn--sm" onClick={() => setEditing(u)} aria-label={`Edit ${u.display_name}`}>Edit</button>
                {u.status === 'active' && <button type="button" className="btn btn--secondary btn--sm" aria-label={`Reset link for ${u.display_name}`} disabled={link.isPending}
                  onClick={() => link.mutate(u.id, { onSuccess: (l) => setShown({ who: u.display_name, link: l }) })}>Reset link</button>}
              </span>
            </div>
          ))}
        </div>
      )}
      {editing && <UserDialog user={editing === 'new' ? null : editing} onClose={() => setEditing(null)} onLink={(who, l) => setShown({ who, link: l })} />}
      {shown && <ResetLinkDialog who={shown.who} link={shown.link} onClose={() => setShown(null)} />}
    </div>
  );
}

export function RolesPanel() {
  const q = useRoles();
  if (q.isPending) return <span className="skeleton" />;
  if (q.isError) return <div className="state state--error" role="alert"><h2>Couldn’t load roles</h2><p>{describeError(q.error).message}</p></div>;
  return (
    <div className="stack">
      <p className="muted">Roles are fixed in this version. Each one is a set of permissions.</p>
      <div className="cards">
        {q.data!.map((r) => (
          <section key={r.key} className="card" aria-label={r.name}>
            <div className="card__top"><h2 className="card__name">{r.name}</h2><span className="badge">{r.users} {r.users === 1 ? 'user' : 'users'}</span></div>
            <p className="card__rule">{r.description}</p>
            <ul className="perms">{r.permissions.map((p) => <li key={p} className="mono small">{p}</li>)}</ul>
          </section>
        ))}
      </div>
    </div>
  );
}

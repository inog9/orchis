import { t as textUI, tr as richUI, useLocale, fv as formatUI } from "../i18n/runtime";
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
export function ResetLinkDialog({ who, link, onClose }: {
    who: string;
    link: ResetLink;
    onClose: () => void;
}) {
    useLocale();
    const url = resetUrl(link.token);
    return (<Modal labelledBy="reset-title" onClose={onClose} width={560}>
      <>
        <header className="dialog__head"><h2 id="reset-title">{richUI("m_6eabeec54495", [formatUI(who)])}</h2><button type="button" className="icon-btn" aria-label={textUI("m_7d9eb7acb13e")} onClick={onClose}><X size={18}/></button></header>
        <p className="subtitle">{richUI("m_960e8e8f737d", [formatUI(who), formatUI(when(link.expires_at))])}</p>
        <div className="secret"><code className="mono" data-testid="reset-url">{url}</code><CopyButton value={url}/></div>
        <footer className="dialog__foot"><button type="button" className="btn btn--primary btn--auto" onClick={onClose}>{textUI("m_11a6767d5674")}</button></footer>
      </>
    </Modal>);
}
function UserDialog({ user, onClose, onLink }: {
    user: UserRow | null;
    onClose: () => void;
    onLink: (who: string, l: ResetLink) => void;
}) {
    useLocale();
    const roles = useRoles();
    const create = useCreateUser();
    const patch = usePatchUser();
    const [email, setEmail] = useState(user?.email ?? '');
    const [name, setName] = useState(user?.display_name ?? '');
    const [role, setRole] = useState(user?.role ?? 'analyst');
    const [accepted, setAccepted] = useState(false);
    const [status, setStatus] = useState(user?.status ?? 'active');
    const m = user ? patch : create;
    const err = m.error;
    const fieldErr = (n: string) => (err instanceof ApiError ? err.fields.find((f) => f.name === n)?.message : undefined);
    const submit = (e: FormEvent) => {
        e.preventDefault();
        if (user)
            patch.mutate({ id: user.id, version: user.version, display_name: name, role, status, accept_role_permissions: accepted }, { onSuccess: onClose });
        else
            create.mutate({ email, display_name: name, role }, { onSuccess: (c) => { onClose(); onLink(c.user.display_name, c.reset_link); } });
    };
    return (<Modal labelledBy="user-title" onClose={onClose} width={520}>
      <>
        <header className="dialog__head"><h2 id="user-title">{formatUI(user ? textUI("m_ab6a164be0c3") : textUI("m_6384ffa7712c"))}</h2><button type="button" className="icon-btn" aria-label={textUI("m_7d9eb7acb13e")} onClick={onClose}><X size={18}/></button></header>
        <form className="dialog__form" onSubmit={submit} noValidate>
          {formatUI(err && !(err instanceof ApiError && err.status === 422) && <div className="alert" role="alert"><strong>{textUI("m_e924a6d48f91")}</strong><span>{formatUI(describeError(err).message)}</span></div>)}
          <Field label={textUI("m_969ccbd3cf63")} value={email} disabled={!!user} onChange={(e) => { m.reset(); setEmail(e.target.value); }} error={fieldErr('email')} placeholder={textUI("m_35d973d92927")} autoFocus={!user}/>
          <Field label={textUI("m_dcd1d5223f73")} value={name} onChange={(e) => { m.reset(); setName(e.target.value); }} error={fieldErr('display_name')}/>
          <div className="select"><label htmlFor="u-role">{textUI("m_14736a2eb9f4")}</label>
            <select id="u-role" value={role} onChange={(e) => { m.reset(); setAccepted(false); setRole(e.target.value); }}>
              {formatUI((roles.data ?? []).map((r) => <option key={r.key} value={r.key}>{formatUI(r.name)}</option>))}
            </select>
            {formatUI(fieldErr('role') && <small className="field__error" role="alert">{formatUI(fieldErr('role'))}</small>)}
          </div>
          {formatUI(user && <div className="select"><label htmlFor="u-status">{textUI("m_920e413c7d41")}</label>
            <select id="u-status" value={status} onChange={(e) => setStatus(e.target.value as 'active' | 'disabled')}><option value="active">{textUI("m_92340695899b")}</option><option value="disabled">{textUI("m_913a4ab22e2d")}</option></select></div>)}
          {formatUI(user?.role_review_required && <div className="notice"><p>{richUI("m_198fd0c8316e", [formatUI(user.previous_role_key)])}</p><p>{formatUI(roles.data?.find(r => r.key === role)?.description)}</p><label>{richUI("m_2f96ded35e38", [<input type="checkbox" checked={accepted} onChange={e => setAccepted(e.target.checked)}/>])}</label>{formatUI(fieldErr('accept_role_permissions') && <p role="alert">{formatUI(fieldErr('accept_role_permissions'))}</p>)}</div>)}
          {formatUI(!user && <p className="muted small">{textUI("m_768fcb924255")}</p>)}
          <footer className="dialog__foot">
            <button type="button" className="btn btn--secondary btn--auto" onClick={onClose}>{textUI("m_19766ed6ccb2")}</button>
            <button type="submit" className="btn btn--primary btn--auto" disabled={m.isPending}>{formatUI(m.isPending ? textUI("m_23e39291d613") : user ? textUI("m_1509f561f241") : textUI("m_f06da128c342"))}</button>
          </footer>
        </form>
      </>
    </Modal>);
}
export function UsersPanel() {
    useLocale();
    const { data: me } = useMe();
    const q = useUsers();
    const link = useResetLink();
    const [editing, setEditing] = useState<UserRow | 'new' | null>(null);
    const [shown, setShown] = useState<{
        who: string;
        link: ResetLink;
    } | null>(null);
    const items = q.data ?? [];
    return (<div className="stack">
      <div className="posture__head"><p className="muted">{textUI("m_34815b36c35c")}</p>
        <button type="button" className="btn btn--primary btn--auto" onClick={() => setEditing('new')}>{richUI("m_b12d6c172d0e", [<Plus size={16} aria-hidden="true"/>])}</button></div>
      {formatUI(link.isError && <div className="alert" role="alert"><strong>{textUI("m_c6815fe39c43")}</strong><span>{formatUI(describeError(link.error).message)}</span></div>)}
      {formatUI(q.isPending ? <span className="skeleton"/> : q.isError ? (<div className="state state--error" role="alert"><h2>{textUI("m_21ca1cb87d4f")}</h2><p>{formatUI(describeError(q.error).message)}</p><button type="button" className="btn btn--secondary btn--auto" onClick={() => q.refetch()}>{textUI("m_d8b8392e2c54")}</button></div>) : (<div className="list" role="table" aria-label={textUI("m_6b0cc904d081")} tabIndex={0}>
          <div className="list__head user-row" role="row"><span role="columnheader">{textUI("m_92b7b421992e")}</span><span role="columnheader">{textUI("m_688783f5f094")}</span><span role="columnheader">{textUI("m_8c2e4a035f5f")}</span><span role="columnheader">{textUI("m_c14fd5f4bf94")}</span><span role="columnheader"><span className="sr-only">{textUI("m_ff8059dc6752")}</span></span></div>
          {formatUI(items.map((u) => (<div key={u.id} className="asset-row user-row" role="row">
              <span role="cell" className="asset-row__text"><strong>{formatUI(u.display_name)}{formatUI(u.id === me?.id ? ' (you)' : '')}</strong><span className="muted small">{formatUI(u.email)}</span></span>
              <span role="cell">{formatUI(u.role_name)}</span>
              <span role="cell"><span className={`status ${u.status === 'active' ? (u.has_password ? 'status--ok' : '') : 'status--bad'}`}>{formatUI(u.role_review_required ? textUI("m_f0742e3a82fe") : u.status === 'disabled' ? textUI("m_75081b593d15") : u.has_password ? textUI("m_92340695899b") : textUI("m_63b17becd812"))}</span></span>
              <span role="cell" className="muted small">{formatUI(u.last_login_at ? when(u.last_login_at) : textUI("m_6300ef800bb8"))}</span>
              <span role="cell" className="row-actions">
                <button type="button" className="btn btn--secondary btn--sm" onClick={() => setEditing(u)} aria-label={textUI("m_fdd9d5053ccc", [u.display_name])}>{textUI("m_464c4ffd019e")}</button>
                {formatUI(u.status === 'active' && <button type="button" className="btn btn--secondary btn--sm" aria-label={textUI("m_7d7a286c2de8", [u.display_name])} disabled={link.isPending} onClick={() => link.mutate(u.id, { onSuccess: (l) => setShown({ who: u.display_name, link: l }) })}>{textUI("m_118ce03de3e3")}</button>)}
              </span>
            </div>)))}
        </div>))}
      {formatUI(editing && <UserDialog user={editing === 'new' ? null : editing} onClose={() => setEditing(null)} onLink={(who, l) => setShown({ who, link: l })}/>)}
      {formatUI(shown && <ResetLinkDialog who={shown.who} link={shown.link} onClose={() => setShown(null)}/>)}
    </div>);
}
export function RolesPanel() {
    useLocale();
    const q = useRoles();
    if (q.isPending)
        return <span className="skeleton"/>;
    if (q.isError)
        return <div className="state state--error" role="alert"><h2>{textUI("m_5bfd02886238")}</h2><p>{formatUI(describeError(q.error).message)}</p></div>;
    return (<div className="stack">
      <p className="muted">{textUI("m_cd16f6d7f103")}</p>
      <div className="cards">
        {formatUI(q.data!.map((r) => (<section key={r.key} className="card" aria-label={r.name}>
            <div className="card__top"><h2 className="card__name">{formatUI(r.name)}</h2><span className="badge">{formatUI(r.users)} {formatUI(r.users === 1 ? 'user' : 'users')}</span></div>
            <p className="card__rule">{formatUI(r.description)}</p>
            <ul className="perms">{formatUI(r.permissions.map((p) => <li key={p} className="mono small">{formatUI(p)}</li>))}</ul>
          </section>)))}
      </div>
    </div>);
}

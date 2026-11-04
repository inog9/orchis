import { t as textUI, tr as richUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { useNavigate, useSearchParams } from 'react-router-dom';
import { useAlertAction, useAlerts } from '../api/hooks';
import type { AlertRow } from '../api/types';
import { describeError } from '../components/ErrorNote';
import { useMe } from '../api/hooks';
import { can } from '../api/types';
import { OrganizationFilter } from '../components/OrganizationFilter';
import { NotificationPolicy } from '../components/NotificationPolicy';
import { notificationTitle } from '../i18n/notifications';
import { when } from '../components/RunBadges';
// Ke mana sebuah alert membawa pengguna.
export function alertTarget(a: AlertRow): string | null {
    if (!a.resource_id)
        return null;
    switch (a.resource_type) {
        case 'scan': return `/simulations/${a.resource_id}`;
        case 'report': return `/reports/${a.resource_id}`;
        case 'agent': return `/assets/agents?agent=${a.resource_id}`;
        case 'asset': return `/assets/${a.resource_id}`;
        case 'organization': return `/organizations/${a.resource_id}`;
        case 'simulation': return `/simulations/legacy/${a.resource_id}`;
        case 'finding': return '/findings';
        case 'execution': return '/simulations';
        case 'user': return '/settings/users';
        default: return null;
    }
}
function AlertButton({ a, onOpen }: {
    a: AlertRow;
    onOpen: (a: AlertRow) => void;
}) {
    useLocale();
    return (<button type="button" className="alert-item__btn" onClick={() => onOpen(a)}>
      <span className={`alert-item__dot alert-item__dot--${a.severity}`} aria-hidden="true"/>
      <span className="alert-item__text"><strong>{formatUI(notificationTitle(a.kind, a.title))}</strong>{formatUI(a.resource_id && <span className="muted small">{formatUI(a.resource_label ? `${a.resource_label} · ` : '')}{formatUI(a.resource_id.slice(-8))}</span>)}{formatUI(a.body && <span className="muted small">{formatUI(a.body)}</span>)}<span className="muted small">{formatUI(when(a.created_at))}</span></span>
      {formatUI(!a.read && <span className="sr-only">{textUI("m_1b9f384c1436")}</span>)}
    </button>);
}
// Satu elemen <li> per alert (daftar yang sah untuk pembaca layar); popover memakai ini langsung.
export function AlertItem({ a, onOpen }: {
    a: AlertRow;
    onOpen: (a: AlertRow) => void;
}) {
    useLocale();
    return (<li className={`alert-item ${a.read ? '' : 'is-unread'}`}>
      <AlertButton a={a} onOpen={onOpen}/>
    </li>);
}
export function NotificationsPage() {
    useLocale();
    const { data: me } = useMe();
    const [params, setParams] = useSearchParams();
    const organization = params.get('org') ?? '';
    const q = useAlerts(false, 100, true, organization);
    const act = useAlertAction();
    const navigate = useNavigate();
    const open = (a: AlertRow) => { if (!a.read)
        act.mutate({ id: a.id, action: 'read' }); const to = alertTarget(a); if (to)
        navigate(to); };
    const items = q.data?.items ?? [];
    return (<section className="page" aria-labelledby="notif-title">
      <div className="page__head">
        <div><div className="page__title-row"><h1 id="notif-title" className="page__title">{textUI("m_788011833a5a")}</h1>{formatUI(q.data && q.data.unread > 0 && <span className="badge">{richUI("m_3f6459da541f", [formatUI(q.data.unread)])}</span>)}</div>
          <p className="page__sub">{textUI("m_fbd378aacd3a")}</p></div>
        <button type="button" className="btn btn--secondary btn--auto" disabled={!q.data || q.data.unread === 0 || act.isPending} onClick={() => act.mutate({ action: 'read-all' })}>{textUI("m_d7592650e1c9")}</button>
      </div>
      {formatUI(can(me, 'organization.read') && <div className="toolbar"><OrganizationFilter value={organization} onChange={org => { const next = new URLSearchParams(params); if (org)
        next.set('org', org);
    else
        next.delete('org'); setParams(next); }}/></div>)}
      {formatUI(can(me, 'settings.manage') && <NotificationPolicy />)}
      {formatUI(act.isError && <p className="alert" role="alert">{formatUI(describeError(act.error).message)}</p>)}
      {formatUI(q.isPending ? <span className="skeleton"/> : q.isError ? (<div className="state state--error" role="alert"><h2>{textUI("m_645b14de56ec")}</h2><p>{formatUI(describeError(q.error).message)}</p><button type="button" className="btn btn--secondary btn--auto" onClick={() => q.refetch()}>{textUI("m_d8b8392e2c54")}</button></div>) : items.length === 0 ? <div className="state"><h2>{textUI("m_c3a40427d593")}</h2><p>{textUI("m_d9d1fc67392d")}</p></div> : (<ul className="list alert-list" aria-label={textUI("m_788011833a5a")}>
          {formatUI(items.map((a) => (<li key={a.id} className={`alert-row alert-item ${a.read ? '' : 'is-unread'}`}>
              <AlertButton a={a} onOpen={open}/>
              <button type="button" className="btn btn--secondary btn--sm btn--auto" aria-label={textUI("m_a17b39acc275", [notificationTitle(a.kind, a.title)])} onClick={() => act.mutate({ id: a.id, action: 'dismiss' })}>{textUI("m_48845bff334a")}</button>
            </li>)))}
        </ul>))}
    </section>);
}

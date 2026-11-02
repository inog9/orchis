import { useNavigate } from 'react-router-dom';
import { useAlertAction, useAlerts } from '../api/hooks';
import type { AlertRow } from '../api/types';
import { describeError } from '../components/ErrorNote';
import { when } from '../components/RunBadges';

// Ke mana sebuah alert membawa pengguna.
export function alertTarget(a: AlertRow): string | null {
  if (!a.resource_id) return null;
  switch (a.resource_type) {
    case 'simulation': return `/simulations/legacy/${a.resource_id}`;
    case 'finding': return '/findings';
    case 'execution': return '/simulations';
    case 'user': return '/settings/users';
    default: return null;
  }
}

function AlertButton({ a, onOpen }: { a: AlertRow; onOpen: (a: AlertRow) => void }) {
  return (
    <button type="button" className="alert-item__btn" onClick={() => onOpen(a)}>
      <span className={`alert-item__dot alert-item__dot--${a.severity}`} aria-hidden="true" />
      <span className="alert-item__text"><strong>{a.title}</strong>{a.body && <span className="muted small">{a.body}</span>}<span className="muted small">{when(a.created_at)}</span></span>
      {!a.read && <span className="sr-only">Unread</span>}
    </button>
  );
}

// Satu elemen <li> per alert (daftar yang sah untuk pembaca layar); popover memakai ini langsung.
export function AlertItem({ a, onOpen }: { a: AlertRow; onOpen: (a: AlertRow) => void }) {
  return (
    <li className={`alert-item ${a.read ? '' : 'is-unread'}`}>
      <AlertButton a={a} onOpen={onOpen} />
    </li>
  );
}

export function NotificationsPage() {
  const q = useAlerts(false, 100);
  const act = useAlertAction();
  const navigate = useNavigate();
  const open = (a: AlertRow) => { if (!a.read) act.mutate({ id: a.id, action: 'read' }); const to = alertTarget(a); if (to) navigate(to); };
  const items = q.data?.items ?? [];
  return (
    <section className="page" aria-labelledby="notif-title">
      <div className="page__head">
        <div><div className="page__title-row"><h1 id="notif-title" className="page__title">Notifications</h1>{q.data && q.data.unread > 0 && <span className="badge">{q.data.unread} unread</span>}</div>
          <p className="page__sub">Alerts about critical findings, failed scans and finished simulations.</p></div>
        <button type="button" className="btn btn--secondary btn--auto" disabled={!q.data || q.data.unread === 0 || act.isPending} onClick={() => act.mutate({ action: 'read-all' })}>Mark all as read</button>
      </div>
      {q.isPending ? <span className="skeleton" /> : q.isError ? (
        <div className="state state--error" role="alert"><h2>Couldn’t load notifications</h2><p>{describeError(q.error).message}</p><button type="button" className="btn btn--secondary btn--auto" onClick={() => q.refetch()}>Try again</button></div>
      ) : items.length === 0 ? <div className="state"><h2>You’re all caught up</h2><p>Alerts show up here when something needs your attention.</p></div> : (
        <ul className="list alert-list" aria-label="Notifications">
          {items.map((a) => (
            <li key={a.id} className={`alert-row alert-item ${a.read ? '' : 'is-unread'}`}>
              <AlertButton a={a} onOpen={open} />
              <button type="button" className="btn btn--secondary btn--sm btn--auto" aria-label={`Dismiss ${a.title}`} onClick={() => act.mutate({ id: a.id, action: 'dismiss' })}>Dismiss</button>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

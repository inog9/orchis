import { navigationFor } from './navigation';
import { PageAccess, PageGuide } from './PageGuide';
import { Bell, ChevronUp, LogOut, Plus, Search, Settings, Server, UserRound, Users } from 'lucide-react';
import { Suspense, useEffect, useRef, useState, type ReactNode } from 'react';
import { Link, Outlet, useNavigate, useLocation } from 'react-router-dom';
import { useAlertAction, useAlerts, useLogout, useMe, useSettings, useSimulations } from '../api/hooks';
import { AlertItem, alertTarget } from '../pages/NotificationsPage';
import { CommandPalette, useCommandPalette } from './CommandPalette';
import { can } from '../api/types';
import { DemoBanner } from './DemoBanner';
import { Wordmark } from './Wordmark';

export function NavDock() {
  const {data:me}=useMe();
  const [tip, setTip] = useState<string | null>(null);
  const items = navigationFor(me);
  const {pathname}=useLocation();
  const active=(n:typeof items[number]) => n.to==='/'?pathname==='/':pathname===n.to||pathname.startsWith(n.to+'/')||(n.id==='actions'&&pathname.startsWith('/atomic'))||(n.id==='scenarios'&&pathname.startsWith('/chains'))||(n.id==='organizations'&&pathname.startsWith('/ai/jobs/'));
  return <nav className="dock dock--nav" aria-label="Primary" onKeyDown={e => { if (e.key === 'Escape') setTip(null); }}>
    <span className="nav-brand">WORKSPACE</span>
    {['Start','Test','Results','Manage'].map(group => <div className="nav-group" key={group}>
      <span className="nav-group__title">{group}</span>
      {items.filter(n => n.group === group).map(n => { const Icon=n.icon; return <div className="nav-entry" key={n.id} onMouseEnter={()=>setTip(n.id)} onMouseLeave={()=>setTip(null)}>
        <Link to={n.to} className={`dock__item ${active(n)?'is-active':''}`} aria-current={active(n)?'page':undefined} aria-label={n.label} aria-describedby={tip===n.id?`nav-tip-${n.id}`:undefined} onFocus={()=>setTip(n.id)} onBlur={()=>setTip(null)}><Icon size={17} aria-hidden="true"/><span>{n.label}</span></Link>
        {tip===n.id&&<span id={`nav-tip-${n.id}`} role="tooltip" className="nav-tip"><strong>{n.label}</strong>{n.hint}</span>}
      </div>;})}
    </div>)}
  </nav>;
}

function useDismiss(open: boolean, close: () => void) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && close();
    const onClick = (e: MouseEvent) => ref.current && !ref.current.contains(e.target as Node) && close();
    document.addEventListener('keydown', onKey);
    document.addEventListener('mousedown', onClick);
    return () => {
      document.removeEventListener('keydown', onKey);
      document.removeEventListener('mousedown', onClick);
    };
  }, [open, close]);
  return ref;
}

function WorkspaceMenu() {
  const { data: me } = useMe();
  const logout = useLogout();
  const navigate = useNavigate();
  const settings = useSettings();
  const [open, setOpen] = useState(false);
  const ref = useDismiss(open, () => setOpen(false));
  const go = (to: string) => { setOpen(false); navigate(to); };
  const initials = (me?.display_name ?? '?')
    .split(/\s+/)
    .map((p) => p[0])
    .slice(0, 2)
    .join('')
    .toUpperCase();
  return (
    <div className="ws" ref={ref}>
      <button type="button" className="ws__button" aria-haspopup="menu" aria-expanded={open} onClick={() => setOpen((o) => !o)}>
        <span>{settings.data?.platform_name ?? 'Platform default'}</span>
        <span className="ws__avatar" aria-hidden="true">
          {initials}
        </span>
      </button>
      {open && (
        <div className="menu" role="menu">
          <div className="menu__who">
            <strong>{me?.display_name}</strong>
            <span>{me?.email}</span>
            <span className="menu__role">{me?.role}</span>
          </div>
          <p className="menu__group">WORKSPACE</p>
          <button role="menuitem" className="menu__item" onClick={() => go('/assets')}><Server size={15} /> Assets &amp; groups</button>
          {can(me, 'people.read') && <button role="menuitem" className="menu__item" onClick={() => go('/people')}><Users size={15} /> People &amp; teams</button>}
          <button role="menuitem" className="menu__item" onClick={() => go('/settings')}><Settings size={15} /> Settings</button>
          <p className="menu__group">ACCOUNT</p>
          <button role="menuitem" className="menu__item" onClick={() => go('/profile')}><UserRound size={15} /> Profile</button>
          <button
            role="menuitem"
            className="menu__item"
            onClick={() => logout.mutate(undefined, { onSuccess: () => navigate('/login', { replace: true }) })}
          >
            <LogOut size={15} /> Sign out
          </button>
        </div>
      )}
    </div>
  );
}

function Notifications() {
  const { data: me } = useMe();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const ref = useDismiss(open, () => setOpen(false));
  const q = useAlerts(false, 8, can(me,'alert.read'));
  const act = useAlertAction();
  if (!can(me, 'alert.read')) return null;
  const unread = q.data?.unread ?? 0;
  return (
    <div className="notif" ref={ref}>
      <button type="button" className="dock__icon" aria-haspopup="dialog" aria-expanded={open} aria-label={unread > 0 ? `Notifications, ${unread} unread` : 'Notifications'} onClick={() => setOpen((o) => !o)}>
        <Bell size={15} />{unread > 0 && <span className="dock__badge" aria-hidden="true">{unread > 9 ? '9+' : unread}</span>}
      </button>
      {open && (
        <div className="menu menu--up notif__pop" role="dialog" aria-label="Notifications">
          <div className="notif__head"><strong>Notifications</strong>
            <button type="button" className="link-quiet" disabled={unread === 0} onClick={() => act.mutate({ action: 'read-all' })}>Mark all as read</button></div>
          {q.isPending ? <p className="muted pad">Loading…</p> : (q.data?.items.length ?? 0) === 0 ? <p className="muted pad">You’re all caught up.</p> : (
            <ul className="notif__list">
              {q.data!.items.map((a) => <AlertItem key={a.id} a={a} onOpen={(x) => { if (!x.read) act.mutate({ id: x.id, action: 'read' }); setOpen(false); const to = alertTarget(x); if (to) navigate(to); }} />)}
            </ul>
          )}
          <button type="button" className="btn btn--secondary btn--sm notif__all" onClick={() => { setOpen(false); navigate('/notifications'); }}>See all notifications</button>
        </div>
      )}
    </div>
  );
}

function ActionDock({ openPalette }: { openPalette: () => void }) {
  const { data: me } = useMe();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const ref = useDismiss(open, () => setOpen(false));
  const liveQuery = useSimulations('running', can(me,'simulation.read'));
  const live = { ...liveQuery, data: liveQuery.data?.filter(s => !s.is_demo) };
  const canCreate=['organization.write','asset.write','scenario.write','people.write'].some(perm=>can(me,perm));
  const n = can(me, 'simulation.read') ? (live.data?.length ?? 0) : 0;
  const item = (label: string, to: string, perm: string) => can(me, perm) ? (
    <button role="menuitem" className="menu__item" disabled={!can(me, perm)} onClick={() => { setOpen(false); navigate(to); }}><Plus size={15} /> {label}</button>
  ) : null;
  return (
    <div className="dock dock--action" ref={ref} role="region" aria-label="Quick actions">
      <button type="button" className="dock__idle" onClick={() => navigate('/simulations')} aria-label={n > 0 ? `${n} live ${n === 1 ? 'run' : 'runs'}` : 'No live runs'}>
        <span className="dock__idle-title">{n > 0 ? `${n} live ${n === 1 ? 'run' : 'runs'}` : 'No live runs'}</span>
        <span className="dock__idle-sub">{n > 0 ? live.data![0]!.name : 'Nothing is running'}</span>
      </button>
      <span className="dock__sep" aria-hidden="true" />
      {canCreate&&<div className="dock__new-wrap">
        <button type="button" className="btn-new" aria-haspopup="menu" aria-expanded={open} onClick={() => setOpen((o) => !o)}>
          <Plus size={14} /> New <ChevronUp size={12} />
        </button>
        {open && (
          <div className="menu menu--up" role="menu">
            {item('Create organisation', '/organizations?new=1', 'organization.write')}<p className="menu__group">ASSETS</p>
            {item('Add asset', '/assets?new=1', 'asset.write')}
            <p className="menu__group">CHAINS</p>
            {item('New scenario', '/scenarios?new=1', 'scenario.write')}
            <p className="menu__group">PEOPLE</p>
            {item('Add person', '/people?new=1', 'people.write')}
          </div>
        )}
      </div>}
      <button type="button" className="search" onClick={openPalette} aria-label="Search or jump to (Ctrl K)">
        <Search size={13} /> Search or jump to… <kbd>⌘K</kbd>
      </button>
      <Notifications />
    </div>
  );
}

export function Shell({ children }: { children?: ReactNode }) {
  const {pathname}=useLocation();
  useEffect(()=>{window.scrollTo(0,0);},[pathname]);
  const [paletteOpen, setPalette] = useCommandPalette();
  return (
    <div className="shell">
      <a className="skip-link" href="#main">Skip to content</a>
      <header className="topbar">
        <Wordmark />
        <ActionDock openPalette={() => setPalette(true)} />
        <WorkspaceMenu />
      </header>
      <NavDock />
      <main className="content" id="main" tabIndex={-1}><DemoBanner /><PageAccess><PageGuide /><Suspense fallback={<div className="page" role="status" aria-label="Loading page"><span className="skeleton" /></div>}>{children ?? <Outlet />}</Suspense></PageAccess></main>
      {paletteOpen && <CommandPalette onClose={() => setPalette(false)} />}
    </div>
  );
}

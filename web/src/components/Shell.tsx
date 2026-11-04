import { LanguagePreference } from '../i18n/LanguagePreference';
import { t as textUI, tr as richUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { useScans } from '../api/scans';
import { advancedEnabled, navigationFor } from './navigation';
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
    useLocale();
    const { data: me } = useMe();
    const [tip, setTip] = useState<string | null>(null);
    const items = navigationFor(me);
    const { pathname } = useLocation();
    const active = (n: typeof items[number]) => n.to === '/' ? pathname === '/' : pathname === n.to || pathname.startsWith(n.to + '/') || (n.id === 'organizations' && pathname.startsWith('/ai/jobs/'));
    return <nav className="dock dock--nav" aria-label={textUI("m_efe10c80ec8a")} onKeyDown={e => {
            if (e.key === 'Escape')
                setTip(null);
        }}>
    <span className="nav-brand">{textUI("m_5493ff8e9397")}</span>
    {formatUI([textUI("m_87bb59ba2f92"), textUI("m_5a23444828db")].filter(group => items.some(n => n.group === group)).map(group => <div className="nav-group" key={group}>
      <span className="nav-group__title">{formatUI(group)}</span>
      {formatUI(items.filter(n => n.group === group).map(n => {
                const Icon = n.icon;
                return <div className="nav-entry" key={n.id} onMouseEnter={() => setTip(n.id)} onMouseLeave={() => setTip(null)}>
        <Link to={n.to} className={`dock__item ${active(n) ? 'is-active' : ''}`} aria-current={active(n) ? 'page' : undefined} aria-label={n.label} aria-describedby={tip === n.id ? `nav-tip-${n.id}` : undefined} onFocus={() => setTip(n.id)} onBlur={() => setTip(null)}><Icon size={17} aria-hidden="true"/><span>{formatUI(n.label)}</span></Link>
        {formatUI(tip === n.id && <span id={`nav-tip-${n.id}`} role="tooltip" className="nav-tip"><strong>{formatUI(n.label)}</strong>{formatUI(n.hint)}</span>)}
      </div>;
            }))}
    </div>))}
  </nav>;
}
function useDismiss(open: boolean, close: () => void) {
    const ref = useRef<HTMLDivElement>(null);
    useEffect(() => {
        if (!open)
            return;
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
    useLocale();
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
    return (<><LanguagePreference /><div className="ws" ref={ref}>
      <button type="button" className="ws__button" aria-haspopup="menu" aria-expanded={open} onClick={() => setOpen((o) => !o)}>
        <span>{formatUI(settings.data?.platform_name ?? textUI("m_7d2364431c1a"))}</span>
        <span className="ws__avatar" aria-hidden="true">
          {formatUI(initials)}
        </span>
      </button>
      {formatUI(open && (<div className="menu" role="menu">
          <div className="menu__who">
            <strong>{formatUI(me?.display_name)}</strong>
            <span>{formatUI(me?.email)}</span>
            <span className="menu__role">{formatUI(me?.role)}</span>
          </div>
          <p className="menu__group">{textUI("m_5493ff8e9397")}</p>
          {formatUI(can(me, 'asset.read') && <button role="menuitem" className="menu__item" onClick={() => go('/assets')}>{richUI("m_fca1d1970b2b", [<Server size={15}/>])}</button>)}
          {formatUI(can(me, 'people.read') && <button role="menuitem" className="menu__item" onClick={() => go('/people')}>{richUI("m_2c1ba97edb9b", [<Users size={15}/>])}</button>)}
          {formatUI(can(me, 'settings.manage') && <button role="menuitem" className="menu__item" onClick={() => go('/settings')}>{richUI("m_de2cebc3e2ae", [<Settings size={15}/>])}</button>)}
          <p className="menu__group">{textUI("m_2d75b76b2dcf")}</p>
          <button role="menuitem" className="menu__item" onClick={() => go('/profile')}>{richUI("m_a0c915185839", [<UserRound size={15}/>])}</button>
          <button role="menuitem" className="menu__item" onClick={() => logout.mutate(undefined, { onSuccess: () => navigate('/login', { replace: true }) })}>{richUI("m_f9248a099a5c", [<LogOut size={15}/>])}</button>
        </div>))}
    </div></>);
}
function Notifications() {
    useLocale();
    const { data: me } = useMe();
    const navigate = useNavigate();
    const [open, setOpen] = useState(false);
    const ref = useDismiss(open, () => setOpen(false));
    const q = useAlerts(false, 8, (can(me, 'alert.read') || can(me, 'finding.read') || can(me, 'report.read')));
    const act = useAlertAction();
    if (!(can(me, 'alert.read') || can(me, 'finding.read') || can(me, 'report.read')))
        return null;
    const unread = q.data?.unread ?? 0;
    return (<div className="notif" ref={ref}>
      <button type="button" className="dock__icon" aria-haspopup="dialog" aria-expanded={open} aria-label={unread > 0 ? textUI("m_b45f7db6de35", [unread]) : textUI("m_788011833a5a")} onClick={() => setOpen((o) => !o)}>
        <Bell size={15}/>{formatUI(unread > 0 && <span className="dock__badge" aria-hidden="true">{formatUI(unread > 9 ? '9+' : unread)}</span>)}
      </button>
      {formatUI(open && (<div className="menu menu--up notif__pop" role="dialog" aria-label={textUI("m_788011833a5a")}>
          <div className="notif__head"><strong>{textUI("m_788011833a5a")}</strong>
            <button type="button" className="link-quiet" disabled={unread === 0} onClick={() => act.mutate({ action: 'read-all' })}>{textUI("m_d7592650e1c9")}</button></div>
          {formatUI(q.isPending ? <p className="muted pad">{textUI("m_ba3bbbe10d8b")}</p> : (q.data?.items.length ?? 0) === 0 ? <p className="muted pad">{textUI("m_47aa1e80a7c7")}</p> : (<ul className="notif__list">
              {formatUI(q.data!.items.map((a) => <AlertItem key={a.id} a={a} onOpen={(x) => {
                        if (!x.read)
                            act.mutate({ id: x.id, action: 'read' });
                        setOpen(false);
                        const to = alertTarget(x);
                        if (to)
                            navigate(to);
                    }}/>))}
            </ul>))}
          <button type="button" className="btn btn--secondary btn--sm notif__all" onClick={() => { setOpen(false); navigate('/notifications'); }}>{textUI("m_00f66a6acc10")}</button>
        </div>))}
    </div>);
}
function ActionDock({ openPalette }: {
    openPalette: () => void;
}) {
    useLocale();
    const { data: me } = useMe();
    const navigate = useNavigate();
    const [open, setOpen] = useState(false);
    const ref = useDismiss(open, () => setOpen(false));
    const liveQuery = useSimulations('running', can(me, 'simulation.read'));
    const live = { ...liveQuery, data: liveQuery.data?.filter(s => !s.is_demo) };
    const canCreate = ['organization.write', 'asset.write', 'simulation.run'].some(perm => can(me, perm));
    const scans = useScans('', '', can(me, 'simulation.read') && can(me, 'assessment.read') && can(me, 'asset.read') && can(me, 'organization.read'));
    const runningScans = scans.data?.items.filter(s => s.status === 'running') ?? [];
    const n = (can(me, 'simulation.read') ? (live.data?.length ?? 0) : 0) + runningScans.length;
    const item = (label: string, to: string, perm: string) => can(me, perm) ? (<button role="menuitem" className="menu__item" disabled={!can(me, perm)} onClick={() => { setOpen(false); navigate(to); }}><Plus size={15}/> {formatUI(label)}</button>) : null;
    return (<div className="dock dock--action" ref={ref} role="region" aria-label={textUI("m_1810407f5ab7")}>
      {formatUI(can(me, 'simulation.read') && <button type="button" className="dock__idle" onClick={() => navigate('/simulations')} aria-label={n > 0 ? textUI("m_c77817bd05a8", [n, n === 1 ? textUI("m_acba25512100") : textUI("m_1f64fff08d78")]) : textUI("m_d9390a6f6519")}>
        <span className="dock__idle-title">{formatUI(n > 0 ? textUI("m_c77817bd05a8", [n, n === 1 ? textUI("m_acba25512100") : textUI("m_1f64fff08d78")]) : textUI("m_d9390a6f6519"))}</span>
        <span className="dock__idle-sub">{formatUI(n > 0 ? (runningScans[0]?.name ?? live.data?.[0]?.name ?? textUI("m_f4ccae29e1bb")) : textUI("m_38e67fb129a3"))}</span>
      </button>)}
      {formatUI(can(me, 'simulation.read') && <span className="dock__sep" aria-hidden="true"/>)}
      {formatUI(canCreate && <div className="dock__new-wrap">
        <button type="button" className="btn-new" aria-haspopup="menu" aria-expanded={open} onClick={() => setOpen((o) => !o)}>{richUI("m_a8a8dccb2649", [<Plus size={14}/>, <ChevronUp size={12}/>])}</button>
        {formatUI(open && (<div className="menu menu--up" role="menu">
            {formatUI(item(textUI("m_1c3b4bf6ffd9"), '/organizations?new=1', 'organization.write'))}<p className="menu__group">{textUI("m_f5476433111a")}</p>
            {formatUI(item(textUI("m_48f15767b06d"), '/assets?new=1', 'asset.write'))}
            {formatUI(item(textUI("m_de2b814aeb96"), '/simulations/new', 'simulation.run'))}
            {formatUI(advancedEnabled(me) && item(textUI("m_7b08c04b5ad8"), '/chains?new=1', 'scenario.write'))}
            <p className="menu__group">{textUI("m_0ab25a59ac13")}</p>
            {formatUI(item(textUI("m_66848b5361f4"), '/people?new=1', 'people.write'))}
          </div>))}
      </div>)}
      <button type="button" className="search" onClick={openPalette} aria-label={textUI("m_e48ebb022089")}>{richUI("m_bab439bb84cf", [<Search size={13}/>, <kbd>⌘K</kbd>])}</button>
      <Notifications />
    </div>);
}
export function Shell({ children }: {
    children?: ReactNode;
}) {
    useLocale();
    const { pathname } = useLocation();
    useEffect(() => { window.scrollTo(0, 0); }, [pathname]);
    const [paletteOpen, setPalette] = useCommandPalette();
    return (<div className="shell">
      <a className="skip-link" href="#main">{textUI("m_ac576a66d456")}</a>
      <header className="topbar">
        <Wordmark />
        <ActionDock openPalette={() => setPalette(true)}/>
        <WorkspaceMenu />
      </header>
      <NavDock />
      <main className="content" id="main" tabIndex={-1}><DemoBanner /><PageAccess><PageGuide /><Suspense fallback={<div className="page" role="status" aria-label={textUI("m_91bf928d3527")}><span className="skeleton"/></div>}>{formatUI(children ?? <Outlet />)}</Suspense></PageAccess></main>
      {formatUI(paletteOpen && <CommandPalette onClose={() => setPalette(false)}/>)}
    </div>);
}

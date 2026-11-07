import { t as textUI, useLocale, fv as formatUI } from "../i18n/runtime";
import './settings.css';
import { Suspense } from 'react';
import { NavLink, Outlet } from 'react-router-dom';
import { useMe } from '../api/hooks';
import { can } from '../api/types';
export function SettingsPage() {
    useLocale();
    const { data: me } = useMe();
    const tabs = [
        { to: '/settings', label: textUI("m_c78ffe195710"), end: true, show: true },
        { to: '/settings/users', label: textUI("m_6b0cc904d081"), show: can(me, 'user.manage') },
        { to: '/settings/roles', label: textUI("m_c25337055464"), show: can(me, 'user.manage') },
        { to: '/settings/integrations', label: textUI("m_090512d93fcc"), show: can(me, 'integration.read') },
        { to: '/settings/audit', label: textUI("m_e4d36f9a4e22"), show: can(me, 'audit.read') },
        { to: '/settings/tags', label: textUI("m_1331275bc537"), show: can(me, 'asset.read') },
    ].filter((t) => t.show);
    return (<section className="page settings" aria-labelledby="settings-title">
      <div className="page__head"><div><h1 id="settings-title" className="page__title">{textUI("m_74a883a037bc")}</h1><p className="page__sub">{textUI("m_ed4a86bd0509")}</p></div></div>
      <nav className="tabs" aria-label={textUI("m_e26d51d36781")}>
        {formatUI(tabs.map((t) => <NavLink key={t.to} to={t.to} end={t.end} className={({ isActive }) => `tab ${isActive ? 'is-active' : ''}`}>{formatUI(t.label)}</NavLink>))}
      </nav>
      <Suspense fallback={<span className="skeleton" role="status" aria-label={textUI("m_f458d7c761f0")}/>}><Outlet /></Suspense>
    </section>);
}

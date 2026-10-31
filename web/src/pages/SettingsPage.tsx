import {Suspense} from 'react';
import { NavLink, Outlet } from 'react-router-dom';
import { useMe } from '../api/hooks';
import { can } from '../api/types';

export function SettingsPage() {
  const { data: me } = useMe();
  const tabs = [
    { to: '/settings', label: 'Platform', end: true, show: true },
    { to: '/settings/users', label: 'Users', show: can(me, 'user.manage') },
    { to: '/settings/roles', label: 'Roles', show: can(me, 'user.manage') },
    { to: '/settings/integrations', label: 'Integrations', show: can(me, 'integration.read') },
    { to: '/settings/audit', label: 'Audit log', show: can(me, 'audit.read') },
    { to: '/settings/tags', label: 'Tags', show: can(me, 'asset.read') },
  ].filter((t) => t.show);
  return (
    <section className="page" aria-labelledby="settings-title">
      <div className="page__head"><div><h1 id="settings-title" className="page__title">Settings</h1><p className="page__sub">Platform parameters, people who can sign in, connected integrations and labels.</p></div></div>
      <nav className="tabs" aria-label="Settings sections">
        {tabs.map((t) => <NavLink key={t.to} to={t.to} end={t.end} className={({ isActive }) => `tab ${isActive ? 'is-active' : ''}`}>{t.label}</NavLink>)}
      </nav>
      <Suspense fallback={<span className="skeleton" role="status" aria-label="Loading settings section" />}><Outlet /></Suspense>
    </section>
  );
}

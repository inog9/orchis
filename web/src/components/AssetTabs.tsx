import { t as textUI, useLocale } from "../i18n/runtime";
import { NavLink } from 'react-router-dom';
export function AssetTabs() {
    useLocale();
    return (<nav className="tabs" aria-label={textUI("m_73ac41d4212a")}>
      <NavLink to="/assets" end className={({ isActive }) => `tab ${isActive ? 'is-active' : ''}`}>{textUI("m_fce7fec2a58c")}</NavLink>
      <NavLink to="/assets/groups" className={({ isActive }) => `tab ${isActive ? 'is-active' : ''}`}>{textUI("m_39dd12de3b02")}</NavLink>
      <NavLink to="/assets/agents" className={({ isActive }) => `tab ${isActive ? 'is-active' : ''}`}>{textUI("m_279b44d2ab4b")}</NavLink>
    </nav>);
}

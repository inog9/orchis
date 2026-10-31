import { NavLink } from 'react-router-dom';

export function AssetTabs() {
  return (
    <nav className="tabs" aria-label="Asset sections">
      <NavLink to="/assets" end className={({ isActive }) => `tab ${isActive ? 'is-active' : ''}`}>
        Endpoints
      </NavLink>
      <NavLink to="/assets/groups" className={({ isActive }) => `tab ${isActive ? 'is-active' : ''}`}>
        Asset groups
      </NavLink>
      <NavLink to="/assets/agents" className={({ isActive }) => `tab ${isActive ? 'is-active' : ''}`}>
        Agents
      </NavLink>
    </nav>
  );
}

import { ChevronRight } from 'lucide-react';
import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { useLocale, fv as formatUI } from '../../i18n/runtime';

export type Crumb = { label: string; to?: string };

export function PageHeader({ crumbs, title, subtitle, actions, titleId = 'page-title' }: { crumbs: Crumb[]; title: string; subtitle?: string; actions?: ReactNode; titleId?: string }) {
    useLocale();
    return (<header className="phead">
      <div className="phead__main">
        <nav className="crumbs" aria-label="Breadcrumb">
          {crumbs.map((c, i) => (<span key={i} className="crumbs__item">
              {i > 0 && <ChevronRight size={12} aria-hidden="true"/>}
              {c.to ? <Link to={c.to}>{formatUI(c.label)}</Link> : <span className="crumbs__current" aria-current="page">{formatUI(c.label)}</span>}
            </span>))}
        </nav>
        <h1 id={titleId} className="page__title">{formatUI(title)}</h1>
        {subtitle && <p className="page__sub">{formatUI(subtitle)}</p>}
      </div>
      {actions && <div className="phead__actions">{actions}</div>}
    </header>);
}

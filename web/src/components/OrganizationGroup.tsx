import { t as textUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { ChevronRight } from 'lucide-react';
import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';

// Satu organisasi dalam daftar global: judul, ringkasan, tautan ke organisasi, dan isi yang bisa dilipat.
export function OrganizationGroup({ id, name, summary, linkable, children }: {
    id: string | null;
    name: string | null;
    summary: ReactNode;
    linkable: boolean;
    children: ReactNode;
}) {
    useLocale();
    const title = id ? (name || textUI("m_6fcd5de74842")) : textUI("m_14d33bd014e6");
    return (<details className="org-group" open>
      <summary className="org-group__head">
        <ChevronRight size={15} aria-hidden="true" className="org-group__chev"/>
        <h2 className="org-group__title">{formatUI(title)}</h2>
        <span className="org-group__summary">{summary}</span>
        {formatUI(id && linkable && <Link className="org-group__open" to={`/organizations/${id}`} onClick={(e) => e.stopPropagation()}>{textUI("m_grp_open_org")}</Link>)}
      </summary>
      {children}
    </details>);
}

import { LayoutGrid, List, Search } from 'lucide-react';
import { t as textUI, useLocale, fv as formatUI } from '../../i18n/runtime';

export type SortKey = 'name' | 'assets' | 'last' | 'findings';
export type ViewMode = 'grid' | 'list';

export function OrgToolbar({ q, onQ, industries, industry, onIndustry, sort, onSort, view, onView, showFindingsSort }: {
    q: string; onQ: (v: string) => void;
    industries: string[]; industry: string; onIndustry: (v: string) => void;
    sort: SortKey; onSort: (v: SortKey) => void;
    view: ViewMode; onView: (v: ViewMode) => void;
    showFindingsSort: boolean;
}) {
    useLocale();
    return (<div className="otb" role="search">
      <label className="search-input otb__search">
        <Search size={15} aria-hidden="true"/>
        <input type="search" placeholder={textUI("m_orgs_search_ph")} aria-label={textUI("m_orgs_search_ph")} value={q} onChange={(e) => onQ(e.target.value)}/>
      </label>
      {industries.length > 8 && (<label className="filter">
        <span className="sr-only">{textUI("m_orgs_filter_industry")}</span>
        <select value={industry} aria-label={textUI("m_orgs_filter_industry")} onChange={(e) => onIndustry(e.target.value)}>
          <option value="">{textUI("m_orgs_all")}</option>
          {formatUI(industries.map((i) => <option key={i} value={i}>{formatUI(i.charAt(0).toUpperCase() + i.slice(1))}</option>))}
        </select>
      </label>)}
      {industries.length > 1 && industries.length <= 8 && (<div className="otb__chips" role="group" aria-label={textUI("m_orgs_filter_industry")}>
        <button type="button" className={`chip${industry === '' ? ' is-on' : ''}`} aria-pressed={industry === ''} onClick={() => onIndustry('')}>{textUI("m_orgs_all")}</button>
        {formatUI(industries.map((i) => <button key={i} type="button" className={`chip${industry === i ? ' is-on' : ''}`} aria-pressed={industry === i} onClick={() => onIndustry(industry === i ? '' : i)}>{formatUI(i.charAt(0).toUpperCase() + i.slice(1))}</button>))}
      </div>)}
      <label className="filter otb__sort">
        <span className="sr-only">{textUI("m_orgs_sort")}</span>
        <select value={sort} aria-label={textUI("m_orgs_sort")} onChange={(e) => onSort(e.target.value as SortKey)}>
          <option value="name">{textUI("m_orgs_sort_name")}</option>
          <option value="assets">{textUI("m_orgs_sort_assets")}</option>
          {showFindingsSort && <option value="last">{textUI("m_orgs_sort_last")}</option>}
          {showFindingsSort && <option value="findings">{textUI("m_orgs_sort_findings")}</option>}
        </select>
      </label>
      <div className="otb__view" role="group" aria-label={textUI("m_orgs_view_toggle")}>
        <button type="button" className={view === 'grid' ? 'is-on' : ''} aria-pressed={view === 'grid'} aria-label={textUI("m_orgs_view_grid")} onClick={() => onView('grid')}><LayoutGrid size={15} aria-hidden="true"/></button>
        <button type="button" className={view === 'list' ? 'is-on' : ''} aria-pressed={view === 'list'} aria-label={textUI("m_orgs_view_list")} onClick={() => onView('list')}><List size={15} aria-hidden="true"/></button>
      </div>
    </div>);
}

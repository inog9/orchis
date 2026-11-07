import { t as textUI, tr as richUI, useLocale, locale, fv as formatUI } from "../i18n/runtime";
import { useQuery } from '@tanstack/react-query';
import { api } from '../api/client';
import { ScanComparison, type Comparison } from '../components/ScanComparison';
import { OrganizationFilter } from '../components/OrganizationFilter';
import { can } from '../api/types';
import { ChevronRight, Search, X } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { useFindings, useHome, useMe } from '../api/hooks';
import { describeError } from '../components/ErrorNote';
import { SeverityBadge } from '../components/RunBadges';
import { StatCard } from '../components/orgs/StatCard';
import { FindingDrawer } from '../components/findings/FindingDrawer';
import { groupByOrganization } from '../lib/groupByOrganization';
import { relativeTime } from '../lib/relativeTime';
import { SEVERITIES, applySeverities, countBySeverity, groupByAsset, isNew, sortFindings, type FindingGroup, type SortKey, type ViewKey, applyDefenseFilters, defenseOf } from '../lib/findingsView';
import type { Finding, User } from '../api/types';
import './orgs.css';
import './findings.css';

const VIEW_KEY = 'orchis.findings.view';
const COLLAPSED_KEY = 'orchis.findings.collapsed';
function readStore<T>(key: string, fallback: T): T {
    try { const raw = localStorage.getItem(key); return raw ? JSON.parse(raw) as T : fallback; } catch { return fallback; }
}
function writeStore(key: string, value: unknown) { try { localStorage.setItem(key, JSON.stringify(value)); } catch { /* penyimpanan diblokir */ } }
function useDebounced<T>(value: T, ms: number) {
    const [v, setV] = useState(value);
    useEffect(() => { const t = setTimeout(() => setV(value), ms); return () => clearTimeout(t); }, [value, ms]);
    return v;
}
const triageLabel = (s: string) => ({ open: '', accepted_risk: textUI("flow_triage_accepted"), false_positive: textUI("flow_triage_false"), fixed_pending: textUI("flow_triage_fixed") } as Record<string, string>)[s] ?? '';
const SEV_LABEL = (s: Finding['severity']) => ({ critical: textUI("m_427dd2969bd1"), high: textUI("m_c4ebc6d4a583"), medium: textUI("m_8e588cd18774"), low: textUI("m_f793de205ead"), info: textUI("m_170322a32f3c") })[s];
const full = (iso: string) => { const d = new Date(iso); return Number.isNaN(d.getTime()) ? '' : d.toLocaleString(locale(), { dateStyle: 'medium', timeStyle: 'short' }); };

function SeverityStrip({ counts, selected, onToggle }: { counts: Record<string, number>; selected: ReadonlySet<string>; onToggle: (s: string) => void }) {
    useLocale();
    const total = SEVERITIES.reduce((n, s) => n + (counts[s] ?? 0), 0);
    return (<div className="fnd-strip">
      <div className="fnd-strip__bar" role="presentation">
        {total === 0 ? <span className="fnd-strip__seg fnd-strip__seg--empty"/> : SEVERITIES.filter((s) => counts[s]).map((s) => <span key={s} className={`fnd-strip__seg fnd-strip__seg--${s}`} style={{ flexGrow: counts[s] }}/>)}
      </div>
      <div className="fnd-strip__legend" role="group" aria-label={textUI("m_fnd_sev_dist")}>
        {SEVERITIES.map((s) => (<button key={s} type="button" className={`fnd-strip__item${selected.has(s) ? ' is-on' : ''}`} aria-pressed={selected.has(s)} onClick={() => onToggle(s)}>
          <i className={`fnd-dot fnd-dot--${s}`} aria-hidden="true"/><span>{SEV_LABEL(s)}</span><b className="mono">{formatUI(counts[s] ?? 0)}</b>
        </button>))}
      </div>
    </div>);
}

function Row({ f, me, onOpen }: { f: Finding; me: User | null | undefined; onOpen: (f: Finding, el: HTMLElement) => void }) {
    useLocale();
    return (<li className="fnd-row">
      <button type="button" className="fnd-row__main" onClick={(e) => onOpen(f, e.currentTarget)}>
        <SeverityBadge severity={f.severity}/>
        <span className="fnd-row__text">
          <strong>{formatUI(f.title)}{isNew(f) && <span className="fnd-new">{textUI("m_fnd_new")}</span>}{defenseOf(f)?.kev && <span className="fnd-def fnd-def--kev">{textUI("m_def_kev")}</span>}{defenseOf(f)?.exposure === 'exposed' && <span className="fnd-def fnd-def--exposed">{textUI("m_def_exp_exposed")}</span>}{f.triage_status && f.triage_status !== 'open' && <span className="fnd-def">{triageLabel(f.triage_status)}</span>}</strong>
          <code className="mono" title={f.value}>{f.value}</code>
        </span>
      </button>
      <span className="fnd-row__asset">{formatUI(f.asset_id && can(me, 'asset.read') ? <Link to={`/assets/${f.asset_id}`}>{formatUI(f.asset_name)}</Link> : f.asset_name || textUI("m_bedb7e61a067"))}</span>
      <span className="fnd-row__org">{formatUI(f.organization_name || '—')}</span>
      <span className="fnd-row__count mono" title={textUI("m_fnd_occurrences")}>×{formatUI(f.occurrence_count)}</span>
      <time className="fnd-row__time" dateTime={f.last_seen_at} title={full(f.last_seen_at)}>{relativeTime(f.last_seen_at) || '—'}</time>
    </li>);
}

function Group({ g, title, linkTo, collapsed, onToggle, children }: { g: FindingGroup; title: string; linkTo?: string; collapsed: boolean; onToggle: () => void; children: React.ReactNode }) {
    useLocale();
    const counts = countBySeverity(g.items);
    return (<section className="fnd-group" aria-label={title}>
      <header className="fnd-group__head">
        <button type="button" className="fnd-group__toggle" aria-expanded={!collapsed} onClick={onToggle}><ChevronRight size={15} aria-hidden="true" className={collapsed ? '' : 'is-open'}/><h2>{formatUI(title)}</h2></button>
        <span className="fnd-group__sum">{richUI("m_grp_n_findings", [formatUI(g.items.length)])}</span>
        <span className="fnd-group__sevs">{SEVERITIES.filter((s) => counts[s]).map((s) => <span key={s} className="fnd-group__sev"><i className={`fnd-dot fnd-dot--${s}`} aria-hidden="true"/><span className="sr-only">{SEV_LABEL(s)}</span>{formatUI(counts[s])}</span>)}</span>
        {formatUI(linkTo && <Link className="fnd-group__open" to={linkTo}>{textUI("m_grp_open_org")}</Link>)}
      </header>
      {!collapsed && children}
    </section>);
}

export function FindingsPage({ organizationID, organizationName }: {
    organizationID?: string;
    organizationName?: string;
} = {}) {
    useLocale();
    const [params, setParams] = useSearchParams();
    const assetID = params.get('asset_id') ?? '';
    const { data: me } = useMe();
    const organization = organizationID ?? params.get('org') ?? params.get('organization_id') ?? '';
    const inOrg = !!organizationID;
    const Heading = inOrg ? 'h2' : 'h1';
    const [text, setText] = useState('');
    const [type, setType] = useState('');
    const [selected, setSelected] = useState<ReadonlySet<string>>(new Set());
    const [kevOnly, setKevOnly] = useState(false);
    const [exposedOnly, setExposedOnly] = useState(false);
    const [actionable, setActionable] = useState(false);
    const [sort, setSort] = useState<SortKey>('severity');
    const [view, setViewState] = useState<ViewKey>(() => { const v = readStore<ViewKey>(VIEW_KEY, 'org'); return (inOrg || new URLSearchParams(window.location.search).get('org')) && v === 'org' ? 'flat' : v; });
    const [collapsed, setCollapsed] = useState<string[]>(() => readStore<string[]>(COLLAPSED_KEY, []));
    const [open, setOpen] = useState<Finding | null>(null);
    const q = useDebounced(text.trim(), 250);
    const severity = selected.size === 1 ? [...selected][0]! : '';
    const res = useFindings(q, type, severity, assetID, organization, actionable ? 'open' : '');
    const home = useHome(!inOrg && can(me, 'finding.read'));
    const loaded = res.data?.pages.flatMap((p) => p.items) ?? [];
    const items = useMemo(() => sortFindings(applyDefenseFilters(applySeverities(loaded, selected), { kev: kevOnly, exposed: exposedOnly }), sort), [loaded, selected, sort, kevOnly, exposedOnly]);
    const [comparisonMode, setComparisonMode] = useState(false);
    useEffect(() => setComparisonMode(false), [organization]);
    const effectiveView: ViewKey = (inOrg || organization) && view === 'org' ? 'flat' : view;
    const setView = (v: ViewKey) => { setViewState(v); writeStore(VIEW_KEY, v); };
    const toggleSev = (s: string) => setSelected((cur) => { const n = new Set(cur); if (n.has(s)) n.delete(s); else n.add(s); return n; });
    const toggleGroup = (key: string) => setCollapsed((cur) => { const n = cur.includes(key) ? cur.filter((k) => k !== key) : [...cur, key]; writeStore(COLLAPSED_KEY, n); return n; });
    const clearAll = () => { setText(''); setType(''); setSelected(new Set()); setKevOnly(false); setExposedOnly(false); setActionable(false); if (!inOrg && (organization || assetID)) { const next = new URLSearchParams(params); next.delete('org'); next.delete('organization_id'); next.delete('asset_id'); setParams(next); } };
    const setOrg = (value: string) => { const next = new URLSearchParams(params); next.delete('organization_id'); if (value) next.set('org', value); else next.delete('org'); setParams(next); };

    const reportList = useQuery({ queryKey: ['latest-scan-report', organization], enabled: !!organization && can(me, 'report.read'), queryFn: () => api.get<{ items: { id: string }[] }>(`/api/v1/reports?organization_id=${encodeURIComponent(organization)}&scan_only=true`), refetchInterval: 5000 });
    const latestID = reportList.data?.items[0]?.id;
    const comparison = useQuery({ queryKey: ['scan-comparison-report', latestID], enabled: !!latestID && comparisonMode, queryFn: () => api.get<{ id: string; created_at: string; snapshot: { comparison_details?: Comparison } }>(`/api/v1/reports/${latestID}`) });

    const typeLabel = (v: string) => ({ open_port: textUI("m_5c9e0324afba"), service: textUI("m_d677190e0a99"), vulnerability: textUI("m_a123dd728a7a") })[v] ?? v;
    const chips: { key: string; label: string; remove: () => void }[] = [];
    SEVERITIES.filter((s) => selected.has(s)).forEach((s) => chips.push({ key: `s-${s}`, label: textUI("m_fnd_chip_sev", [SEV_LABEL(s)]), remove: () => toggleSev(s) }));
    if (kevOnly) chips.push({ key: 'kev', label: textUI("m_def_chip_kev"), remove: () => setKevOnly(false) });
    if (exposedOnly) chips.push({ key: 'exp', label: textUI("m_def_chip_exposed"), remove: () => setExposedOnly(false) });
    if (type) chips.push({ key: 't', label: textUI("m_fnd_chip_type", [typeLabel(type)]), remove: () => setType('') });
    if (q) chips.push({ key: 'q', label: textUI("m_fnd_chip_q", [q]), remove: () => setText('') });
    if (assetID) chips.push({ key: 'a', label: textUI("m_fnd_chip_asset", [loaded[0]?.asset_name || textUI("m_a04e2c43801b")]), remove: () => { const next = new URLSearchParams(params); next.delete('asset_id'); setParams(next); } });
    if (!inOrg && organization) chips.push({ key: 'o', label: textUI("m_fnd_chip_org", [loaded[0]?.organization_name || textUI("m_6fcd5de74842")]), remove: () => setOrg('') });
    const filtered = chips.length > 0;

    const stats = home.data?.findings;
    const sevCounts = stats?.by_severity ?? {};
    const renderRows = (list: Finding[]) => (<ul className="fnd-list" aria-label={textUI("m_e171c2ff25b5")}>{list.map((f) => <Row key={f.id} f={f} me={me} onOpen={(item) => setOpen(item)}/>)}</ul>);
    const listBody = effectiveView === 'org' ? groupByOrganization(items).map((g) => (
        <Group key={`o-${g.key}`} g={g} title={g.id ? (g.name || textUI("m_6fcd5de74842")) : textUI("m_14d33bd014e6")} linkTo={g.id && can(me, 'organization.read') ? `/organizations/${g.id}` : undefined} collapsed={collapsed.includes(`o-${g.key}`)} onToggle={() => toggleGroup(`o-${g.key}`)}>{renderRows(g.items)}</Group>))
        : effectiveView === 'asset' ? groupByAsset(items).map((g) => (
        <Group key={`a-${g.key}`} g={g} title={g.id ? (g.name || textUI("m_a04e2c43801b")) : textUI("m_bedb7e61a067")} linkTo={g.id && can(me, 'asset.read') ? `/assets/${g.id}` : undefined} collapsed={collapsed.includes(`a-${g.key}`)} onToggle={() => toggleGroup(`a-${g.key}`)}>{renderRows(g.items)}</Group>))
        : renderRows(items);

    return (<section className="page fnd" aria-labelledby="findings-title">
      <div className="page__head"><div>
        <div className="page__title-row"><Heading id="findings-title" className="page__title">{textUI("m_e171c2ff25b5")}</Heading></div>
        <p className="page__sub">{textUI("m_215192db3223")}</p>
      </div></div>

      {!inOrg && can(me, 'finding.read') && (<div className="fnd-stats">
        <StatCard label={textUI("m_fnd_total")} value={stats?.total} loading={home.isPending}><span className="fnd-stat__sub">{textUI("m_fnd_total_sub", [stats?.vulnerabilities ?? 0, stats?.open_ports ?? 0])}</span></StatCard>
        <button type="button" className="fnd-stat-btn" aria-pressed={selected.has('critical') && selected.has('high')} onClick={() => setSelected(selected.has('critical') && selected.has('high') ? new Set() : new Set(['critical', 'high']))}>
          <StatCard label={textUI("m_fnd_crit_high")} value={stats ? (stats.by_severity.critical ?? 0) + (stats.by_severity.high ?? 0) : undefined} loading={home.isPending}><span className="fnd-stat__sub">{textUI("m_fnd_crit_high_sub", [stats?.by_severity.critical ?? 0, stats?.by_severity.high ?? 0])}</span></StatCard>
        </button>
        <StatCard label={textUI("m_fnd_new7")} value={stats?.new_7d} loading={home.isPending}/>
        <StatCard label={textUI("m_fnd_vuln_assets")} value={stats?.vulnerable_assets} loading={home.isPending}>{home.data?.assets && <span className="fnd-stat__sub">{textUI("m_fnd_vuln_assets_sub", [home.data.assets.total])}</span>}</StatCard>
        <div className="stat fnd-stat--wide"><span className="stat__label">{textUI("m_fnd_sev_dist")}</span>{home.isPending ? <span className="skeleton stat__skeleton" aria-hidden="true"/> : <SeverityStrip counts={sevCounts} selected={selected} onToggle={toggleSev}/>}</div>
      </div>)}

      <div className="fnd-toolbar" role="search">
        {formatUI(can(me, 'organization.read') && <OrganizationFilter value={organization} locked={inOrg} name={organizationName} onChange={setOrg}/>)}
        <label className="search-input fnd-search"><Search size={15} aria-hidden="true"/><input type="search" placeholder={textUI("m_fc3f375fe446")} aria-label={textUI("m_fc3f375fe446")} value={text} onChange={(e) => setText(e.target.value)}/></label>
        <label className="filter"><span className="sr-only">{textUI("m_51035b5b67dc")}</span><select aria-label={textUI("m_51035b5b67dc")} value={type} onChange={(e) => setType(e.target.value)}>
          <option value="">{textUI("m_f10988e79e8d")}</option><option value="open_port">{textUI("m_5c9e0324afba")}</option><option value="service">{textUI("m_d677190e0a99")}</option><option value="vulnerability">{textUI("m_a123dd728a7a")}</option></select></label>
        <label className="filter"><span className="sr-only">{textUI("m_fnd_sort")}</span><select aria-label={textUI("m_fnd_sort")} value={sort} onChange={(e) => setSort(e.target.value as SortKey)}>
          <option value="severity">{textUI("m_fnd_sort_sev")}</option><option value="recent">{textUI("m_fnd_sort_recent")}</option><option value="frequent">{textUI("m_fnd_sort_freq")}</option></select></label>
        <div className="fnd-seg" role="group" aria-label={textUI("m_fnd_view")}>
          {!inOrg && !organization && <button type="button" aria-pressed={effectiveView === 'org'} onClick={() => setView('org')}>{textUI("m_fnd_view_org")}</button>}
          <button type="button" aria-pressed={effectiveView === 'asset'} onClick={() => setView('asset')}>{textUI("m_fnd_view_asset_g")}</button>
          <button type="button" aria-pressed={effectiveView === 'flat'} onClick={() => setView('flat')}>{textUI("m_fnd_view_flat")}</button>
        </div>
      </div>
      <div className="fnd-sevchips" role="group" aria-label={textUI("m_d44b62308969")}>
        {SEVERITIES.map((s) => <button key={s} type="button" className={`fnd-chip fnd-chip--${s}${selected.has(s) ? ' is-on' : ''}`} aria-pressed={selected.has(s)} onClick={() => toggleSev(s)}><i className={`fnd-dot fnd-dot--${s}`} aria-hidden="true"/>{SEV_LABEL(s)}</button>)}
      </div>
      <div className="fnd-sevchips" role="group" aria-label={textUI("m_def_filters")}>
        <button type="button" className={`fnd-chip${kevOnly ? ' is-on' : ''}`} aria-pressed={kevOnly} onClick={() => setKevOnly((v) => !v)}>{textUI("m_def_f_kev")}</button>
        <button type="button" className={`fnd-chip${exposedOnly ? ' is-on' : ''}`} aria-pressed={exposedOnly} onClick={() => setExposedOnly((v) => !v)}>{textUI("m_def_f_exposed")}</button>
        {formatUI(can(me, 'finding.write') && <button type="button" className={`fnd-chip${actionable ? ' is-on' : ''}`} aria-pressed={actionable} onClick={() => setActionable((v) => !v)}>{textUI("flow_fnd_actionable")}</button>)}
      </div>
      {filtered && (<div className="fnd-active" aria-label={textUI("m_fnd_active")}>
        <span className="fnd-active__label">{textUI("m_fnd_active")}</span>
        {chips.map((c) => <span key={c.key} className="fnd-tag">{formatUI(c.label)}<button type="button" aria-label={textUI("m_fnd_remove", [c.label])} onClick={c.remove}><X size={12} aria-hidden="true"/></button></span>)}
        <button type="button" className="fnd-clear" onClick={clearAll}>{textUI("m_fnd_clear")}</button>
      </div>)}

      {!!organization && can(me, 'report.read') && (<div className="fnd-compare"><label className="select">{richUI("m_cff8ed38d138", [<select aria-label={textUI("m_7e4ef287d309")} value={comparisonMode ? 'comparison' : 'all'} onChange={e => setComparisonMode(e.target.value === 'comparison')}><option value="all">{textUI("m_417102d18176")}</option><option value="comparison">{textUI("m_0e4edebd2891")}</option></select>])}</label>
        {comparisonMode && (reportList.error || comparison.error ? <p role="alert">{formatUI(describeError(reportList.error || comparison.error).message)}</p> : reportList.isPending || !!latestID && comparison.isPending ? <p role="status">{textUI("m_71bfd8846532")}</p> : !latestID ? <p>{textUI("m_5d13849e66cb")}</p> : <><p>{richUI("m_eb210a614bb5", [formatUI(new Date(comparison.data!.created_at).toLocaleString(locale())), <Link to={`/reports/${latestID}`}>{textUI("m_9f2bda9a060f")}</Link>])}</p><ScanComparison comparison={comparison.data?.snapshot.comparison_details} search={q} severity={severity} type={type} assetID={assetID}/></>)}
      </div>)}

      {!comparisonMode && (res.isPending ? (<ul className="fnd-list" aria-label={textUI("m_3b06d3881b0f")} aria-busy="true">{[0, 1, 2, 3, 4].map((i) => <li key={i} className="fnd-row fnd-row--skeleton"><span className="skeleton"/></li>)}</ul>)
        : res.isError ? (<div className="state state--error" role="alert"><h2>{textUI("m_8891e771bd5c")}</h2><p>{richUI("m_19636dc199da", [formatUI(describeError(res.error).message)])}</p><button type="button" className="btn btn--secondary btn--auto" onClick={() => res.refetch()}>{textUI("m_d8b8392e2c54")}</button></div>)
        : items.length === 0 ? (<div className="state" role="status"><h2>{formatUI(filtered ? textUI("m_991467873550") : textUI("m_3076f1ee8a92"))}</h2><p>{formatUI(filtered ? textUI("m_fff0463f16e2") : can(me, 'organization.read') ? textUI("m_af431b0a42ac") : textUI("m_dd629c0d9e7e"))}</p>
            {filtered ? <button type="button" className="btn btn--secondary btn--auto" onClick={clearAll}>{textUI("m_fnd_clear_filters")}</button> : can(me, 'organization.read') && <Link className="btn btn--secondary btn--auto" to="/organizations">{textUI("m_d78c44ecc1be")}</Link>}</div>)
        : (<>
          <p className="fnd-count" aria-live="polite">{textUI("m_fnd_shown", [items.length])}</p>
          {listBody}
          {res.hasNextPage && <div className="more"><button type="button" className="btn btn--secondary btn--auto" onClick={() => res.fetchNextPage()} disabled={res.isFetchingNextPage}>{formatUI(res.isFetchingNextPage ? textUI("m_ba3bbbe10d8b") : textUI("m_ac8991ef0101"))}</button></div>}
        </>))}
      {open && <FindingDrawer finding={open} canAsset={can(me, 'asset.read')} onClose={() => setOpen(null)}/>}
    </section>);
}

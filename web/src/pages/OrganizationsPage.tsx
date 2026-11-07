import { t as textUI, tr as richUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { DeleteOrganization } from '../components/DeleteOrganization';
import { ScanControls } from '../components/ScanControls';
import { ReportsPage } from './ReportsPage';
import { ScanList } from './ScansPage';
import { advancedEnabled } from '../components/navigation';
import { AssetsPage } from './AssetsPage';
import { FindingsPage } from './FindingsPage';
import { OrganizationEndpoints } from '../components/OrganizationEndpoints';
import { AssessmentReadiness } from '../components/AssessmentReadiness';
import { useEffect, useState } from 'react';
import { Plus } from 'lucide-react';
import './orgs.css';
import './orgd.css';
import { EmptyState } from '../components/orgs/EmptyState';
import { OrgCard, OrgCardSkeleton, SeverityBar } from '../components/orgs/OrgCard';
import { OrgToolbar, type SortKey, type ViewMode } from '../components/orgs/OrgToolbar';
import { PageHeader } from '../components/orgs/PageHeader';
import { Pagination } from '../components/orgs/Pagination';
import { StatCard } from '../components/orgs/StatCard';
import { OrgOverviewStats } from '../components/orgs/OrgOverviewStats';
import { totalFindings, useAllOrganizations, useOrgReports, useStoredChoice } from '../components/orgs/useOrgData';
import { relativeTime } from '../lib/relativeTime';
import { Link, useParams, useSearchParams } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/client';
import { useAgents, useHome, useMe } from '../api/hooks';
import { can, type Asset } from '../api/types';
import { describeError } from '../components/ErrorNote';
import { Modal } from '../components/Modal';
import { InstallDialog } from './AgentsPage';
type Org = {
    id: string;
    name: string;
    slug: string;
    description: string;
    industry: string;
    contact: string;
    version: number;
};
type Page<T> = {
    items: T[];
    next_cursor: string | null;
};
export type Assessment = {
    results_expired_at?: string;
    id: string;
    organization_id: string;
    name: string;
    status: string;
    profile: string;
    created_at: string;
    targets?: {
        asset_id: string;
        name: string;
        status: string;
        skip_reason: string;
        simulation_id: string | null;
    }[];
};
function ErrorMessage({ error }: {
    error: unknown;
}) { useLocale(); return error ? <p className="alert" role="alert">{formatUI(describeError(error).message)}</p> : null; }
function OrgForm({ current, onClose }: {
    current?: Org;
    onClose: () => void;
}) {
    useLocale();
    const qc = useQueryClient();
    const [slugEdited, setSlugEdited] = useState(!!current);
    const [form, setForm] = useState({ name: current?.name ?? '', slug: current?.slug ?? '', description: current?.description ?? '', industry: current?.industry ?? '', contact: current?.contact ?? '' });
    const save = useMutation({ mutationFn: () => current ? api.patch<Org>(`/api/v1/organizations/${current.id}`, { ...form, version: current.version }) : api.post<Org>('/api/v1/organizations', form), onSuccess: async () => { await Promise.all(['organizations', 'setup-organizations', 'home'].map(key => qc.invalidateQueries({ queryKey: [key] }))); onClose(); } });
    return <Modal labelledBy="org-form-title" onClose={onClose}><form className="dialog__form" onSubmit={e => { e.preventDefault(); save.mutate(); }}><h2 id="org-form-title">{formatUI(current ? textUI("m_e735f16f97b6") : textUI("m_1c3b4bf6ffd9"))}</h2>
    {formatUI((['name', 'slug', 'industry', 'contact', 'description'] as const).map(key => <div className="field" key={key}><label htmlFor={`org-${key}`}>{formatUI(key.charAt(0).toUpperCase() + key.slice(1))}</label><div className="field__input"><input id={`org-${key}`} required={key === 'name' || key === 'slug'} maxLength={key === 'description' ? 4000 : key === 'contact' ? 500 : key === 'slug' ? 100 : 200} value={form[key]} onChange={e => { if (key === 'slug')
        setSlugEdited(true); setForm({ ...form, [key]: e.target.value, ...(key === 'name' && !slugEdited ? { slug: e.target.value.toLowerCase().normalize('NFKD').replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '') } : {}) }); }}/></div></div>))}
    <p className="field__hint">{textUI("m_505233cf57b9")}</p><ErrorMessage error={save.error}/><footer className="dialog__foot"><button type="button" className="btn btn--secondary btn--auto" onClick={onClose}>{textUI("m_19766ed6ccb2")}</button><button className="btn btn--primary btn--auto" disabled={save.isPending}>{textUI("m_f55d1a1c73b8")}</button></footer></form></Modal>;
}
const PAGE_SIZE = 12;
const SORTS = ['name', 'assets', 'last', 'findings'] as const;

export function OrganizationsPage() {
    useLocale();
    const { data: me } = useMe();
    const [searchParams, setSearchParams] = useSearchParams();
    const [create, setCreate] = useState(searchParams.get('new') === '1' && can(me, 'organization.write'));
    const all = useAllOrganizations();
    const home = useHome(can(me, 'organization.read'));
    const agents = useAgents(can(me, 'agent.read'));
    const canReport = can(me, 'report.read');
    const [q, setQ] = useState('');
    const [industry, setIndustry] = useState('');
    const [sort, setSort] = useStoredChoice<SortKey>('orchis.orgs.sort', 'name', SORTS);
    const [view, setView] = useStoredChoice<ViewMode>('orchis.orgs.view', 'grid', ['grid', 'list']);
    const [page, setPage] = useState(1);
    useEffect(() => setPage(1), [q, industry, sort]);

    const items = all.data ?? [];
    const overview = new Map((home.data?.organizations ?? []).map((o) => [o.id, o]));
    const agentStats = new Map<string, { agents: number; online: number }>();
    (agents.data ?? []).forEach((a) => {
        if (!a.organization_id || a.status === 'revoked') return;
        const s = agentStats.get(a.organization_id) ?? { agents: 0, online: 0 };
        s.agents += 1;
        if (a.status === 'online') s.online += 1;
        agentStats.set(a.organization_id, s);
    });
    const industries = [...new Set(items.map((o) => o.industry.trim().toLowerCase()).filter(Boolean))].sort();
    const needle = q.trim().toLowerCase();
    const matched = items.filter((o) => (!industry || o.industry.trim().toLowerCase() === industry) && (!needle || [o.name, o.slug, o.industry, o.description].some((f) => f.toLowerCase().includes(needle))));
    // Urutan "terakhir" dan "temuan" butuh laporan semua organisasi yang cocok; selain itu cukup halaman yang tampil.
    const needAllReports = sort === 'last' || sort === 'findings';
    const reports = useOrgReports((needAllReports ? matched : matched.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE)).map((o) => o.id), canReport);
    const sorted = [...matched].sort((x, y) => {
        if (sort === 'assets') return (overview.get(y.id)?.assets ?? 0) - (overview.get(x.id)?.assets ?? 0) || x.name.localeCompare(y.name);
        if (sort === 'last') return Date.parse(reports[y.id]?.created_at ?? '') - Date.parse(reports[x.id]?.created_at ?? '') || x.name.localeCompare(y.name);
        if (sort === 'findings') return totalFindings(reports[y.id]) - totalFindings(reports[x.id]) || x.name.localeCompare(y.name);
        return x.name.localeCompare(y.name);
    });
    const pageItems = sorted.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE);
    const pageReports = useOrgReports(pageItems.map((o) => o.id), canReport && !needAllReports);
    const reportOf = (id: string) => (needAllReports ? reports[id] : pageReports[id] ?? reports[id]);
    const stats = home.data;
    const filtered = !!(needle || industry);
    const header = <PageHeader titleId="orgs-title" crumbs={[{ label: textUI("m_87bb59ba2f92") }, { label: textUI("m_7bd34abba2ae") }]} title={textUI("m_7bd34abba2ae")} subtitle={textUI("m_a5577299d5a1")} actions={<>
        {canReport && <Link className="btn btn--secondary btn--sm" to="/reports">{textUI("m_orgs_reports_btn")}</Link>}
        {can(me, 'organization.write') && <button type="button" className="btn btn--primary btn--sm" onClick={() => setCreate(true)}><Plus size={14} aria-hidden="true"/>{textUI("m_1c3b4bf6ffd9")}</button>}
      </>}/>;
    return <section className="page orgs" aria-labelledby="orgs-title">
      {header}
      {!all.isError && (all.isPending || items.length > 0) && <div className="stats">
        <StatCard label={textUI("m_7bd34abba2ae")} value={items.length} loading={all.isPending}/>
        {(home.isPending || stats?.assets) && <StatCard label={textUI("m_orgs_stat_assets")} value={stats?.assets?.total} loading={home.isPending}/>}
        {(home.isPending || stats?.findings) && <StatCard label={textUI("m_orgs_stat_findings")} value={stats?.findings?.total} loading={home.isPending}>
          {stats?.findings && <div className="stat__sev"><SeverityBar by={stats.findings.by_severity}/><span className="sr-only">{textUI("m_orgs_stat_sev")}</span></div>}
        </StatCard>}
        {(home.isPending || stats?.latest_runs?.[0]) && <StatCard label={textUI("m_orgs_stat_last")} text={stats?.latest_runs?.[0] ? relativeTime(stats.latest_runs[0].created_at) : undefined} loading={home.isPending}/>}
      </div>}
      {all.isError && <div className="banner banner--error" role="alert"><span>{formatUI(describeError(all.error).message)}</span><button type="button" className="btn btn--secondary btn--sm" onClick={() => all.refetch()}>{textUI("m_d8b8392e2c54")}</button></div>}
      {!all.isError && items.length > 0 && <OrgToolbar q={q} onQ={setQ} industries={industries} industry={industry} onIndustry={setIndustry} sort={sort} onSort={setSort} view={view} onView={setView} showFindingsSort={canReport}/>}
      {all.isPending && <ul className={`orgs__list orgs__list--${view}`} aria-busy="true" aria-label={textUI("m_orgs_loading")}>{[0, 1, 2].map((i) => <OrgCardSkeleton key={i}/>)}</ul>}
      {all.isSuccess && items.length === 0 && <EmptyState title={textUI("m_f95a4411a949")} body={textUI("m_de3ad10af78c")}>
        {can(me, 'organization.write') && <button type="button" className="btn btn--primary btn--sm" onClick={() => setCreate(true)}><Plus size={14} aria-hidden="true"/>{textUI("m_1c3b4bf6ffd9")}</button>}
        <Link className="link-quiet" to="/assets?org=unassigned">{textUI("m_929c0876caac")}</Link>
      </EmptyState>}
      {all.isSuccess && items.length > 0 && matched.length === 0 && <EmptyState title={textUI("m_orgs_nomatch_t")} body={textUI("m_orgs_nomatch_b")}>
        <button type="button" className="btn btn--secondary btn--sm" onClick={() => { setQ(''); setIndustry(''); }}>{textUI("m_orgs_clear")}</button>
      </EmptyState>}
      {pageItems.length > 0 && <ul className={`orgs__list orgs__list--${view}`} aria-label={textUI("m_orgs_list_aria")}>
        {formatUI(pageItems.map((o, i) => <li className="oc-wrap" key={o.id} style={{ ['--i' as string]: i }}>
          <OrgCard org={o} metrics={{ assets: overview.get(o.id)?.assets, agents: agents.data ? (agentStats.get(o.id)?.agents ?? 0) : undefined, online: agentStats.get(o.id)?.online ?? 0 }}
            report={reportOf(o.id)} reportKnown={reportOf(o.id) !== undefined || !canReport ? true : false} canReport={canReport} canRun={can(me, 'assessment.run') && can(me, 'simulation.run')}/>
        </li>))}
      </ul>}
      {filtered && pageItems.length === 0 ? null : <Pagination page={page} total={sorted.length} pageSize={PAGE_SIZE} onPage={setPage}/>}
      {formatUI(create && <OrgForm onClose={() => { setCreate(false); setSearchParams({}, { replace: true }); }}/>)}</section>;
}
export function OrganizationDetailPage() {
    useLocale();
    const { id = '' } = useParams();
    const { data: me } = useMe();
    const qc = useQueryClient();
    const [edit, setEdit] = useState(false);
    const [install, setInstall] = useState(false);
    const [tabParams, setTabParams] = useSearchParams();
    const requestedTab = tabParams.get('tab') ?? 'overview';
    const tab = ['overview', 'agents', 'assets', 'scans', 'findings', 'reports'].includes(requestedTab) ? requestedTab : 'overview';
    const setTab = (value: string) => { const next = new URLSearchParams(tabParams); next.set('tab', value); setTabParams(next); };
    const [cursor, setCursor] = useState('');
    const [selected, setSelected] = useState<Record<string, number>>({});
    const [assign, setAssign] = useState(false);
    const q = useQuery({ queryKey: ['organizations', id], queryFn: () => api.get<Org>(`/api/v1/organizations/${id}`) });
    const assets = useQuery({ queryKey: ['organization-assets', id, assign, cursor], enabled: can(me, 'asset.read'), queryFn: () => api.get<Page<Asset>>(`/api/v1/assets?limit=100&organization_id=${assign ? 'unassigned' : id}&cursor=${encodeURIComponent(cursor)}`) });
    const agents = useAgents(can(me, 'agent.read'));
    const latest = useQuery({ queryKey: ['organization-report', id], enabled: can(me, 'report.read') && can(me, 'finding.read'), queryFn: () => api.get<{
            items: {
                id: string;
                snapshot: {
                    risk: string;
                    by_severity: Record<string, number>;
                };
            }[];
        }>(`/api/v1/reports?organization_id=${id}`), refetchInterval: 5000 });
    const members = useMutation({ mutationFn: (options: {
            testing_authorized?: boolean;
            unassign?: boolean;
        }) => api.put(`/api/v1/organizations/${id}/assets`, { assets: Object.entries(selected).map(([id, version]) => ({ id, version })), ...options }), onSuccess: async () => { setSelected({}); await Promise.all(['organization-assets', 'organization-readiness', 'assets', 'asset', 'agents', 'home'].map(key => qc.invalidateQueries({ queryKey: [key] }))); } });
    if (q.isPending)
        return <p role="status">{textUI("m_580b0448c5b8")}</p>;
    if (!q.data)
        return <ErrorMessage error={q.error}/>;
    const o = q.data;
    const localAgents = agents.data?.filter(a => a.organization_id === id) ?? [];
    const bySev = latest.data?.items[0]?.snapshot.by_severity;
    return <section className="page orgd"><Link to="/organizations" className="link-quiet">{textUI("m_c242e176c772")}</Link><div className="page__head"><div><p className="eyebrow">{formatUI(o.industry || textUI("org_eyebrow_default"))}</p><h1 className="page__title">{formatUI(o.name)}</h1><p className="page__sub">{formatUI(o.description || textUI("m_938d049615ff"))}</p><p className="muted">{formatUI(o.contact)}</p></div><div className="row">{formatUI(can(me, 'assessment.run') && can(me, 'simulation.run') && <Link className="btn btn--primary btn--auto" to={`/simulations/new?org=${id}`}>{textUI("m_ea1c510d1ddb")}</Link>)}{formatUI(can(me, 'organization.write') && <button className="btn btn--secondary btn--auto" onClick={() => setEdit(true)}>{textUI("m_e735f16f97b6")}</button>)}{formatUI(can(me, 'agent.manage') && can(me, 'organization.write') && <button className="btn btn--secondary btn--auto" onClick={() => setInstall(true)}>{textUI("m_2816f2ffe728")}</button>)}</div></div>
    <ScanControls org={id} compact/>
    {formatUI(tab === 'overview' && <OrgOverviewStats orgID={id} agents={localAgents} bySeverity={bySev} canAssets={can(me, 'asset.read')} canScans={can(me, 'assessment.read') && can(me, 'simulation.read')} canFindings={can(me, 'report.read') && can(me, 'finding.read')}/>)}
    {formatUI(advancedEnabled(me) && can(me, 'ai.read') && <p><Link to={`/organizations/${id}/ai`}>{textUI("m_c7db80dbcbe1")}</Link></p>)}{formatUI(tab === 'overview' && can(me, 'asset.read') && <AssessmentReadiness id={id}/>)}
    {formatUI(tab === 'overview' && latest.data?.items[0] && <section className="org-card" aria-label={textUI("m_205881c06747")}><p className="eyebrow">{textUI("m_d57cfdbf6eb3")}</p><h2>{formatUI(latest.data.items[0].snapshot.risk)}</h2><div className="severity-strip">{formatUI(['critical', 'high', 'medium', 'low', 'info'].map(severity => <p key={severity}><strong>{formatUI(latest.data!.items[0]!.snapshot.by_severity[severity] ?? 0)}</strong> {formatUI(severity)}</p>))}</div><Link to={`/reports/${latest.data.items[0].id}`}>{textUI("m_a7ff4f55b2f9")}</Link></section>)}
    <ErrorMessage error={members.error}/><nav className="tabs" aria-label={textUI("m_3c324254fabb")}>{formatUI(['overview', ...(can(me, 'agent.read') ? ['agents'] : []), ...(can(me, 'asset.read') ? ['assets'] : []), ...(can(me, 'assessment.read') ? ['scans'] : []), ...(can(me, 'finding.read') ? ['findings'] : []), ...(can(me, 'report.read') ? ['reports'] : [])].map(t => <button key={t} className={`tab ${tab === t ? 'is-active' : ''}`} aria-pressed={tab === t} onClick={() => setTab(t)}>{formatUI(t === 'agents' ? textUI("m_a6ea76b2725e") : textUI(({overview:'tab_overview',assets:'tab_assets',scans:'tab_scans',findings:'tab_findings',reports:'tab_reports'} as const)[t as 'overview']))}</button>))}</nav>
    {formatUI(tab === 'overview' && <p>{textUI("m_7d392c72d06d")}</p>)}
    {formatUI(tab === 'reports' && <ReportsPage organizationID={id} organizationName={o.name}/>)}
    {formatUI(tab === 'findings' && <FindingsPage organizationID={id} organizationName={o.name}/>)}
    {formatUI(tab === 'assets' && <><AssetsPage organizationID={id} organizationName={o.name}/><details><summary>{textUI("m_9403bbbdfcc2")}</summary><div className="row"><button className="btn btn--secondary btn--auto" onClick={() => { setAssign(!assign); setSelected({}); setCursor(''); }}>{formatUI(assign ? textUI("m_50994d4c508b") : textUI("m_3a634c05b884"))}</button>{formatUI(can(me, 'organization.write') && can(me, 'asset.write') && <button className="btn btn--secondary btn--auto" disabled={!Object.keys(selected).length || members.isPending} onClick={() => members.mutate(assign ? {} : { unassign: true })}>{formatUI(assign ? textUI("m_fe751c91825c") : textUI("m_de9c719b5627"))}</button>)}{formatUI(can(me, 'asset.authorize') && can(me, 'organization.write') && !assign && <button className="btn btn--secondary btn--auto" disabled={!Object.keys(selected).length || members.isPending} onClick={() => members.mutate({ testing_authorized: true })}>{textUI("m_5cf9ccd97a0e")}</button>)}</div><ErrorMessage error={assets.error}/>
    <div className="org-table-wrap" role="region" aria-label={textUI("m_80a5f49abcd1")} tabIndex={0}><table className="org-table"><thead><tr><th>{textUI("m_2a78025de6aa")}</th><th>{textUI("m_80d298c9f240")}</th><th>{textUI("m_e806a291cfc3")}</th><th>{textUI("m_11b39c93777e")}</th></tr></thead><tbody>{formatUI(assets.data?.items.map(a => <tr key={a.id}><td><input type="checkbox" aria-label={textUI("m_5d348f18de17", [a.name])} checked={selected[a.id] !== undefined} onChange={e => setSelected(s => { const v = { ...s }; if (e.target.checked)
            v[a.id] = a.version;
        else
            delete v[a.id]; return v; })}/></td><td><Link to={`/assets/${a.id}`}>{formatUI(a.name)}</Link></td><td>{formatUI(a.testing_authorized ? textUI("m_67f542ba6d8f") : textUI("m_52c79c7b5ba0"))}</td><td>{formatUI(a.agent_state === 'none' ? textUI("m_30f96cff8944") : a.agent_state)}</td></tr>))}</tbody></table></div>{formatUI(assets.data?.items.length === 0 && <div className="state"><h2>{textUI("m_e3d7edd3667c")}</h2><p>{formatUI(assign ? textUI("m_d6f66e4c90a6") : textUI("m_f4a82eba6b6a"))}</p>{formatUI(can(me, 'asset.write') && <Link to={`/organizations/${id}?tab=assets&new=1`}>{textUI("m_719aca414b88")}</Link>)}</div>)}<div className="row"><button className="btn btn--secondary btn--auto" disabled={!cursor} onClick={() => setCursor('')}>{textUI("m_14a7f568d2a9")}</button><button className="btn btn--secondary btn--auto" disabled={!assets.data?.next_cursor} onClick={() => { setCursor(assets.data!.next_cursor!); setSelected({}); }}>{textUI("m_1f4e681dd0c4")}</button></div></details></>)}
    {formatUI(tab === 'agents' && <OrganizationEndpoints org={id} agents={localAgents}/>)}
    {formatUI(tab === 'scans' && <ScanList organizationID={id} organizationName={o.name} embedded/>)}
    {formatUI(tab === 'overview' && <ScanControls org={id}/>)}
    {formatUI(tab === 'overview' && can(me, 'settings.manage') && <DeleteOrganization key={id} org={id}/>)}
    {formatUI(edit && <OrgForm current={o} onClose={() => setEdit(false)}/>)}{formatUI(install && <InstallDialog organizationID={id} onClose={() => setInstall(false)}/>)}</section>;
}
export function AssessmentPage() {
    useLocale();
    const { id = '' } = useParams();
    const { data: me } = useMe();
    const qc = useQueryClient();
    const q = useQuery({ queryKey: ['assessment', id], queryFn: () => api.get<Assessment>(`/api/v1/assessments/${id}`), refetchInterval: 3000 });
    const cancel = useMutation({ mutationFn: () => api.post(`/api/v1/assessments/${id}/cancel`), onSuccess: () => qc.invalidateQueries({ queryKey: ['assessment', id] }) });
    if (q.isPending)
        return <p role="status">{textUI("m_1ff97a5cf95b")}</p>;
    if (!q.data)
        return <ErrorMessage error={q.error}/>;
    const a = q.data;
    return <section className="page"><Link to={`/organizations/${a.organization_id}`}>{textUI("m_7f371c34a81e")}</Link><div className="page__head"><div><p className="eyebrow">{richUI("m_d643132988fa", [formatUI(a.profile)])}</p><h1 className="page__title">{formatUI(a.name)}</h1><p className="page__sub" role="status">{formatUI(a.status)}</p><p className="muted">{textUI("m_b647e37d8109")}</p></div>{formatUI(a.status === 'running' && can(me, 'assessment.run') && <button className="btn btn--danger btn--auto" disabled={cancel.isPending} onClick={() => cancel.mutate()}>{textUI("m_fe6573bb6f34")}</button>)}</div><ErrorMessage error={cancel.error}/>{formatUI(can(me, 'report.read') && <Link to={`/reports?assessment_id=${id}`}>{textUI("m_9ee9e99f3e0d")}</Link>)}{formatUI(a.results_expired_at && <p role="status" className="alert">{textUI("m_1711a4c34b1c")}</p>)}<div className="org-grid">{formatUI(a.targets?.map(t => <article key={t.asset_id} className="org-card"><h2><Link to={`/assets/${t.asset_id}`}>{formatUI(t.name)}</Link></h2><p>{formatUI(t.status)}</p>{formatUI(t.skip_reason && <p>{formatUI(t.skip_reason)}</p>)}{formatUI(t.simulation_id && <Link to={`/simulations/legacy/${t.simulation_id}`}>{textUI("m_383a1b092841")}</Link>)}</article>))}</div></section>;
}

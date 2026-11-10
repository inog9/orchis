import { t as textUI, tr as richUI, useLocale, fv as formatUI, locale } from "../i18n/runtime";
import { DeleteOrganization } from '../components/DeleteOrganization';
import { ScanControls, useScanControl } from '../components/ScanControls';
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
import '../components/pen/pen.css';
import { EmptyState } from '../components/orgs/EmptyState';
import { OrgCard, OrgCardSkeleton, SeverityBar } from '../components/orgs/OrgCard';
import { OrgToolbar, type SortKey, type ViewMode } from '../components/orgs/OrgToolbar';
import { PageHeader } from '../components/orgs/PageHeader';
import { Pagination } from '../components/orgs/Pagination';
import { StatCard } from '../components/orgs/StatCard';
import { OrgOverviewStats } from '../components/orgs/OrgOverviewStats';
import { CoveragePanel } from '../components/orgs/CoveragePanel';
import { totalFindings, useAllOrganizations, useOrgReports, useStoredChoice } from '../components/orgs/useOrgData';
import { relativeTime } from '../lib/relativeTime';
import { MetricCard, PanelCard, StateTag } from '../components/pen/primitives';
import { StatusBadge } from '../components/sims/StatusBadge';
import { useReadiness, type ReadinessItem } from '../api/readiness';
import { localizeReason } from '../lib/readinessReason';
import { Link, useParams, useSearchParams } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/client';
import { useAgents, useHome, useMe } from '../api/hooks';
import { can, type Asset, type AgentRow } from '../api/types';
import { describeError } from '../components/ErrorNote';
import { OrgForm, type Org } from './OrgForm';
import { InstallDialog } from './AgentsPage';
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
    // Bawaan daftar (baris) sesuai fixed.pen "BARIS TABEL"; pilihan pengguna tetap diingat.
    const [view, setView] = useStoredChoice<ViewMode>('orchis.orgs.view', 'list', ['grid', 'list']);
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
// Waktu lokal untuk kolom "Waktu" dan heartbeat; ISO kosong atau tak valid mengembalikan kosong.
function whenLabel(iso: string | null | undefined, timeStyle: 'short' | 'medium' = 'short'): string {
    if (!iso) return '';
    const d = new Date(iso);
    return Number.isNaN(d.getTime()) ? '' : d.toLocaleString(locale(), { dateStyle: 'medium', timeStyle });
}

// Bagian "Aset" (spec HALAMAN 5): tabel kesiapan dari GET /organizations/{id}/readiness.
// Kolom: nama, platform, status agent (titik hijau bila online), status pindai terakhir, waktu.
function OrgAssetsSection({ orgID, readiness, canWrite }: {
    orgID: string;
    readiness: ReturnType<typeof useReadiness>;
    canWrite: boolean;
}) {
    useLocale();
    const rows = readiness.data?.items ?? [];
    return <PanelCard title={textUI("pen_orgd_assets_title")} actions={canWrite ? <Link className="btn btn--secondary btn--auto" to={`/organizations/${orgID}?tab=assets&new=1`}>{textUI("m_719aca414b88")}</Link> : undefined}>
      {readiness.isError && <div className="alert" role="alert"><span>{formatUI(describeError(readiness.error).message)}</span><button type="button" className="btn btn--secondary btn--sm" onClick={() => readiness.refetch()}>{textUI("m_d8b8392e2c54")}</button></div>}
      {readiness.isPending && <p role="status">{textUI("m_6470b2b671b5")}</p>}
      {!readiness.isPending && !readiness.isError && rows.length === 0 && <div className="state" role="status"><h2>{textUI("pen_orgd_assets_empty_t")}</h2><p>{textUI("pen_orgd_assets_empty_b")}</p></div>}
      {rows.length > 0 && <div className="orgd__tablewrap" role="region" aria-label={textUI("pen_orgd_assets_aria")} tabIndex={0}><table className="orgd__table">
        <thead><tr><th scope="col">{textUI("pen_orgd_col_asset")}</th><th scope="col">{textUI("pen_orgd_col_platform")}</th><th scope="col">{textUI("pen_orgd_col_agent")}</th><th scope="col">{textUI("pen_orgd_col_scan")}</th><th scope="col">{textUI("pen_orgd_col_time")}</th></tr></thead>
        <tbody>{rows.map(item => <OrgAssetRow key={item.id} item={item}/>)}</tbody>
      </table></div>}
    </PanelCard>;
}

// Satu baris aset: titik hijau hanya saat agent_status=online (spec HALAMAN 5).
function OrgAssetRow({ item }: { item: ReadinessItem }) {
    const platform = [item.os, item.arch].filter(Boolean).join(' · ');
    return <tr>
      <td><div className="orgd__asset"><span className="orgd__monogram" aria-hidden="true">{item.name.slice(0, 2).toUpperCase()}</span>
        <div className="orgd__asset-text"><Link className="orgd__asset-name" to={`/assets/${item.id}`}>{formatUI(item.name)}</Link>
          <span className="orgd__asset-tag" title={item.reason ? localizeReason(item.reason) : undefined}><StateTag tone={item.reason ? 'neutral' : 'success'}>{item.reason ? textUI("pen_orgd_not_ready") : textUI("pen_orgd_ready")}</StateTag></span>
        </div></div></td>
      <td>{platform || '-'}</td>
      <td><span className={`orgd__agentcell${item.agent_status === 'online' ? ' orgd__agentcell--on' : ''}`}><span className={`orgd__dot${item.agent_status === 'online' ? ' orgd__dot--on' : ''}`} aria-hidden="true"/>{textUI(item.agent_status === 'online' ? "pen_orgd_agent_online" : item.agent_status === 'offline' ? "pen_orgd_agent_offline" : "pen_orgd_agent_none")}</span></td>
      <td>{item.last_scan_status ? <StatusBadge status={item.last_scan_status}/> : textUI("pen_orgd_scan_none")}</td>
      <td className="mono">{whenLabel(item.last_scan_at) || '-'}</td>
    </tr>;
}

// Bagian "Agen terpasang" (spec HALAMAN 5): kartu ringkas per agent milik organisasi ini:
// hostname, os/arch, tag status, versi agent, dan heartbeat terakhir (nilai teknis memakai mono).
function OrgAgentsSection({ agents, canInstall, onInstall }: {
    agents: AgentRow[];
    canInstall: boolean;
    onInstall: () => void;
}) {
    useLocale();
    return <PanelCard title={textUI("pen_orgd_agents_title")} actions={canInstall ? <button type="button" className="btn btn--secondary btn--sm" onClick={onInstall}>{textUI("m_2816f2ffe728")}</button> : undefined}>
      {agents.length === 0 ? <p role="status">{textUI("pen_orgd_agents_empty")}</p> : <ul className="orgd__agents">
        {agents.map(a => <li key={a.id} className="orgd__agent">
          <div className="orgd__agent-top"><h3>{a.hostname || a.asset_name}</h3><StateTag tone={a.status === 'online' ? 'success' : 'neutral'}>{textUI(a.status === 'online' ? "pen_orgd_agent_online" : "pen_orgd_agent_offline")}</StateTag></div>
          <p className="orgd__agent-os">{`${a.os} · ${a.arch}`}</p>
          <dl className="orgd__agent-meta">
            <div><dt>{textUI("pen_orgd_agent_version")}</dt><dd className="mono">{a.version}</dd></div>
            <div><dt>{textUI("pen_orgd_agent_heartbeat")}</dt><dd className="mono">{whenLabel(a.last_heartbeat_at, 'medium') || '-'}</dd></div>
          </dl>
        </li>)}
      </ul>}
    </PanelCard>;
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
        }) => api.put(`/api/v1/organizations/${id}/assets`, { assets: Object.entries(selected).map(([id, version]) => ({ id, version })), ...options }), onSuccess: async () => { setSelected({}); await Promise.all(['organization-assets', 'organization-readiness', 'readiness', 'assets', 'asset', 'agents', 'home'].map(key => qc.invalidateQueries({ queryKey: [key] }))); } });
    // Data metric strip dan section Aset/Agen (spec HALAMAN 5). useReadiness dari api/readiness
    // punya shape penuh sesuai handler Go (internal/organizations/readiness.go): scan_window_open + emergency_stop.
    const readiness = useReadiness(id, can(me, 'asset.read') && can(me, 'organization.read'));
    const control = useScanControl(id);
    if (q.isPending)
        return <p role="status">{textUI("m_580b0448c5b8")}</p>;
    if (!q.data)
        return <ErrorMessage error={q.error}/>;
    const o = q.data;
    const localAgents = agents.data?.filter(a => a.organization_id === id) ?? [];
    const bySev = latest.data?.items[0]?.snapshot.by_severity;
    // Agent milik organisasi ini; yang dicabut tidak dihitung sebagai terpasang.
    const installedAgents = localAgents.filter(a => a.status !== 'revoked');
    const onlineAgents = installedAgents.filter(a => a.status === 'online').length;
    // Metric strip: angka hanya saat datanya benar-benar ada (tidak menebak dari undefined).
    const r = readiness.data;
    const readyValue = r && typeof r.total === 'number' && typeof r.ready === 'number' ? `${r.ready}/${r.total}` : '-';
    const readyCaption = r && typeof r.authorized === 'number' ? textUI("pen_orgd_metric_ready_caption", [r.authorized]) : undefined;
    const authorizedValue = r && typeof r.authorized === 'number' ? formatUI(r.authorized) : '-';
    const authorizedCaption = r && typeof r.total === 'number' ? textUI("pen_orgd_metric_authorized_caption", [r.total]) : undefined;
    // Jendela pindai: nilai dari readiness.scan_window_open; caption paused/emergency_stop dari scan-control.
    const windowOpen = r?.scan_window_open ?? (control.data ? control.data.allowed_now : undefined);
    const emergency = control.data ? (control.data.paused || control.data.global_paused) : r?.emergency_stop;
    const windowCaptionParts: string[] = [];
    if (control.data) windowCaptionParts.push(textUI(control.data.paused ? "pen_orgd_window_paused" : "pen_orgd_window_not_paused"));
    if (emergency !== undefined) windowCaptionParts.push(textUI("pen_orgd_window_emergency", [emergency ? textUI("pen_orgd_yes") : textUI("pen_orgd_no")]));
    const windowCaption = windowCaptionParts.length > 0 ? windowCaptionParts.join(' · ') : undefined;
    return <section className="page orgd">
    <Link to="/organizations" className="link-quiet">{textUI("m_c242e176c772")}</Link>
    {/* Kepala halaman: overline ORGANISASI, nama, slug+deskripsi (mockup "Page Heading"). */}
    <header className="orgd__head">
      <div className="orgd__heading">
        <p className="orgd__overline">{textUI("pen_orgd_overline")}</p>
        <h1 className="orgd__title">{formatUI(o.name)}</h1>
        <p className="orgd__desc">{formatUI([o.slug, o.description].filter(Boolean).join(' · ') || textUI("m_938d049615ff"))}</p>
        {formatUI((o.industry || o.contact) && <p className="orgd__meta">{formatUI([o.industry, o.contact].filter(Boolean).join(' · '))}</p>)}
      </div>
      <div className="orgd__actions">
        {formatUI(can(me, 'report.read') && <Link className="btn btn--secondary btn--auto" to={`/reports?organization_id=${id}`}>{textUI("pen_orgd_action_reports")}</Link>)}
        {can(me, 'assessment.run') && can(me, 'simulation.run') && <Link className="btn btn--secondary btn--auto" to={`/simulations/schedules?org=${id}&new=1`}>{textUI("sched_create")}</Link>}
        {formatUI(can(me, 'assessment.run') && can(me, 'simulation.run') && <Link className="btn btn--primary btn--auto" to={`/simulations/new?org=${id}`}>{textUI("pen_orgd_action_scan")}</Link>)}
        {formatUI(can(me, 'organization.write') && <button className="btn btn--secondary btn--auto" onClick={() => setEdit(true)}>{textUI("m_e735f16f97b6")}</button>)}
        {formatUI(can(me, 'agent.manage') && can(me, 'organization.write') && <button className="btn btn--secondary btn--auto" onClick={() => setInstall(true)}>{textUI("m_2816f2ffe728")}</button>)}
      </div>
    </header>
    {/* Metric strip selalu di atas tab (spec HALAMAN 5): satu permukaan, sel dipisah garis. */}
    <div className="orgd__metrics" aria-label={textUI("pen_orgd_metrics_aria")} aria-busy={readiness.isPending || undefined}>
      <MetricCard label={textUI("pen_orgd_metric_ready")} value={readyValue} caption={readyCaption}/>
      <MetricCard label={textUI("pen_orgd_metric_authorized")} value={authorizedValue} caption={authorizedCaption}/>
      <MetricCard label={textUI("pen_orgd_metric_agents")} value={agents.data ? formatUI(onlineAgents) : '-'} caption={agents.data ? textUI("pen_orgd_metric_agents_caption", [installedAgents.length]) : undefined}/>
      <MetricCard label={textUI("pen_orgd_metric_window")} value={windowOpen === undefined ? '-' : windowOpen ? textUI("pen_orgd_window_open") : textUI("pen_orgd_window_closed")} caption={windowCaption}/>
    </div>
    <ScanControls org={id} compact/>
    <nav className="tabs" aria-label={textUI("m_3c324254fabb")}>{formatUI(['overview', ...(can(me, 'agent.read') ? ['agents'] : []), ...(can(me, 'asset.read') ? ['assets'] : []), ...(can(me, 'assessment.read') ? ['scans'] : []), ...(can(me, 'finding.read') ? ['findings'] : []), ...(can(me, 'report.read') ? ['reports'] : [])].map(t => <button key={t} className={`tab ${tab === t ? 'is-active' : ''}`} aria-pressed={tab === t} onClick={() => setTab(t)}>{formatUI(t === 'agents' ? textUI("m_a6ea76b2725e") : textUI(({overview:'tab_overview',assets:'tab_assets',scans:'tab_scans',findings:'tab_findings',reports:'tab_reports'} as const)[t as 'overview']))}</button>))}</nav>
    <ErrorMessage error={members.error}/>
    {/* Konten tab Overview: section Aset dan Agen diletakkan di tab Overview (bukan di atas tab)
        agar tab lain tetap fokus dan tabel besar tidak diduplikasi di setiap tab; strip metrik
        tetap selalu terlihat di atas. Overview adalah tab default, susunan mockup langsung terlihat. */}
    {formatUI(tab === 'overview' && <p className="orgd__intro">{textUI("m_7d392c72d06d")}</p>)}
    {formatUI(tab === 'overview' && can(me, 'asset.read') && can(me, 'organization.read') && <OrgAssetsSection orgID={id} readiness={readiness} canWrite={can(me, 'asset.write')}/>)}
    {formatUI(tab === 'overview' && can(me, 'agent.read') && <OrgAgentsSection agents={installedAgents} canInstall={can(me, 'agent.manage') && can(me, 'organization.write')} onInstall={() => setInstall(true)}/>)}
    {formatUI(tab === 'overview' && <OrgOverviewStats orgID={id} agents={localAgents} bySeverity={bySev} canAssets={can(me, 'asset.read')} canScans={can(me, 'assessment.read') && can(me, 'simulation.read')} canFindings={can(me, 'report.read') && can(me, 'finding.read')}/>)}
    {formatUI(advancedEnabled(me) && can(me, 'ai.read') && <p><Link to={`/organizations/${id}/ai`}>{textUI("m_c7db80dbcbe1")}</Link></p>)}{formatUI(tab === 'overview' && can(me, 'asset.read') && <><AssessmentReadiness id={id}/></>)}
    {formatUI(tab === 'overview' && latest.data?.items[0] && <section className="org-card" aria-label={textUI("m_205881c06747")}><p className="eyebrow">{textUI("m_d57cfdbf6eb3")}</p><h2>{formatUI(latest.data.items[0].snapshot.risk)}</h2><div className="severity-strip">{formatUI(['critical', 'high', 'medium', 'low', 'info'].map(severity => <p key={severity}><strong>{formatUI(latest.data!.items[0]!.snapshot.by_severity[severity] ?? 0)}</strong> {formatUI(severity)}</p>))}</div><Link to={`/reports/${latest.data.items[0].id}`}>{textUI("m_a7ff4f55b2f9")}</Link></section>)}
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

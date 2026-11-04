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
import { useState } from 'react';
import { Link, useParams, useSearchParams } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/client';
import { useAgents, useMe } from '../api/hooks';
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
export function OrganizationsPage() {
    useLocale();
    const { data: me } = useMe();
    const [cursor, setCursor] = useState('');
    const [searchParams, setSearchParams] = useSearchParams();
    const [create, setCreate] = useState(searchParams.get('new') === '1' && can(me, 'organization.write'));
    const q = useQuery({ queryKey: ['organizations', cursor], queryFn: () => api.get<Page<Org>>(`/api/v1/organizations?cursor=${encodeURIComponent(cursor)}`) });
    return <section className="page"><div className="page__head"><div><p className="eyebrow">{textUI("m_5493ff8e9397")}</p><h1 className="page__title">{textUI("m_7bd34abba2ae")}</h1><p className="page__sub">{textUI("m_a5577299d5a1")}</p></div>{formatUI(can(me, 'organization.write') && <button className="btn btn--primary btn--auto" onClick={() => setCreate(true)}>{textUI("m_1c3b4bf6ffd9")}</button>)}</div>
    <ErrorMessage error={q.error}/>{formatUI(q.isPending ? <p role="status">{textUI("m_8be87068539e")}</p> : <div className="org-grid">{formatUI(q.data?.items.map(o => <Link className="org-card" aria-label={o.name} key={o.id} to={`/organizations/${o.id}`}><span className="eyebrow">{formatUI(o.industry || textUI("org_eyebrow_default"))}</span><h2>{formatUI(o.name)}</h2><p>{formatUI(o.description || textUI("m_a352158377b2"))}</p><span className="link-quiet">{textUI("m_91020e1cae44")}</span></Link>))}</div>)}
    {formatUI(q.data?.items.length === 0 && <div className="state"><h2>{textUI("m_f95a4411a949")}</h2><p>{textUI("m_de3ad10af78c")}</p><Link to="/assets?org=unassigned">{textUI("m_929c0876caac")}</Link></div>)}
    <div className="row"><button className="btn btn--secondary btn--auto" disabled={!cursor} onClick={() => setCursor('')}>{textUI("m_0bdbb750609c")}</button><button className="btn btn--secondary btn--auto" disabled={!q.data?.next_cursor} onClick={() => setCursor(q.data!.next_cursor!)}>{textUI("m_c08ac736a5e2")}</button></div>
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
    return <section className="page"><Link to="/organizations" className="link-quiet">{textUI("m_c242e176c772")}</Link><div className="page__head"><div><p className="eyebrow">{formatUI(o.industry || textUI("org_eyebrow_default"))}</p><h1 className="page__title">{formatUI(o.name)}</h1><p className="page__sub">{formatUI(o.description || textUI("m_938d049615ff"))}</p><p className="muted">{formatUI(o.contact)}</p></div><div className="row">{formatUI(can(me, 'assessment.run') && can(me, 'simulation.run') && <Link className="btn btn--primary btn--auto" to={`/simulations/new?org=${id}`}>{textUI("m_ea1c510d1ddb")}</Link>)}{formatUI(can(me, 'organization.write') && <button className="btn btn--secondary btn--auto" onClick={() => setEdit(true)}>{textUI("m_e735f16f97b6")}</button>)}{formatUI(can(me, 'agent.manage') && can(me, 'organization.write') && <button className="btn btn--secondary btn--auto" onClick={() => setInstall(true)}>{textUI("m_2816f2ffe728")}</button>)}</div></div>
    <ScanControls org={id}/>
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

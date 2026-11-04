import {displayLabel} from '../i18n/display';
import {language} from '../i18n/runtime';
import { t as textUI, tr as richUI, useLocale, locale, fv as formatUI } from "../i18n/runtime";
import { ScanComparison } from '../components/ScanComparison';
import { ScanControls, useScanControl } from '../components/ScanControls';
import { useMemo, useState } from 'react';
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/client';
import { useAgents, useAssets, useMe } from '../api/hooks';
import { useScans, type Scan, type ScanInput } from '../api/scans';
import { can } from '../api/types';
import { OrganizationFilter } from '../components/OrganizationFilter';
import { AgentNetworks } from '../components/AgentNetworks';
import { describeError } from '../components/ErrorNote';
import { LegacySimulations } from './LegacySimulations';
import { scannable } from './categorySchema';
export function ScanList({ organizationID, organizationName, embedded = false }: {
    organizationID?: string;
    organizationName?: string;
    embedded?: boolean;
} = {}) {
    useLocale();
    const { data: me } = useMe();
    const [params, setParams] = useSearchParams();
    const [cursor, setCursor] = useState('');
    const org = organizationID ?? params.get('org') ?? '';
    const q = useScans(org, cursor, can(me, 'assessment.read') && can(me, 'simulation.read') && can(me, 'asset.read') && can(me, 'organization.read'));
    const Heading = embedded ? 'h2' : 'h1';
    return <section className="page"><div className="page__head"><div><p className="eyebrow">{textUI("m_b0537b978fef")}</p><Heading className="page__title">{formatUI(embedded ? textUI("m_d128917a9540") : textUI("m_078a864ea0f1"))}</Heading><p className="page__sub">{textUI("m_bb8add5b0686")}</p></div>{formatUI(can(me, 'assessment.run') && can(me, 'simulation.run') && <Link className="btn btn--primary btn--auto" to={`/simulations/new${org ? `?org=${encodeURIComponent(org)}` : ''}`}>{textUI("m_de2b814aeb96")}</Link>)}</div><div className="toolbar"><OrganizationFilter value={org} name={organizationName} locked={!!organizationID} onChange={v => { const next = new URLSearchParams(params); if (v)
        next.set('org', v);
    else
        next.delete('org'); setParams(next); setCursor(''); }}/></div>{formatUI(q.isPending && q.fetchStatus !== 'idle' && <p role="status">{textUI("m_e0b80da465d6")}</p>)}{formatUI(q.error && <p role="alert" className="alert">{formatUI(describeError(q.error).message)}</p>)}<div className="org-grid">{formatUI(q.data?.items.map(s => <article key={s.id} className="org-card"><h2><Link to={`/simulations/${s.id}`}>{formatUI(s.name)}</Link></h2><p>{formatUI(s.status)} · {formatUI(s.options.tools.join(' + '))} · {formatUI(s.options.preset)}</p><p className="muted">{richUI("m_9afcf13c0f9e", [formatUI(new Date(s.created_at).toLocaleString(locale()))])}</p></article>))}</div>{formatUI(q.data?.items.length === 0 && <div className="state"><h2>{textUI("m_3c2ec23816e3")}</h2><p>{textUI("m_9ec336bb3650")}</p></div>)}<div className="row"><button className="btn btn--secondary btn--auto" disabled={!cursor} onClick={() => setCursor('')}>{textUI("m_9990a5defe90")}</button><button className="btn btn--secondary btn--auto" disabled={!q.data?.next_cursor} onClick={() => setCursor(q.data!.next_cursor!)}>{textUI("m_c0f7567862e4")}</button></div>{formatUI(!embedded && <details><summary>{textUI("m_04ab162cdd89")}</summary><p>{textUI("m_4310bb792a7d")}</p><LegacySimulations /></details>)}</section>;
}
export function NewScanPage() {
    useLocale();
    const { data: me } = useMe();
    const [params, setParams] = useSearchParams();
    const navigate = useNavigate();
    const qc = useQueryClient();
    const org = params.get('org') ?? '';
    const [assets, setAssets] = useState<string[]>([]);
    const [agent, setAgent] = useState('');
    const [ranges, setRanges] = useState<string[]>([]);
    const [tools, setTools] = useState(['nmap', 'nuclei']);
    const [preset, setPreset] = useState<'quick' | 'standard'>('quick');
    const [validation, setValidation] = useState('');
    const control = useScanControl(org || undefined, !!org);
    const list = useAssets('', '', org, !!org);
    const agents = useAgents(!!org && can(me, 'agent.read'));
    const localAgents = agents.data?.filter(a => a.organization_id === org && a.status !== 'revoked' && !a.is_demo) ?? [];
    const allAssets = list.data?.pages.flatMap(p => p.items) ?? [];
    const scanTargets = allAssets.filter(a => scannable(a.addresses));
    const hiddenInventory = allAssets.length - scanTargets.length;
    const signature = JSON.stringify({ org, assets, ranges, tools, preset });
    const key = useMemo(() => crypto.randomUUID(), [signature, locale()]);
    const launch = useMutation({ mutationFn: () => api.postIdempotent<Scan>('/api/v1/scans', { language:language(), organization_id: org, asset_ids: assets, range_ids: ranges, tools, preset } satisfies ScanInput, key), onSuccess: async (s) => { await Promise.all(['scans', 'home', 'assessments'].map(k => qc.invalidateQueries({ queryKey: [k] }))); navigate(`/simulations/${s.id}`); } });
    const changeOrg = (value: string) => { const next = new URLSearchParams(params); if (value)
        next.set('org', value);
    else
        next.delete('org'); setParams(next); setAssets([]); setAgent(''); setRanges([]); setValidation(''); };
    const toggle = (value: string, items: string[], set: (v: string[]) => void) => set(items.includes(value) ? items.filter(v => v !== value) : [...items, value]);
    return <section className="page"><Link to="/simulations">{textUI("m_ee68966f9267")}</Link><div className="page__head"><div><h1 className="page__title">{textUI("m_de2b814aeb96")}</h1><p className="page__sub">{textUI("m_a04d69dfb42d")}</p></div></div>{formatUI(org && <ScanControls org={org} compact/>)}<form className="dialog__form" onSubmit={e => { e.preventDefault(); if (!org || assets.length + ranges.length === 0 || tools.length === 0) {
        setValidation(displayLabel('Choose an organisation, at least one authorized target and at least one tool.'));
        return;
    } ; setValidation(''); launch.mutate(); }}><fieldset className="fieldset"><legend>{textUI("m_f8025f721b46")}</legend><OrganizationFilter value={org} onChange={changeOrg} required/></fieldset><fieldset className="fieldset" disabled={!org}><legend>{textUI("m_255ab0fc6f97")}</legend>{formatUI(!org && <p role="status" className="notice">{textUI("m_scan_pick_org_hint")}</p>)}<h2>{textUI("m_bd12731d7bc9")}</h2>{formatUI(list.error && <p role="alert">{formatUI(describeError(list.error).message)}</p>)}<ul className="stack-list">{formatUI(scanTargets.map(a => <li key={a.id}><label className="check"><input type="checkbox" disabled={!a.testing_authorized || a.is_demo} checked={assets.includes(a.id)} onChange={() => toggle(a.id, assets, setAssets)}/><span className="check__box" aria-hidden="true"/><span>{formatUI(a.name)} · {formatUI(a.testing_authorized ? textUI("m_67f542ba6d8f") : textUI("m_4e35c67de055"))} · {formatUI(a.addresses.filter(d => d.scan_approved !== false).map(d => d.value).join(', ') || textUI("m_2d0f6f621c7a"))}</span></label></li>))}</ul>{formatUI(hiddenInventory > 0 && <p role="note" className="small">{textUI("m_scan_hidden_inventory", [hiddenInventory])}</p>)}{formatUI(list.hasNextPage && <button className="btn btn--secondary btn--auto" type="button" disabled={list.isFetchingNextPage} onClick={() => list.fetchNextPage()}>{textUI("m_1f4e681dd0c4")}</button>)}{formatUI(org && scanTargets.length === 0 && !list.isPending && <p>{richUI("m_3c5206be1c43", [<Link to={`/organizations/${org}?tab=assets&new=1`}>{textUI("m_a8f2d03d958c")}</Link>])}</p>)}<h2>{textUI("m_8c10602211d2")}</h2><p>{textUI("m_017a3a447309")}</p>{formatUI(can(me, 'agent.read') && <label className="select">{richUI("m_d81c657d9af0", [<select value={agent} onChange={e => { setAgent(e.target.value); setRanges([]); }}><option value="">{textUI("m_632e8f465445")}</option>{formatUI(localAgents.map(a => <option key={a.id} value={a.id}>{formatUI(a.instance_name || a.asset_name)} · {formatUI(a.os)}</option>))}</select>])}</label>)}{formatUI(agent && <AgentNetworks org={org} agent={agent} selected={ranges} onSelect={setRanges}/>)}</fieldset><fieldset className="fieldset"><legend>{textUI("m_3ccd0a2166aa")}</legend>{formatUI(['nmap', 'nuclei'].map(tool => <label className="check" key={tool}><input type="checkbox" checked={tools.includes(tool)} onChange={() => toggle(tool, tools, setTools)}/><span className="check__box" aria-hidden="true"/><span>{formatUI(tool === 'nmap' ? textUI("m_1c11562a7589") : textUI("m_3b4a80522815"))}</span></label>))}<label className="select">{richUI("m_ca3826f56e88", [<select value={preset} onChange={e => setPreset(e.target.value as 'quick' | 'standard')}><option value="quick">{textUI("m_9f49fadb6a86")}</option><option value="standard">{textUI("m_ef6691545d2c")}</option></select>])}</label><p className="field__hint">{textUI("m_5e786bd6d8e9")}</p></fieldset>{formatUI(validation && <p className="alert" role="alert">{formatUI(validation)}</p>)}{formatUI(launch.error && <p className="alert" role="alert">{formatUI(describeError(launch.error).message)}</p>)}<button type="submit" className="btn btn--primary btn--auto" disabled={control.data?.allowed_now === false || launch.isPending || !can(me, 'assessment.run') || !can(me, 'simulation.run')}>{formatUI(launch.isPending ? textUI("m_bbe5fc3b9ef3") : textUI("m_68ac7da5dfff"))}</button></form></section>;
}
export function ScanDetailPage() {
    useLocale();
    const { id = '' } = useParams();
    const { data: me } = useMe();
    const qc = useQueryClient();
    const q = useQuery({ queryKey: ['scan', id], queryFn: () => api.get<Scan>(`/api/v1/scans/${id}`), refetchInterval: query => query.state.data?.status === 'running' ? 1500 : false });
    const [added, setAdded] = useState<Record<string, string>>({});
    const cancel = useMutation({ mutationFn: () => api.post(`/api/v1/scans/${id}/cancel`), onSuccess: () => q.refetch() });
    const add = useMutation({ mutationFn: (job: string) => api.post<{
            id: string;
        }>(`/api/v1/scan-jobs/${job}/asset`, {}), onSuccess: async (asset, job) => { setAdded(current => ({ ...current, [job]: asset.id })); await qc.invalidateQueries({ queryKey: ['assets'] }); } });
    if (q.isPending)
        return <p role="status">{textUI("m_8eda2014e161")}</p>;
    if (!q.data)
        return <p className="alert" role="alert">{formatUI(describeError(q.error).message)}</p>;
    const s = q.data;
    const groups = new Map<string, NonNullable<Scan['jobs']>>();
    for (const job of s.jobs ?? []) {
        const group = job.asset_id ?? job.name;
        groups.set(group, [...(groups.get(group) ?? []), job]);
    }
    return <section className="page"><Link to="/simulations">{textUI("m_ee68966f9267")}</Link><div className="page__head"><div><p className="eyebrow">{richUI("m_d573647405a4", [formatUI(s.options.preset)])}</p><h1 className="page__title">{formatUI(s.name)}</h1><p className="page__sub" role="status">{formatUI(s.status)} · {formatUI(new Date(s.created_at).toLocaleString(locale()))}</p><p>{textUI("m_795608b006f9")}</p></div>{formatUI(s.status === 'running' && can(me, 'assessment.run') && can(me, 'simulation.run') && <button className="btn btn--danger btn--auto" disabled={cancel.isPending} onClick={() => cancel.mutate()}>{textUI("m_298360e0b166")}</button>)}</div><div className="row"><Link to={`/organizations/${s.organization_id}?tab=scans`}>{textUI("m_350be3643ce7")}</Link>{formatUI(can(me, 'finding.read') && <Link to={`/findings?org=${s.organization_id}`}>{textUI("m_3bd3a989b0f0")}</Link>)}{formatUI(s.status !== 'running' && can(me, 'report.read') && <Link to={`/reports?assessment_id=${id}`}>{textUI("m_e81de18ed78e")}</Link>)}</div>{formatUI((cancel.error || add.error) && <p className="alert" role="alert">{formatUI(describeError(cancel.error || add.error).message)}</p>)}<>{formatUI(s.results_expired_at && <p role="status" className="alert">{textUI("m_6e77528b9b49")}</p>)}</><ScanComparison comparison={s.comparison_details} pending={s.status === 'running'}/>{formatUI(Array.from(groups).map(([key, jobs]) => <article className="org-card" key={key}><h2>{formatUI(jobs[0]!.name)}</h2>{formatUI(jobs.map(j => <section key={j.id} className="org-card"><h3>{formatUI(j.tool === 'nmap' ? textUI("m_8ecf2262720c") : textUI("m_fc5796e8828b"))} · {formatUI(j.status)}</h3><p className="mono">{formatUI(j.target || textUI("m_6789458750ba"))}</p>{formatUI(j.error && <p className="alert">{formatUI(j.error)}</p>)}{formatUI(j.status === 'finished' && <p>{formatUI(j.tool === 'nmap' ? textUI("m_2ba781aa59ab") : textUI("m_fc367d4af9bc"))}{formatUI(j.findings.length === 0 ? ' No findings recorded for this coverage.' : '')}</p>)}<ul className="stack-list">{formatUI(j.findings.map(f => <li key={f.id}><strong>{formatUI(f.title)}</strong><p>{formatUI(f.severity)} · {formatUI(f.type)} · {formatUI(f.value)}</p>{formatUI(Object.entries(f.evidence ?? {}).map(([name, value]) => <p key={name} className="muted small">{formatUI(name)}: {formatUI(String(value))}</p>))}</li>))}</ul>{formatUI(j.range_id && j.status === 'finished' && can(me, 'asset.write') && can(me, 'organization.write') && <p>{formatUI(added[j.id] ? <Link to={`/assets/${added[j.id]}`}>{textUI("m_97c0d733eb49")}</Link> : <button className="btn btn--secondary btn--auto" disabled={add.isPending} onClick={() => add.mutate(j.id)}>{textUI("m_dbe1efc343e2")}</button>)}<span className="muted small">{textUI("m_4da4598a9f0b")}</span></p>)}</section>))}</article>))}</section>;
}

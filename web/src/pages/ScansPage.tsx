
import {language} from '../i18n/runtime';
import { t as textUI, tr as richUI, useLocale, locale, fv as formatUI } from "../i18n/runtime";
import { ScanComparison } from '../components/ScanComparison';
import { DefenseResult } from '../components/DefenseResult';
import { Plus } from 'lucide-react';
import { StatusBadge, scanStatusLabel } from '../components/sims/StatusBadge';
import { useAllOrganizations } from '../components/orgs/useOrgData';
import { durationMs, flattenFindings, formatDuration, jobCounts } from '../lib/scanView';
import { SeverityBadge } from '../components/RunBadges';
import './sims.css';
import './newscan.css';
import { ScanControls, useScanControl } from '../components/ScanControls';
import { useMemo, useState } from 'react';
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/client';
import { useAssets, useMe } from '../api/hooks';
import { useScans, type Scan, type ScanInput } from '../api/scans';
import { can } from '../api/types';
import { OrganizationFilter } from '../components/OrganizationFilter';

import { describeError } from '../components/ErrorNote';
import { LegacySimulations } from './LegacySimulations';

const units = () => ({ h: textUI("m_sim_u_h"), m: textUI("m_sim_u_m"), s: textUI("m_sim_u_s") });
const startedAt = (iso: string) => { const d = new Date(iso); return Number.isNaN(d.getTime()) ? '—' : d.toLocaleString(locale(), { dateStyle: 'medium', timeStyle: 'short' }); };
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
    const enabled = can(me, 'assessment.read') && can(me, 'simulation.read') && can(me, 'asset.read') && can(me, 'organization.read');
    const q = useScans(org, cursor, enabled);
    const orgs = useAllOrganizations();
    const orgName = (id: string) => orgs.data?.find((o) => o.id === id)?.name ?? organizationName ?? id.slice(0, 8);
    const Heading = embedded ? 'h2' : 'h1';
    const items = q.data?.items ?? [];
    const canRun = can(me, 'assessment.run') && can(me, 'simulation.run');
    const setOrg = (v: string) => { const next = new URLSearchParams(params); if (v) next.set('org', v); else next.delete('org'); setParams(next); setCursor(''); };
    return <section className="page sims" aria-labelledby="scans-title">
      <div className="page__head"><div>
        <p className="eyebrow">{textUI("m_sim_eyebrow")}</p>
        <Heading id="scans-title" className="page__title">{formatUI(embedded ? textUI("m_d128917a9540") : textUI("m_078a864ea0f1"))}</Heading>
        <p className="page__sub">{textUI("m_bb8add5b0686")}</p>
      </div>{formatUI(canRun && <Link className="btn btn--primary btn--auto sims__new" to={`/simulations/new${org ? `?org=${encodeURIComponent(org)}` : ''}`}><Plus size={14} aria-hidden="true"/>{textUI("m_sim_new")}</Link>)}</div>
      <div className="toolbar"><OrganizationFilter value={org} name={organizationName} locked={!!organizationID} onChange={setOrg}/></div>
      {!me || (q.isPending && q.fetchStatus !== 'idle') ? <ul className="sims__list" aria-busy="true" aria-label={textUI("m_e0b80da465d6")}>{[0, 1, 2, 3].map((i) => <li key={i} className="sims__row sims__row--skeleton"><span className="skeleton"/></li>)}</ul>
        : q.error ? <div className="state state--error" role="alert"><h2>{textUI("m_sim_err_t")}</h2><p>{formatUI(describeError(q.error).message)}</p><button type="button" className="btn btn--secondary btn--auto" onClick={() => q.refetch()}>{textUI("m_sim_retry")}</button></div>
        : items.length === 0 && q.data ? <div className="state" role="status"><h2>{formatUI(org ? textUI("m_sim_none_t") : textUI("m_3c2ec23816e3"))}</h2><p>{formatUI(org ? textUI("m_sim_none_b") : textUI("m_9ec336bb3650"))}</p>{org && !organizationID ? <button type="button" className="btn btn--secondary btn--auto" onClick={() => setOrg('')}>{textUI("m_sim_clear_filter")}</button> : canRun && <Link className="btn btn--secondary btn--auto" to="/simulations/new">{textUI("m_sim_new")}</Link>}</div>
        : !q.data ? null
        : <div className="sims__table" role="table" aria-label={textUI("m_sim_list")}>
          <div className="sims__row sims__head" role="row"><span role="columnheader">{textUI("m_sim_col_scan")}</span><span role="columnheader">{textUI("m_sim_col_status")}</span><span role="columnheader">{textUI("m_sim_col_tools")}</span><span role="columnheader">{textUI("m_sim_col_preset")}</span><span role="columnheader">{textUI("m_sim_col_started")}</span><span role="columnheader">{textUI("m_sim_col_duration")}</span></div>
          {items.map((s) => <div key={s.id} className="sims__row" role="row">
            <span role="cell" className="sims__scan"><Link to={`/simulations/${s.id}`}>{formatUI(embedded ? s.name : orgName(s.organization_id))}</Link><code className="mono" title={s.id}>{s.id.slice(0, 13)}</code></span>
            <span role="cell"><StatusBadge status={s.status}/></span>
            <span role="cell" className="mono">{formatUI((s.options.tools ?? ['nmap','nuclei','hackbot']).join(' + '))}</span>
            <span role="cell">{formatUI(s.options.preset)}</span>
            <span role="cell" className="mono">{startedAt(s.created_at)}</span>
            <span role="cell" className="mono">{formatDuration(durationMs(s), units())}</span>
          </div>)}
        </div>}
      <div className="row"><button className="btn btn--secondary btn--auto" disabled={!cursor} onClick={() => setCursor('')}>{textUI("m_sim_newer")}</button><button className="btn btn--secondary btn--auto" disabled={!q.data?.next_cursor} onClick={() => setCursor(q.data!.next_cursor!)}>{textUI("m_sim_older")}</button></div>
      {formatUI(!embedded && <details><summary>{textUI("m_04ab162cdd89")}</summary><p>{textUI("m_4310bb792a7d")}</p><LegacySimulations /></details>)}
    </section>;
}
export function NewScanPage() {
    useLocale();
    const { data: me } = useMe();
    const [params, setParams] = useSearchParams();
    const navigate = useNavigate();
    const qc = useQueryClient();
    const org = params.get('org') ?? '';
    const [assets, setAssets] = useState<string[]>([]);
    const [validation, setValidation] = useState('');
    const control = useScanControl(org || undefined, !!org);
    const list = useAssets('', '', org, !!org);
    const allAssets = list.data?.pages.flatMap(p => p.items) ?? [];
    const scanTargets = allAssets.filter(a => a.addresses.some(d => d.kind==='url' && d.scan_approved!==false && /^https?:\/\//i.test(d.value)));
    const hiddenInventory = allAssets.length - scanTargets.length;
    const signature = JSON.stringify({ org, assets });
    const key = useMemo(() => crypto.randomUUID(), [signature, locale()]);
    const launch = useMutation({ mutationFn: () => api.postIdempotent<Scan>('/api/v1/scans', { language:language(), organization_id: org, asset_ids: assets } satisfies ScanInput, key), onSuccess: async (s) => { await Promise.all(['scans', 'home', 'assessments'].map(k => qc.invalidateQueries({ queryKey: [k] }))); navigate(`/simulations/${s.id}`); } });
    const changeOrg = (value: string) => { const next = new URLSearchParams(params); if (value)
        next.set('org', value);
    else
        next.delete('org'); setParams(next); setAssets([]); setValidation(''); };
    const toggle = (value: string, items: string[], set: (v: string[]) => void) => set(items.includes(value) ? items.filter(v => v !== value) : [...items, value]);
    return <section className="page newscan"><Link className="sims__back" to="/simulations">{textUI("m_ee68966f9267")}</Link><div className="page__head"><div><h1 className="page__title">{textUI("hackbot_run")}</h1><p className="page__sub">{textUI("hackbot_intro")}</p></div></div>{formatUI(org && <ScanControls org={org} compact/>)}<form className="dialog__form" onSubmit={e => { e.preventDefault(); if (!org || assets.length === 0) {
        setValidation(textUI("hackbot_choose_target"));
        return;
    } ; setValidation(''); launch.mutate(); }}><fieldset className="fieldset"><legend>{textUI("m_f8025f721b46")}</legend><OrganizationFilter value={org} onChange={changeOrg} required/></fieldset><fieldset className="fieldset" disabled={!org}><legend>{textUI("m_255ab0fc6f97")}</legend>{formatUI(!org && <p role="status" className="notice">{textUI("m_scan_pick_org_hint")}</p>)}<h2>{textUI("m_bd12731d7bc9")}</h2>{formatUI(list.error && <p role="alert">{formatUI(describeError(list.error).message)}</p>)}<ul className="stack-list">{formatUI(scanTargets.map(a => <li key={a.id}><label className="check"><input type="checkbox" disabled={!a.testing_authorized || a.is_demo} checked={assets.includes(a.id)} onChange={() => toggle(a.id, assets, setAssets)}/><span className="check__box" aria-hidden="true"/><span>{formatUI(a.name)} · {formatUI(a.testing_authorized ? textUI("m_67f542ba6d8f") : textUI("m_4e35c67de055"))} · {formatUI(a.addresses.filter(d => d.scan_approved !== false).map(d => d.value).join(', ') || textUI("m_2d0f6f621c7a"))}</span></label></li>))}</ul>{formatUI(hiddenInventory > 0 && <p role="note" className="small">{textUI("m_scan_hidden_inventory", [hiddenInventory])}</p>)}{formatUI(list.hasNextPage && <button className="btn btn--secondary btn--auto" type="button" disabled={list.isFetchingNextPage} onClick={() => list.fetchNextPage()}>{textUI("m_1f4e681dd0c4")}</button>)}{formatUI(org && scanTargets.length === 0 && !list.isPending && <p>{richUI("m_3c5206be1c43", [<Link to={`/organizations/${org}?tab=assets&new=1`}>{textUI("m_a8f2d03d958c")}</Link>])}</p>)}</fieldset><p className="field__hint">{textUI("hackbot_intro")}</p>{formatUI(validation && <p className="alert" role="alert">{formatUI(validation)}</p>)}{formatUI(launch.error && <p className="alert" role="alert">{formatUI(describeError(launch.error).message)}</p>)}<button type="submit" className="btn btn--primary btn--auto" disabled={control.data?.allowed_now === false || launch.isPending || !can(me, 'assessment.run') || !can(me, 'simulation.run')}>{formatUI(launch.isPending ? textUI("m_bbe5fc3b9ef3") : textUI("hackbot_run"))}</button><Link className="btn btn--secondary btn--auto" to="/simulations">{textUI("m_19766ed6ccb2")}</Link></form></section>;
}
export function ScanDetailPage() {
    useLocale();
    const { id = '' } = useParams();
    const { data: me } = useMe();
    const qc = useQueryClient();
    const q = useQuery({ queryKey: ['scan', id], queryFn: () => api.get<Scan>(`/api/v1/scans/${id}`), refetchInterval: query => query.state.data?.status === 'running' || ['queued','rendering'].includes(query.state.data?.report_status ?? '') ? 1500 : false });
    const orgQ = useQuery({ queryKey: ['organization-name', q.data?.organization_id], enabled: !!q.data && can(me, 'organization.read'), queryFn: () => api.get<{ name: string }>(`/api/v1/organizations/${q.data!.organization_id}`), retry: false });
    const [added, setAdded] = useState<Record<string, string>>({});
    const cancel = useMutation({ mutationFn: () => api.post(`/api/v1/scans/${id}/cancel`), onSuccess: () => q.refetch() });
    const add = useMutation({ mutationFn: (job: string) => api.post<{
            id: string;
        }>(`/api/v1/scan-jobs/${job}/asset`, {}), onSuccess: async (asset, job) => { setAdded(current => ({ ...current, [job]: asset.id })); await qc.invalidateQueries({ queryKey: ['assets'] }); } });
    if (q.isPending)
        return <p role="status">{textUI("m_sim_loading")}</p>;
    if (!q.data)
        return <p className="alert" role="alert">{formatUI(describeError(q.error).message)}</p>;
    const s = q.data;
    const jobs = s.jobs ?? [];
    const counts = jobCounts(jobs);
    const flat = flattenFindings(jobs);
    const running = s.status === 'running';
    const cmp = s.comparison_details?.totals;
    const groups = new Map<string, NonNullable<Scan['jobs']>>();
    for (const job of jobs) {
        const group = job.asset_id ?? job.name;
        groups.set(group, [...(groups.get(group) ?? []), job]);
    }
    const stamp = (iso: string | null) => { if (!iso) return '—'; const d = new Date(iso); return Number.isNaN(d.getTime()) ? '—' : d.toLocaleString(locale(), { dateStyle: 'medium', timeStyle: 'medium' }); };
    return <section className="page sims" aria-labelledby="scan-title">
      <Link className="sims__back" to="/simulations">{textUI("m_sim_back")}</Link>
      <div className="page__head"><div>
        <p className="eyebrow">{richUI("m_d573647405a4", [formatUI(s.options.preset)])}</p>
        <h1 id="scan-title" className="page__title">{formatUI(orgQ.data?.name ?? s.name)}</h1>
        <p className="page__sub" role="status"><StatusBadge status={s.status}/> <span className="mono">{stamp(s.created_at)}</span></p>
        <p className="sims__caveat">{textUI("m_sim_caveat")}</p>
      </div>
      <div className="sims__actions">
        {formatUI(running && can(me, 'assessment.run') && can(me, 'simulation.run') && <button className="btn btn--danger btn--auto" disabled={cancel.isPending} onClick={() => cancel.mutate()}>{textUI("m_sim_cancel")}</button>)}
        <Link className="btn btn--secondary btn--auto" to={`/organizations/${s.organization_id}?tab=scans`}>{textUI("m_sim_open_org")}</Link>
        {formatUI(can(me, 'finding.read') && <Link className="btn btn--secondary btn--auto" to={`/findings?org=${s.organization_id}`}>{textUI("m_sim_org_findings")}</Link>)}
        {formatUI(!running && can(me, 'report.read') && <Link className="btn btn--secondary btn--auto" to={`/reports?assessment_id=${id}`}>{textUI("m_sim_scan_report")}</Link>)}
      </div></div>
      {formatUI((cancel.error || add.error) && <p className="alert" role="alert">{formatUI(describeError(cancel.error || add.error).message)}</p>)}
      {formatUI(s.results_expired_at && <p role="status" className="alert">{textUI("m_6e77528b9b49")}</p>)}
      <div className="sims__stats">
        <div className="stat" role="group" aria-label={textUI("m_sim_d_status")}><span className="stat__label">{textUI("m_sim_d_status")}</span><span className="sims__stat-v"><StatusBadge status={s.status}/></span><span className="stat__sub">{formatUI(s.ended_at ? textUI("m_sim_ended_at", [stamp(s.ended_at)]) : textUI("m_sim_not_ended"))}</span></div>
        <div className="stat" role="group" aria-label={textUI("m_sim_d_duration")}><span className="stat__label">{textUI("m_sim_d_duration")}</span><span className="stat__value mono">{formatDuration(durationMs(s), units())}</span></div>
        <div className="stat" role="group" aria-label={textUI("m_sim_d_jobs")}><span className="stat__label">{textUI("m_sim_d_jobs")}</span><span className="stat__value mono">{counts.finished} / {counts.total}</span><span className="stat__sub">{textUI("m_sim_jobs_sub", [counts.running, counts.waiting, counts.failed])}</span></div>
        <div className="stat" role="group" aria-label={textUI("m_sim_d_findings")}><span className="stat__label">{textUI("m_sim_d_findings")}</span><span className="stat__value mono">{flat.length}</span>{formatUI(cmp ? <span className="stat__sub">{formatUI(`${cmp.new} ${textUI("m_18fdd549b2ed").toLowerCase()} · ${cmp.persistent} ${textUI("m_6bf85034bd50").toLowerCase()}`)}</span> : running ? <span className="stat__sub">{textUI("m_sim_findings_sub")}</span> : null)}</div>
      </div>
      <section className="sims__panel" aria-labelledby="tl-h"><h2 id="tl-h" className="sims__h">{textUI("m_sim_timeline")}</h2>
        <ol className="sims__timeline">
          <li><b>{textUI("m_sim_t_created")}</b><span className="mono">{stamp(s.created_at)}</span></li>
          {jobs.map((j) => <li key={j.id}><b>{formatUI(j.tool)} · {formatUI(j.status)}</b><span className="muted small">{formatUI(j.target || '—')}</span></li>)}
          <li><b>{formatUI(s.ended_at ? textUI("m_sim_t_ended") : textUI("m_sim_t_waiting"))}</b><span className="mono">{stamp(s.ended_at)}</span></li>
        </ol></section>
      {s.summary && <section className="sims__panel" aria-label={textUI("hackbot_summary")}><h2 className="sims__h">{textUI("hackbot_summary")}</h2><p>{s.summary}</p></section>}
      {s.report_id && can(me,'report.read') && <section className="sims__panel"><h2 className="sims__h">{textUI("hackbot_report")}</h2>{s.report_status==='ready' ? <iframe className="pentest-pdf" title={textUI("hackbot_report")} src={`/api/v1/reports/${s.report_id}/export/pdf?preview=1`}/> : <p role="status">{s.report_status==='failed' ? textUI("hackbot_pdf_failed") : textUI("hackbot_pdf_pending")}</p>}<Link to={`/reports/${s.report_id}`}>{textUI("hackbot_open_report")}</Link></section>}
      {jobs.some((j) => j.defense) && <section className="sims__panel" aria-labelledby="def-h"><h2 id="def-h" className="sims__h">{textUI("m_def_scan_h")}</h2>{jobs.filter((j) => j.defense).map((j) => <div key={j.id}><h3 className="ed-h4">{formatUI(j.name)}</h3><DefenseResult summary={j.defense!}/></div>)}</section>}
      <ScanComparison comparison={s.comparison_details} pending={running}/>
      <section className="sims__panel" aria-labelledby="jobs-h"><h2 id="jobs-h" className="sims__h">{textUI("m_sim_jobs_h")} <span className="muted">{groups.size}</span></h2>
        {formatUI(Array.from(groups).map(([key, gj]) => <div key={key} className="sims__group"><h3>{formatUI(gj[0]!.name)}</h3>
          <div className="sims__jobs" role="table" aria-label={gj[0]!.name}>
            <div className="sims__jrow sims__head" role="row"><span role="columnheader">{textUI("m_sim_job_tool")}</span><span role="columnheader">{textUI("m_sim_job_target")}</span><span role="columnheader">{textUI("m_sim_job_status")}</span><span role="columnheader">{textUI("m_sim_job_findings")}</span><span role="columnheader">{textUI("m_sim_job_note")}</span></div>
            {gj.map((j) => <div key={j.id} className="sims__jrow" role="row">
              <span role="cell" className="mono">{formatUI(j.tool==='hackbot'?'Hackbot Lite':j.tool==='defensebot'?'DefenseBot':j.tool)}</span>
              <span role="cell" className="mono" title={j.target}>{formatUI(j.target || textUI("m_6789458750ba"))}</span>
              <span role="cell"><StatusBadge status={j.status}/></span>
              <span role="cell" className="mono">{j.findings.length}</span>
              <span role="cell" className="sims__note">{j.tool==='defensebot' && j.defense && <span className="muted small">{textUI("m_def_job_note", [j.defense.findings, j.defense.known_exploited])}</span>}{j.tool==='hackbot' && <span className="pipeline-phase muted small">{j.phase || (j.status==='waiting' ? textUI('hackbot_wait_scanners') : scanStatusLabel(j.status))}{j.disposition ? ` · ${j.disposition}` : ''}</span>}<span>{formatUI(j.error || '—')}</span>{formatUI(j.range_id && j.status === 'finished' && can(me, 'asset.write') && can(me, 'organization.write') && <> {added[j.id] ? <Link to={`/assets/${added[j.id]}`}>{textUI("m_97c0d733eb49")}</Link> : <button className="btn btn--secondary btn--sm" disabled={add.isPending} onClick={() => add.mutate(j.id)}>{textUI("m_dbe1efc343e2")}</button>} <span className="muted small">{textUI("m_4da4598a9f0b")}</span></>)}</span>
            </div>)}
          </div></div>))}
      </section>
      <section className="sims__panel" aria-labelledby="fnd-h"><h2 id="fnd-h" className="sims__h">{textUI("m_sim_produced")} <span className="muted">{flat.length}</span></h2>
        {flat.length === 0 ? <p className="muted">{textUI("m_sim_no_findings")}</p> : <div className="sims__finds" role="table" aria-label={textUI("m_sim_produced")}>
          <div className="sims__frow sims__head" role="row"><span role="columnheader">{textUI("m_sim_f_sev")}</span><span role="columnheader">{textUI("m_sim_f_type")}</span><span role="columnheader">{textUI("m_sim_f_title")}</span><span role="columnheader">{textUI("m_sim_f_value")}</span><span role="columnheader">{textUI("m_sim_f_tool")}</span></div>
          {flat.map((f) => <div key={f.id} className="sims__frow" role="row">
            <span role="cell"><SeverityBadge severity={f.severity as 'info'}/></span><span role="cell" className="mono">{formatUI(f.type)}</span>
            <span role="cell"><strong>{formatUI(f.title)}</strong></span><span role="cell" className="mono" title={f.value}>{formatUI(f.value)}</span><span role="cell" className="mono">{formatUI(f.tool)}</span>
          </div>)}
        </div>}
      </section>
    </section>;
}

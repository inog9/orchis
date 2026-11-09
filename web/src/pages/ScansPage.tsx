import { t as textUI, tr as richUI, useLocale, locale, fv as formatUI } from "../i18n/runtime";
import { ScanComparison } from '../components/ScanComparison';
import { DefenseResult } from '../components/DefenseResult';
import { Plus, Search } from 'lucide-react';
import { StatusBadge, scanStatusLabel } from '../components/sims/StatusBadge';
import { useAllOrganizations } from '../components/orgs/useOrgData';
import { durationMs, flattenFindings, formatDuration, jobCounts } from '../lib/scanView';
import { engineLabel, engineRows, engineSetLabel } from '../lib/engines';
import { defenseWhyIncomplete } from '../components/DefenseNotes';
import { SeverityBadge } from '../components/RunBadges';
import './sims.css';
import './newscan.css';
import { ScanControls, useScanControl } from '../components/ScanControls';
import { useState } from 'react';
import { Link, useParams, useSearchParams } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/client';
import { useMe } from '../api/hooks';
import { useScans, type Scan } from '../api/scans';
import { can } from '../api/types';
import { OrganizationFilter } from '../components/OrganizationFilter';

import { describeError } from '../components/ErrorNote';
import { LegacySimulations } from './LegacySimulations';
import { advancedEnabled } from '../components/navigation';

export { NewScanPage } from './NewScanPage';

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
    const [search, setSearch] = useState('');
    const org = organizationID ?? params.get('org') ?? '';
    const enabled = can(me, 'assessment.read') && can(me, 'simulation.read') && can(me, 'asset.read') && can(me, 'organization.read');
    const q = useScans(org, cursor, enabled);
    const orgs = useAllOrganizations();
    const orgName = (id: string) => orgs.data?.find((o) => o.id === id)?.name ?? organizationName ?? id.slice(0, 8);
    const Heading = embedded ? 'h2' : 'h1';
    const searchable = useQuery({ queryKey: ['scan-search', org], enabled: enabled && !!search.trim(), queryFn: async () => {
        const all: Scan[] = []; let next = ''; const seen = new Set<string>();
        do { const page = await api.get<{items: Scan[]; next_cursor: string | null}>(`/api/v1/scans?org=${encodeURIComponent(org)}&cursor=${encodeURIComponent(next)}`); all.push(...page.items); next = page.next_cursor ?? ''; if (seen.has(next)) break; seen.add(next); } while (next);
        return all;
    }, refetchInterval: 10000 });
    const needle = search.trim().toLowerCase();
    const items = (needle ? searchable.data ?? [] : q.data?.items ?? []).filter(s => !needle || [s.name, s.id, orgName(s.organization_id), s.status, scanStatusLabel(s.status), engineSetLabel(s.options.tools)].join(' ').toLowerCase().includes(needle));
    const canRun = can(me, 'assessment.run') && can(me, 'simulation.run');
    const setOrg = (v: string) => { const next = new URLSearchParams(params); if (v) next.set('org', v); else next.delete('org'); setParams(next); setCursor(''); };
    return <section className="page sims" aria-labelledby="scans-title">
      <div className="page__head"><div>
        <p className="eyebrow">{textUI("m_sim_eyebrow")}</p>
        <Heading id="scans-title" className="page__title">{formatUI(embedded ? textUI("m_d128917a9540") : textUI("flow_nav_scans"))}</Heading>
        <p className="page__sub">{textUI("m_bb8add5b0686")}</p>
      </div>{formatUI(canRun && <Link className="btn btn--primary btn--auto sims__new" to={`/simulations/new${org ? `?org=${encodeURIComponent(org)}` : ''}`}><Plus size={14} aria-hidden="true"/>{textUI("m_sim_new")}</Link>)}</div>
      <div className="toolbar"><OrganizationFilter value={org} name={organizationName} locked={!!organizationID} onChange={setOrg}/><label className="search-input"><Search size={15} aria-hidden="true"/><input type="search" aria-label={textUI('ui_search_scans')} placeholder={textUI('ui_search_scans')} value={search} onChange={e => setSearch(e.target.value)}/></label></div>
      {!me || (needle ? searchable.isPending : q.isPending && q.fetchStatus !== 'idle') ? <ul className="sims__list" aria-busy="true" aria-label={textUI("m_e0b80da465d6")}>{[0, 1, 2, 3].map((i) => <li key={i} className="sims__row sims__row--skeleton"><span className="skeleton"/></li>)}</ul>
        : (needle ? searchable.error : q.error) ? <div className="state state--error" role="alert"><h2>{textUI("m_sim_err_t")}</h2><p>{formatUI(describeError(needle ? searchable.error : q.error).message)}</p><button type="button" className="btn btn--secondary btn--auto" onClick={() => needle ? searchable.refetch() : q.refetch()}>{textUI("m_sim_retry")}</button></div>
        : needle && items.length === 0 ? <div className="state" role="status"><h2>{textUI('ui_no_search_results')}</h2><button type="button" className="btn btn--secondary btn--auto" onClick={() => setSearch('')}>{textUI('m_fnd_clear')}</button></div>
        : items.length === 0 && q.data ? <div className="state" role="status"><h2>{formatUI(org ? textUI("m_sim_none_t") : textUI("m_3c2ec23816e3"))}</h2><p>{formatUI(org ? textUI("m_sim_none_b") : textUI("m_9ec336bb3650"))}</p>{canRun && <p className="field__hint">{textUI("flow_hint_scan")}</p>}{org && !organizationID ? <button type="button" className="btn btn--secondary btn--auto" onClick={() => setOrg('')}>{textUI("m_sim_clear_filter")}</button> : canRun && <Link className="btn btn--secondary btn--auto" to="/simulations/new">{textUI("m_sim_new")}</Link>}</div>
        : !q.data ? null
        : <div className="sims__table" role="table" aria-label={textUI("m_sim_list")}>
          <div className="sims__row sims__head" role="row"><span role="columnheader">{textUI("m_sim_col_scan")}</span><span role="columnheader">{textUI("m_sim_col_status")}</span><span role="columnheader">{textUI("m_sim_col_tools")}</span><span role="columnheader">{textUI("m_sim_col_preset")}</span><span role="columnheader">{textUI("m_sim_col_started")}</span><span role="columnheader">{textUI("m_sim_col_duration")}</span></div>
          {items.map((s) => <div key={s.id} className="sims__row" role="row">
            <span role="cell" className="sims__scan"><Link to={`/simulations/${s.id}`}>{formatUI(embedded ? s.name : orgName(s.organization_id))}</Link><code className="mono" title={s.id}>{s.id.slice(0, 13)}</code></span>
            <span role="cell"><StatusBadge status={s.status}/></span>
            <span role="cell">{formatUI(engineSetLabel(s.options.tools))}</span>
            <span role="cell" className="mono">{textUI("flow_n_assets", [(s.options.asset_ids ?? []).length])}</span>
            
            <span role="cell" className="mono">{startedAt(s.created_at)}</span>
            <span role="cell" className="mono">{formatDuration(durationMs(s), units())}</span>
          </div>)}
        </div>}
      <div className="row" hidden={!!needle}><button className="btn btn--secondary btn--auto" disabled={!cursor} onClick={() => setCursor('')}>{textUI("m_sim_newer")}</button><button className="btn btn--secondary btn--auto" disabled={!q.data?.next_cursor} onClick={() => setCursor(q.data!.next_cursor!)}>{textUI("m_sim_older")}</button></div>
      {formatUI(!embedded && advancedEnabled(me) && <details><summary>{textUI("m_04ab162cdd89")}</summary><p>{textUI("m_4310bb792a7d")}</p><LegacySimulations /></details>)}
    </section>;
}
export function ScanDetailPage() {
    useLocale();
    const { id = '' } = useParams();
    const { data: me } = useMe();
    const qc = useQueryClient();
    const q = useQuery({ queryKey: ['scan', id], queryFn: () => api.get<Scan>(`/api/v1/scans/${id}`), refetchInterval: query => query.state.data?.status === 'running' || ['queued','rendering'].includes(query.state.data?.report_status ?? '') ? 1500 : false });
    const orgQ = useQuery({ queryKey: ['organization-name', q.data?.organization_id], enabled: !!q.data && can(me, 'organization.read'), queryFn: () => api.get<{ name: string }>(`/api/v1/organizations/${q.data!.organization_id}`), retry: false });
    const cancel = useMutation({ mutationFn: () => api.post(`/api/v1/scans/${id}/cancel`), onSuccess: () => q.refetch() });
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
        <p className="eyebrow">{richUI("m_d573647405a4", [formatUI(engineSetLabel(s.options.tools))])}</p>
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
      {formatUI(cancel.error && <p className="alert" role="alert">{formatUI(describeError(cancel.error).message)}</p>)}
      {formatUI(s.results_expired_at && <p role="status" className="alert">{textUI("m_6e77528b9b49")}</p>)}
      <div className="sims__stats">
        <div className="stat" role="group" aria-label={textUI("m_sim_d_status")}><span className="stat__label">{textUI("m_sim_d_status")}</span><span className="sims__stat-v"><StatusBadge status={s.status}/></span><span className="stat__sub">{formatUI(s.ended_at ? textUI("m_sim_ended_at", [stamp(s.ended_at)]) : textUI("m_sim_not_ended"))}</span></div>
        <div className="stat" role="group" aria-label={textUI("m_sim_d_duration")}><span className="stat__label">{textUI("m_sim_d_duration")}</span><span className="stat__value">{formatDuration(durationMs(s), units())}</span></div>
        <div className="stat" role="group" aria-label={textUI("m_sim_d_jobs")}><span className="stat__label">{textUI("m_sim_d_jobs")}</span><span className="stat__value">{counts.finished} / {counts.total}</span><span className="stat__sub">{textUI("m_sim_jobs_sub", [counts.running, counts.waiting, counts.failed])}</span></div>
        <div className="stat" role="group" aria-label={textUI("m_sim_d_findings")}><span className="stat__label">{textUI("m_sim_d_findings")}</span><span className="stat__value">{flat.length}</span>{formatUI(cmp ? <span className="stat__sub">{formatUI(`${cmp.new} ${textUI("m_18fdd549b2ed").toLowerCase()} · ${cmp.persistent} ${textUI("m_6bf85034bd50").toLowerCase()}`)}</span> : running ? <span className="stat__sub">{textUI("m_sim_findings_sub")}</span> : null)}</div>
      </div>
      <section className="sims__panel" aria-labelledby="engines-h"><h2 id="engines-h" className="sims__h">{textUI("flow_wiz_s2")}</h2>
        {Array.from(groups).map(([key, gj]) => <div key={key} className="sims__group"><h3>{formatUI(gj[0]!.name)}</h3>
          <div className="sims__jobs" role="table" aria-label={gj[0]!.name}>
            <div className="sims__jrow sims__head" role="row"><span role="columnheader">{textUI("m_sim_job_tool")}</span><span role="columnheader">{textUI("flow_chk_latest")}</span><span role="columnheader">{textUI("m_sim_job_status")}</span><span role="columnheader">{textUI("m_sim_job_findings")}</span><span role="columnheader">{textUI("m_sim_job_note")}</span></div>
            {engineRows(gj).map((e) => <div key={e.engine} className="sims__jrow" role="row">
              <span role="cell"><strong>{e.engine}</strong></span>
              <span role="cell">{e.stage || '—'}</span>
              <span role="cell"><StatusBadge status={e.status}/></span>
              <span role="cell" className="mono">{e.findings}</span>
              <span role="cell" className="sims__note"><span>{formatUI(e.error || (e.engine === 'orchis-defense-agent' && e.status === 'incomplete' && e.jobs[0]?.defense ? defenseWhyIncomplete(e.jobs[0].defense.notes) : '—'))}</span>{e.engine === 'orchis-defense-agent' && e.jobs[0]?.defense && <span className="muted small">{textUI("m_def_job_note", [e.jobs[0].defense.findings, e.jobs[0].defense.known_exploited])}</span>}</span>
            </div>)}
          </div></div>)}
      </section>
      {s.summary && <section className="sims__panel" aria-label={textUI("hackbot_summary")}><h2 className="sims__h">{textUI("hackbot_summary")}</h2><p>{s.summary}</p></section>}
      {s.report_id && can(me,'report.read') && <section className="sims__panel"><h2 className="sims__h">{textUI("hackbot_report")}</h2>{s.report_status==='ready' ? <iframe className="pentest-pdf" title={textUI("hackbot_report")} src={`/api/v1/reports/${s.report_id}/export/pdf?preview=1`}/> : <p role="status">{s.report_status==='failed' ? textUI("hackbot_pdf_failed") : textUI("hackbot_pdf_pending")}</p>}<Link to={`/reports/${s.report_id}`}>{textUI("hackbot_open_report")}</Link></section>}
      {jobs.some((j) => j.defense) && <section className="sims__panel" aria-labelledby="def-h"><h2 id="def-h" className="sims__h">{textUI("m_def_scan_h")}</h2>{jobs.filter((j) => j.defense).map((j) => <div key={j.id}><h3 className="ed-h4">{formatUI(j.name)}</h3><DefenseResult summary={j.defense!}/></div>)}</section>}
      <ScanComparison comparison={s.comparison_details} pending={running}/>
      <section className="sims__panel" aria-labelledby="fnd-h"><h2 id="fnd-h" className="sims__h">{textUI("m_sim_produced")} <span className="muted">{flat.length}</span></h2>
        {flat.length === 0 ? <p className="muted">{textUI("m_sim_no_findings")}</p> : <div className="sims__finds" role="table" aria-label={textUI("m_sim_produced")}>
          <div className="sims__frow sims__head" role="row"><span role="columnheader">{textUI("m_sim_f_sev")}</span><span role="columnheader">{textUI("m_sim_f_type")}</span><span role="columnheader">{textUI("m_sim_f_title")}</span><span role="columnheader">{textUI("m_sim_f_value")}</span><span role="columnheader">{textUI("m_sim_f_tool")}</span></div>
          {flat.map((f) => <div key={f.id} className="sims__frow" role="row">
            <span role="cell"><SeverityBadge severity={f.severity as 'info'}/></span><span role="cell" className="mono">{formatUI(f.type)}</span>
            <span role="cell"><strong>{formatUI(f.title)}</strong></span><span role="cell" className="mono" title={f.value}>{formatUI(f.value)}</span><span role="cell">{engineLabel(f.tool)}</span>
          </div>)}
        </div>}
      </section>
    </section>;
}

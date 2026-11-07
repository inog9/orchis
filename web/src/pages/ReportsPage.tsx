import {displayLabel} from '../i18n/display';
import { t as textUI, tr as richUI, useLocale, locale, fv as formatUI } from "../i18n/runtime";
import { ScanComparison, type Comparison } from '../components/ScanComparison';
import { OrganizationFilter } from '../components/OrganizationFilter';
import { advancedEnabled } from '../components/navigation';
import { useState } from 'react';
import { Link, useParams, useSearchParams } from 'react-router-dom';
import { useMutation, useQuery } from '@tanstack/react-query';
import { api } from '../api/client';
import { useMe } from '../api/hooks';
import { can } from '../api/types';
import { describeError } from '../components/ErrorNote';
import { StatusBadge } from '../components/sims/StatusBadge';
import './reports.css';
type Posture = {
    collected_at: string;
    source: string;
    osquery_version?: string;
    attributes: Record<string, unknown>;
    findings: {
        severity: string;
        title: string;
        query: string;
    }[];
    ports?: {
        both: number[];
        scan_only: number[];
        host_only: number[];
        note: string;
    };
};
type Finding = {
    id: string;
    type: string;
    value: string;
    title: string;
    severity: string;
    evidence: Record<string, unknown>;
    mitre?: {
        id: string;
        name: string;
        basis: string;
        reason: string;
    }[];
};
type Snapshot = {
 summary?:string;
    is_demo?: boolean;
    label?: string;
    comparison_details?: Comparison;
    scan_id?: string;
    organization: string;
    profile: string;
    status: string;
    risk: string;
    by_severity: Record<string, number>;
    comparison: string;
    remediation: string;
    endpoint_summary?: {
        assets_total: number;
        assets_with_data: number;
        by_severity: Record<string, number>;
    };
    assets: {
        id: string;
        name: string;
        status: string;
        skip_reason: string;
        endpoint?: Posture;
        findings: Finding[];
        methods: {
            mitre?: string[];
            action: string;
            version: number;
            target: string;
            outcome: string;
            error: string;
        }[];
    }[];
};
type Report = {
    id: string;
    organization_id: string;
    assessment_id: string;
    status: string;
    error: string;
    version: number;
    created_at: string;
    snapshot: Snapshot;
};
export function ReportsPage({ organizationID, organizationName }: {
    organizationID?: string;
    organizationName?: string;
} = {}) {
    useLocale();
    const { data: me } = useMe();
    const [params, setParams] = useSearchParams();
    const [cursor, setCursor] = useState('');
    const org = organizationID ?? params.get('org') ?? params.get('organization_id') ?? '';
    const filters = new URLSearchParams();
    if (org)
        filters.set('organization_id', org);
    if (!organizationID && params.get('assessment_id'))
        filters.set('assessment_id', params.get('assessment_id')!);
    const filter = filters.toString();
    const Heading = organizationID ? 'h2' : 'h1';
    const q = useQuery({ queryKey: ['reports', filter, cursor], queryFn: () => api.get<{
            items: Report[];
            next_cursor: string | null;
        }>(`/api/v1/reports?${filter}&cursor=${encodeURIComponent(cursor)}`), refetchInterval: 3000 });
    const items = q.data?.items ?? [];
    const setOrg = (value: string) => { const next = new URLSearchParams(params); next.delete('organization_id'); next.delete('assessment_id'); if (value) next.set('org', value); else next.delete('org'); setParams(next); setCursor(''); };
    const when = (iso: string) => { const d = new Date(iso); return Number.isNaN(d.getTime()) ? '—' : d.toLocaleString(locale(), { dateStyle: 'medium', timeStyle: 'short' }); };
    return <section className="page rep" aria-labelledby="reports-title"><div className="page__head"><div><p className="eyebrow">{textUI("m_3de3ebca8ffb")}</p><Heading id="reports-title" className="page__title">{textUI("m_dacca3cba3f3")}</Heading><p className="page__sub">{textUI("m_3d289a285283")}</p></div></div>
      {formatUI(can(me, 'organization.read') && <OrganizationFilter value={org} name={organizationName} locked={!!organizationID} onChange={setOrg}/>)}
      {q.isPending ? <ul className="rep__list" aria-busy="true" aria-label={textUI("m_4ef739f597a4")}>{[0, 1, 2, 3].map((i) => <li key={i} className="rep__row rep__row--skeleton"><span className="skeleton"/></li>)}</ul>
        : q.error ? <div className="state state--error" role="alert"><h2>{textUI("m_rep_err_t")}</h2><p>{formatUI(describeError(q.error).message)}</p><button type="button" className="btn btn--secondary btn--auto" onClick={() => q.refetch()}>{textUI("m_sim_retry")}</button></div>
        : items.length === 0 ? <div className="state" role="status"><h2>{formatUI(org || params.get('assessment_id') ? textUI("m_sim_none_t") : textUI("m_c142ec36ff6e"))}</h2><p>{formatUI(org || params.get('assessment_id') ? textUI("m_rep_none_b") : textUI("m_62a1665b61e6"))}</p>{org && !organizationID ? <button type="button" className="btn btn--secondary btn--auto" onClick={() => setOrg('')}>{textUI("m_sim_clear_filter")}</button> : can(me, 'organization.read') && <Link className="btn btn--secondary btn--auto" to="/organizations">{textUI("m_d78c44ecc1be")}</Link>}</div>
        : <div className="rep__table" role="table" aria-label={textUI("m_dacca3cba3f3")}>
          <div className="rep__row rep__head" role="row"><span role="columnheader">{textUI("m_rep_col_org")}</span><span role="columnheader">{textUI("m_rep_col_profile")}</span><span role="columnheader">{textUI("m_rep_col_risk")}</span><span role="columnheader">{textUI("m_rep_col_source")}</span><span role="columnheader">{textUI("m_rep_col_created")}</span><span role="columnheader">{textUI("m_rep_col_pdf")}</span><span role="columnheader">{textUI("m_rep_col_actions")}</span></div>
          {items.map((r) => <div key={r.id} className="rep__row" role="row">
            <span role="cell" className="rep__org"><Link to={`/reports/${r.id}`}>{formatUI(r.snapshot.organization)}</Link><code className="mono" title={r.id}>{r.id.slice(0, 13)}</code></span>
            <span role="cell">{richUI("m_4c31e6bba070", [formatUI(displayLabel(r.snapshot.profile)), formatUI(r.version)])}</span>
            <span role="cell">{formatUI(displayLabel(r.snapshot.risk))}</span>
            <span role="cell" className="rep__source">{formatUI(r.snapshot.scan_id ? textUI("m_0fe8524d1d55") : textUI("m_e1cf71e6c627"))}</span>
            <span role="cell" className="mono">{when(r.created_at)}</span>
            <span role="cell"><StatusBadge status={r.status}/></span>
            <span role="cell" className="rep__acts"><Link to={`/reports/${r.id}`}>{textUI("m_rep_open")}</Link>{r.status === 'ready' && <a href={`/api/v1/reports/${r.id}/export/pdf`} download>PDF</a>}</span>
          </div>)}
        </div>}
      <div className="row"><button className="btn btn--secondary btn--auto" disabled={!cursor} onClick={() => setCursor('')}>{textUI("m_0bdbb750609c")}</button><button className="btn btn--secondary btn--auto" disabled={!q.data?.next_cursor} onClick={() => setCursor(q.data!.next_cursor!)}>{textUI("m_8b99e4a0a612")}</button></div></section>;
}
export function ReportDetailPage() {
    useLocale();
    const { id = '' } = useParams();
    const { data: me } = useMe();
    const q = useQuery({ queryKey: ['report', id], queryFn: () => api.get<Report>(`/api/v1/reports/${id}`), refetchInterval: query => query.state.data?.status === 'ready' ? false : 3000 });
    const retry = useMutation({ mutationFn: () => api.post(`/api/v1/reports/${id}/retry`), onSuccess: () => q.refetch() });
    if (q.isPending)
        return <p role="status">{textUI("m_3b475fd2f1e9")}</p>;
    if (!q.data)
        return <p role="alert">{formatUI(describeError(q.error).message)}</p>;
    const r = q.data;
    const s = r.snapshot;
    const skipped = s.assets.filter(a => a.status === 'skipped' || !!a.skip_reason).length;
    const tested = s.assets.filter(a => a.status === 'finished' && !a.skip_reason && a.methods.length > 0 && a.methods.every(m => m.outcome === 'success')).length;
    const incomplete = s.assets.length - skipped - tested;
    return <section className="page rep rep--detail"><Link className="rep__back" to="/reports">{textUI("m_c912bea5a73c")}</Link><div className="page__head"><div><p className="eyebrow">{richUI("m_a8a0511b45e0", [formatUI(s.scan_id ? textUI("m_0fe8524d1d55") : textUI("m_e1cf71e6c627")), formatUI(r.version)])}</p><h1 className="page__title">{formatUI(s.organization)}</h1><p className="page__sub">{formatUI(new Date(r.created_at).toLocaleString(locale()))} · {formatUI(displayLabel(s.profile))} <StatusBadge status={r.status}/></p></div><div className="row">{formatUI(r.status === 'ready' && <a className="btn btn--secondary btn--auto" href={`/api/v1/reports/${id}/export/pdf?preview=1`} target="_blank" rel="noreferrer">{textUI("m_995032218e76")}</a>)}{formatUI(['pdf', 'json'].map(format => <a className="btn btn--secondary btn--auto" key={format} href={`/api/v1/reports/${id}/export/${format}`} download aria-disabled={format === 'pdf' && r.status !== 'ready'} onClick={e => {
                if (format === 'pdf' && r.status !== 'ready')
                    e.preventDefault();
            }}>{richUI("m_465e0e809a40", [formatUI(format.toUpperCase())])}</a>))}</div></div>{formatUI(s.is_demo && <div className="notice" role="status">{formatUI(s.label || 'DEMO / SYNTHETIC DATA - not a real assessment.')}</div>)}{formatUI(r.status !== 'ready' && <div className={`notice rep__notice rep__notice--${r.status === 'failed' ? 'bad' : 'live'}`} role={r.status === 'failed' ? 'alert' : 'status'}><p>{richUI("m_43e72fde1467", [formatUI(displayLabel(r.status)), formatUI(r.error)])}</p>{formatUI(r.status === 'failed' && can(me, 'report.generate') && <button className="btn btn--secondary btn--auto" disabled={retry.isPending} onClick={() => retry.mutate()}>{textUI("m_2cdede71ecc0")}</button>)}</div>)}{formatUI(retry.error && <p role="alert">{formatUI(describeError(retry.error).message)}</p>)}<article className="org-card"><p className="eyebrow">{textUI("m_dbb54f6b8942")}</p><h2>{formatUI(displayLabel(s.risk))}</h2><p>{richUI("m_eb8d9e1bbe4c", [formatUI(s.assets.length), formatUI(tested), formatUI(skipped), formatUI(incomplete)])}</p><p>{textUI("m_12ae8050bc3e")}</p><h3>{textUI("m_22fe13ca6a2a")}</h3><div className="severity-strip">{formatUI(['critical', 'high', 'medium', 'low', 'info'].map(severity => <p key={severity}><strong>{formatUI(s.by_severity[severity] ?? 0)}</strong> {formatUI(displayLabel(severity))}</p>))}</div>{formatUI(s.endpoint_summary && <section aria-label={textUI("m_28c9aeab86c9")}><h2>{textUI("m_28c9aeab86c9")}</h2><p>{richUI("m_b692f52058c0", [formatUI(s.endpoint_summary.assets_with_data), formatUI(s.endpoint_summary.assets_total)])}</p><p className="muted small">{textUI("m_c0897afc8d4f")}</p><div className="ed-scroll" role="region" aria-label={textUI("m_d7800ca39df8")} tabIndex={0}><table className="org-table ed-report"><thead><tr><th>{textUI("m_80d298c9f240")}</th><th>{textUI("m_6725e7bbcd28")}</th><th>{textUI("m_7f7ecd155f4a")}</th><th>{textUI("m_c2584dfa6903")}</th></tr></thead><tbody>{formatUI(s.assets.filter(a => a.endpoint).map(a => { const e = a.endpoint!; const at = e.attributes; return <tr key={a.id}><td>{formatUI(a.name)}</td><td>{formatUI(String(at.os_name ?? ''))} {formatUI(String(at.os_version ?? ''))}{formatUI(at.kernel ? `, kernel ${String(at.kernel)}` : '')}</td><td>{formatUI(e.findings.length === 0 ? textUI("m_dc937b598926") : <ul className="stack-list">{formatUI(e.findings.slice(0, 8).map((f, i) => <li key={i}>{formatUI(`${displayLabel(f.severity)}: ${f.title}`)}</li>))}{formatUI(e.findings.length > 8 && <li className="muted small">{formatUI(textUI("m_cca2a7065289", [e.findings.length - 8]))}</li>)}</ul>)}</td><td>{formatUI(e.ports ? textUI("m_3bca60964638", [e.ports.both.join(', ') || 'none', e.ports.scan_only.join(', ') || 'none', e.ports.host_only.join(', ') || 'none']) : textUI("m_c7bcc01273bc"))}</td></tr>; }))}</tbody></table></div><p className="muted small">{textUI("m_303b527be639")}</p></section>)}{s.summary && <section className="org-card" aria-label={textUI("hackbot_summary")}><h2>{textUI("hackbot_summary")}</h2><p>{s.summary}</p></section>}{s.summary && r.status==='ready' && <iframe className="pentest-pdf" title={textUI("hackbot_report")} src={`/api/v1/reports/${id}/export/pdf?preview=1`}/>} {(locale() === 'en-US' || !s.comparison_details) && <p>{formatUI(displayLabel(s.comparison))}</p>}<ScanComparison comparison={s.comparison_details}/><div className="report-relations">{formatUI(can(me, 'organization.read') && <Link to={`/organizations/${r.organization_id}`}>{textUI("m_81acc0a4d1ff")}</Link>)}{formatUI(can(me, s.scan_id ? 'simulation.read' : 'assessment.read') && <Link to={s.scan_id ? `/simulations/${s.scan_id}` : `/assessments/${r.assessment_id}`}>{formatUI(s.scan_id ? textUI("m_b5e09d8b7d1e") : textUI("m_7cb1f270ab5b"))}</Link>)}</div></article><h2>{textUI("m_36fb48e6a063")}</h2>{formatUI(s.assets.map(a => <article className="org-card" key={a.id}><h2>{formatUI(a.name)}</h2><p>{formatUI(displayLabel(a.status))}{formatUI(a.skip_reason ? ` · ${a.skip_reason}` : '')}</p>{formatUI(a.methods.some(m => m.outcome !== 'success') && <div className="notice" role="status"><p>{textUI("m_3f3e8a4c4961")}</p><ul>{formatUI(a.methods.filter(m => m.outcome !== 'success').map((m, i) => <li key={i}>{formatUI(m.action)}: {formatUI(m.error || m.outcome || textUI("m_0192502471c4"))}</li>))}</ul></div>)}{formatUI(a.findings.map(f => <section className="org-card" key={f.id}><p className="eyebrow">{formatUI(displayLabel(f.severity))} · {formatUI(displayLabel(f.type))}</p><h3>{formatUI(f.title)}</h3><p className="mono">{formatUI(f.value)}</p>{formatUI((f.mitre ?? []).map((t) => <p key={t.id} className="muted small">{richUI("m_446d40d8d310", [formatUI(t.basis), formatUI(t.id), formatUI(t.name), formatUI(t.reason)])}</p>))}{formatUI(Object.entries(f.evidence ?? {}).map(([key, value]) => <p key={key}><strong>{formatUI(key)}:</strong> {formatUI(String(value))}</p>))}</section>))}{formatUI(a.findings.length === 0 && <p>{textUI("m_12686d00c818")}</p>)}{formatUI(a.methods.length > 0 && <details><summary>{textUI("m_c9d16df2aa67")}</summary>{formatUI(a.methods.map((m, i) => <p key={i}>{richUI("m_2be959524133", [formatUI(m.action), formatUI(m.version), formatUI(m.target), formatUI(displayLabel(m.outcome)), formatUI(m.error), formatUI(m.mitre?.length ? ` · MITRE ATT&CK: ${m.mitre.join(', ')}` : '')])}</p>))}</details>)}</article>))}<article className="org-card"><h2>{textUI("m_5a0ee3452091")}</h2><p>{formatUI(displayLabel(s.remediation))}</p>{formatUI(advancedEnabled(me) && can(me, 'ai.read') && <Link to={`/organizations/${r.organization_id}/ai`}>{textUI("m_9f7b9389a0d3")}</Link>)}</article></section>;
}

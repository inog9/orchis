import {displayLabel} from '../i18n/display';
import { t as textUI, tr as richUI, useLocale, locale, fv as formatUI, language } from "../i18n/runtime";
import { ScanComparison, type Comparison } from '../components/ScanComparison';
import { OrganizationFilter } from '../components/OrganizationFilter';
import { advancedEnabled } from '../components/navigation';
import { MetricCard, PanelCard, SeverityBar, SeverityPill, StateTag } from '../components/pen/primitives';
import { Fragment, useState } from 'react';
import { ChevronDown } from 'lucide-react';
import { Link, useParams, useSearchParams } from 'react-router-dom';
import { useMutation, useQuery } from '@tanstack/react-query';
import { api } from '../api/client';
import { useMe } from '../api/hooks';
import { can } from '../api/types';
import { describeError } from '../components/ErrorNote';
import { StatusBadge } from '../components/sims/StatusBadge';
import '../components/pen/pen.css';
import './reports.css';

// Marker data demo pada snapshot adalah teks tetap dari server (internal/demo/snapshots.go).
// UI menampilkannya dalam bahasa antarmuka; teks lain dari snapshot ditampilkan apa adanya.
const DEMO_TEXT_ID: Record<string, string> = {
    'DEMO / SYNTHETIC - not a real assessment. Do not use for real security decisions.': 'DEMO / SINTETIS - bukan assessment nyata. Jangan dipakai untuk keputusan keamanan sungguhan.',
    'Synthetic demo data - risk not evaluated.': 'Data demo sintetis - risiko tidak dinilai.',
    'Synthetic example only. Verify actual observations before any remediation.': 'Contoh sintetis saja. Verifikasi observasi nyata sebelum remediasi.',
    'Comparison unavailable: first synthetic snapshot.': 'Perbandingan tidak tersedia: snapshot sintetis pertama.',
};
function demoText(value: string | undefined): string {
    if (!value) return '';
    return language() === 'id' ? (DEMO_TEXT_ID[value] ?? value) : value;
}
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
    // Evidence bebas bentuknya (internal/reporting/export.go): paket, CVE, EPSS, dst. Bisa tidak ada.
    evidence?: Record<string, unknown>;
    mitre?: {
        id: string;
        name: string;
        basis: string;
        reason: string;
    }[];
};
type Snapshot = {
 summary?:string;
    language?: string;
    schema_version?: string;
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
        defense?: Record<string, unknown>;
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

// ===== Detail laporan (redesain pen) =====

// Jumlah temuan yang ditampilkan per aset sebelum tombol "Muat lagi"; larang render ratusan baris sekaligus.
const FINDINGS_PAGE = 20;
// Kolom evidence yang sudah tampil di baris utama; sisanya masuk panel detail yang bisa dibuka.
const EVIDENCE_INLINE = new Set(['package', 'installed_version', 'fixed_version', 'fix_status', 'data_as_of']);

// Ambil nilai evidence sebagai teks; evidence bisa hilang sama sekali atau berisi angka/boolean.
function evString(evidence: Record<string, unknown> | undefined, key: string): string {
    const value = evidence?.[key];
    if (value === null || value === undefined || value === '') return '';
    return String(value);
}

// Label fix_status dari server (internal/defensebot/run.go); nilai di luar daftar tampil apa adanya.
function fixLabel(status: string): string {
    switch (status) {
        case 'fix_available': return textUI("pen_rpt_fix_available");
        case 'no_fix_yet': return textUI("pen_rpt_fix_none");
        case 'fix_requires_subscription': return textUI("pen_rpt_fix_subscription");
        case 'fix_pending_reboot': return textUI("pen_rpt_fix_reboot");
        default: return status;
    }
}

// data_as_of biasanya tanggal; bila tidak bisa diurai tampilkan teks mentahnya.
function dayText(iso: string): string {
    if (!iso) return '';
    const d = new Date(iso);
    return Number.isNaN(d.getTime()) ? iso : d.toLocaleDateString(locale(), { dateStyle: 'medium' });
}

export function ReportDetailPage() {
    useLocale();
    const { id = '' } = useParams();
    const { data: me } = useMe();
    // Batas tampil temuan per aset (kunci id aset) dan baris bukti yang terbuka (kunci id temuan).
    const [preview, setPreview] = useState(false);
    const [limits, setLimits] = useState<Record<string, number>>({});
    const [openRows, setOpenRows] = useState<Record<string, boolean>>({});
    const q = useQuery({ queryKey: ['report', id], queryFn: () => api.get<Report>(`/api/v1/reports/${id}`), refetchInterval: query => query.state.data?.status === 'ready' ? false : 3000 });
    const retry = useMutation({ mutationFn: () => api.post(`/api/v1/reports/${id}/retry`), onSuccess: () => q.refetch() });
    if (q.isPending)
        return <section className="page rep" aria-busy="true"><p role="status">{textUI("m_3b475fd2f1e9")}</p></section>;
    if (!q.data)
        return <section className="page rep"><div className="state state--error" role="alert"><p>{formatUI(describeError(q.error).message)}</p><button type="button" className="btn btn--secondary btn--auto" onClick={() => q.refetch()}>{textUI("m_sim_retry")}</button></div></section>;
    const r = q.data;
    const s = r.snapshot;
    const assets = s.assets ?? [];
    // Count the same snapshot findings rendered in the table, including port/service observations.
    // by_severity from older reports may count vulnerabilities only.
    const snapshotFindings = [...new Map(assets.flatMap(a => a.findings ?? []).map(f => [f.id, f])).values()];
    const sev: Record<string, number> = snapshotFindings.reduce<Record<string, number>>((counts, f) => { counts[f.severity] = (counts[f.severity] ?? 0) + 1; return counts; }, {});
    const severities = ['critical', 'high', 'medium', 'low', 'info'] as const;
    const findingsTotal = severities.reduce((acc, k) => acc + (sev[k] ?? 0), 0);
    const skipped = assets.filter(a => a.status === 'skipped' || !!a.skip_reason).length;
    const tested = assets.filter(a => a.status === 'finished' && !a.skip_reason && a.methods.length > 0 && a.methods.every(m => m.outcome === 'success')).length;
    const incomplete = assets.length - skipped - tested;
    const comparison = s.comparison_details;
    const totals = comparison?.totals;
    const comparedTotal = totals ? totals.new + totals.persistent + totals.resolved + totals.uncompared : 0;
    // Temuan berstatus "baru" menurut perbandingan scan, dipetakan dari finding_id baris comparison.
    const newFindingIds = new Set((comparison?.rows ?? []).filter(row => row.change === 'new').map(row => row.finding_id));
    const created = new Date(r.created_at);
    const createdText = Number.isNaN(created.getTime()) ? '' : created.toLocaleString(locale(), { dateStyle: 'medium', timeStyle: 'short' });
    const highest = severities.find(k => (sev[k] ?? 0) > 0);
    const needsCoverage = (incomplete > 0 || skipped > 0) && (!highest || highest === 'info' || highest === 'low');
    const priority = needsCoverage ? '' : highest === 'critical' ? 'P0' : highest === 'high' ? 'P1' : highest === 'medium' ? 'P2' : highest ? 'P3' : '';
    const priorityCaption = {P0: textUI('ui_priority_P0'), P1: textUI('ui_priority_P1'), P2: textUI('ui_priority_P2'), P3: textUI('ui_priority_P3')};
    return <section className="page rep rep--detail">
      <nav className="rep__crumbs" aria-label={textUI("pen_rpt_breadcrumb")}>
        {can(me, 'organization.read') ? <Link to={`/organizations/${r.organization_id}`}>{formatUI(s.organization)}</Link> : <span>{formatUI(s.organization)}</span>}
        <span aria-hidden="true">/</span>
        <span aria-current="page">{richUI("pen_rpt_crumb_current", [formatUI(r.version)])}</span>
      </nav>
      <div className="page__head">
        <div>
          <h1 className="page__title">{richUI("pen_rpt_title", [formatUI(s.organization), formatUI(r.version)])}</h1>
          <p className="page__sub">{richUI("pen_rpt_meta", [formatUI(displayLabel(r.status)), formatUI(createdText), formatUI(displayLabel(s.profile))])}</p>
        </div>
      </div>
      {s.is_demo && <div className="notice" role="status">{formatUI(demoText(s.label) || 'DEMO / SYNTHETIC DATA - not a real assessment.')}</div>}
      {r.status !== 'ready' && <div className={`notice rep__notice rep__notice--${r.status === 'failed' ? 'bad' : 'live'}`} role={r.status === 'failed' ? 'alert' : 'status'}>
        <p>{richUI("m_43e72fde1467", [formatUI(displayLabel(r.status)), formatUI(r.error)])}</p>
        {r.status === 'failed' && can(me, 'report.generate') && <button className="btn btn--secondary btn--auto" disabled={retry.isPending} onClick={() => retry.mutate()}>{textUI("m_2cdede71ecc0")}</button>}
      </div>}
      {retry.error && <p role="alert">{formatUI(describeError(retry.error).message)}</p>}
      <div className="rep__metrics">
        <MetricCard label={textUI('ui_highest_severity')} value={highest ? <span className={`rep__risk rep__risk--${highest}`}>{displayLabel(highest)}</span> : textUI(incomplete || skipped ? 'ui_not_assessed' : 'ui_no_findings')} caption={highest ? textUI('ui_observed_snapshot') : displayLabel(demoText(s.risk))}/>
        <MetricCard label={textUI('ui_remediation_priority')} value={priority || textUI(incomplete || skipped ? 'ui_complete_assessment' : 'ui_monitor')} caption={priority ? priorityCaption[priority] : textUI(incomplete || skipped ? 'ui_coverage_action' : 'ui_monitor_caption')}/>
        <MetricCard label={textUI('pen_rpt_metric_assets')} value={`${tested} / ${assets.length}`} caption={richUI('pen_rpt_assets_caption', [formatUI(tested), formatUI(skipped), formatUI(incomplete)])}/>
      </div>
      <section className="rep__severity" aria-label={textUI('ui_snapshot_severity')}>
        <div className="rep__section-heading"><h2>{textUI('ui_snapshot_severity')}</h2><span>{findingsTotal} {textUI('ui_findings_recorded')}</span></div>
        <div className="rep__severity-grid">{severities.map(k => <div key={k} className={`rep__severity-cell rep__severity-cell--${k}`}><span>{displayLabel(k)}</span><strong>{sev[k] ?? 0}</strong></div>)}</div>
        {!!findingsTotal && (incomplete > 0 || skipped > 0) && <p className="muted small">{textUI('ui_partial_observations')}</p>}
        {!findingsTotal && <p className="muted small">{textUI(incomplete || skipped ? 'ui_empty_incomplete' : 'ui_empty_complete')}</p>}
      </section>
      {s.endpoint_summary && <PanelCard title={textUI("m_28c9aeab86c9")}>
        <p>{richUI("m_b692f52058c0", [formatUI(s.endpoint_summary.assets_with_data), formatUI(s.endpoint_summary.assets_total)])}</p>
        <SeverityBar bySeverity={s.endpoint_summary.by_severity ?? {}}/>
        <div className="ed-scroll" role="region" aria-label={textUI("m_d7800ca39df8")} tabIndex={0}>
          <table className="org-table ed-report">
            <thead><tr><th scope="col">{textUI("m_80d298c9f240")}</th><th scope="col">{textUI("m_6725e7bbcd28")}</th><th scope="col">{textUI("m_7f7ecd155f4a")}</th><th scope="col">{textUI("m_c2584dfa6903")}</th></tr></thead>
            <tbody>{assets.filter(a => a.endpoint).map(a => { const e = a.endpoint!; const at = e.attributes; return <tr key={a.id}>
              <th scope="row">{formatUI(a.name)}</th>
              <td>{formatUI(String(at.os_name ?? ''))} {formatUI(String(at.os_version ?? ''))}{formatUI(at.kernel ? `, kernel ${String(at.kernel)}` : '')}</td>
              <td>{formatUI(e.findings.length === 0 ? textUI("m_dc937b598926") : <ul className="stack-list">{formatUI(e.findings.slice(0, 8).map((f, i) => <li key={i}>{formatUI(`${displayLabel(f.severity)}: ${f.title}`)}</li>))}{formatUI(e.findings.length > 8 && <li className="muted small">{formatUI(textUI("m_cca2a7065289", [e.findings.length - 8]))}</li>)}</ul>)}</td>
              <td>{formatUI(e.ports ? textUI("m_3bca60964638", [e.ports.both.join(', ') || 'none', e.ports.scan_only.join(', ') || 'none', e.ports.host_only.join(', ') || 'none']) : textUI("m_c7bcc01273bc"))}</td>
            </tr>; })}</tbody>
          </table>
        </div>
        <p className="muted small">{textUI("m_c0897afc8d4f")}</p>
        <p className="muted small">{textUI("m_303b527be639")}</p>
      </PanelCard>}
      {comparison && <details className="rep__comparison"><summary>{textUI('ui_compare_previous')}</summary><ScanComparison comparison={comparison}/></details>}
      {s.summary && <PanelCard title={textUI("hackbot_summary")}><p>{s.summary}</p></PanelCard>}
      <PanelCard title={textUI("pen_rpt_findings_title")}>
        <p className="muted small">{textUI("pen_rpt_findings_desc")}</p>
        {/* Satu tabel, dikelompokkan per aset lewat tbody + th scope="rowgroup" agar pembaca layar tetap paham konteks. */}
        <table className="rep__fnd">
          <thead><tr>
            <th scope="col">{textUI("pen_rpt_fnd_col_severity")}</th>
            <th scope="col">{textUI("pen_rpt_fnd_col_finding")}</th>
            <th scope="col">{textUI("pen_rpt_fnd_col_package")}</th>
            <th scope="col">{textUI("pen_rpt_fnd_col_fix")}</th>
            <th scope="col">{textUI("pen_rpt_fnd_col_updated")}</th>
            <th scope="col"><span className="sr-only">{textUI("pen_rpt_fnd_evidence")}</span></th>
          </tr></thead>
          {assets.map(a => {
            const rank: Record<string, number> = {critical: 0, high: 1, medium: 2, low: 3, info: 4};
            const findings = [...(a.findings ?? [])].sort((x,y) => (rank[x.severity] ?? 5) - (rank[y.severity] ?? 5));
            const shown = Math.min(limits[a.id] ?? FINDINGS_PAGE, findings.length);
            const failedMethods = a.methods.filter(m => m.outcome !== 'success');
            const criticalCount = findings.filter(f => f.severity === 'critical').length;
            const highCount = findings.filter(f => f.severity === 'high').length;
            return <tbody key={a.id} className="rep__fgroup">
              <tr className="rep__fgroup-row"><th colSpan={6} scope="rowgroup">
                <div className="rep__fgroup-head">
                  <span className="rep__monogram" aria-hidden="true">{(a.name.trim()[0] ?? '?').toUpperCase()}</span>
                  <div className="rep__fgroup-name">
                    <h3>{formatUI(a.name)}</h3>
                    <p className="muted small">{formatUI(displayLabel(a.status))}{formatUI(a.skip_reason ? ` · ${a.skip_reason}` : '')}</p>
                  </div>
                  {criticalCount > 0 && <span className="rep__fmini rep__fmini--critical" title={displayLabel('critical')}>{formatUI(criticalCount)}</span>}
                  {highCount > 0 && <span className="rep__fmini rep__fmini--high" title={displayLabel('high')}>{formatUI(highCount)}</span>}
                  <span className="rep__fgroup-count">{richUI("pen_rpt_fnd_count", [formatUI(findings.length)])}</span>
                  {can(me, 'asset.read') && <Link className="rep__fgroup-open" to={`/assets/${a.id}`}>{textUI("pen_rpt_open_asset")}</Link>}
                </div>
              </th></tr>
              {failedMethods.length > 0 && <tr className="rep__fmeta"><td colSpan={6}><div className="notice" role="status"><p>{textUI("m_3f3e8a4c4961")}</p><ul>{failedMethods.map((m, i) => <li key={i}>{formatUI(m.action)}: {formatUI(m.error || m.outcome || textUI("m_0192502471c4"))}</li>)}</ul></div></td></tr>}
              {findings.length === 0 && <tr className="rep__fmeta"><td colSpan={6} className="muted small">{textUI("m_12686d00c818")}</td></tr>}
              {findings.slice(0, shown).map(f => {
                const pkg = evString(f.evidence, 'package');
                const installed = evString(f.evidence, 'installed_version');
                const fixed = evString(f.evidence, 'fixed_version');
                const packageText = pkg ? (fixed ? `${pkg} ${installed} → ${fixed}` : [pkg, installed].filter(Boolean).join(' ')) : f.value;
                const fixStatus = evString(f.evidence, 'fix_status');
                const asOf = evString(f.evidence, 'data_as_of');
                const expanded = !!openRows[f.id];
                const rest = Object.entries(f.evidence ?? {}).filter(([key]) => !EVIDENCE_INLINE.has(key));
                return <Fragment key={f.id}>
                  <tr className="rep__frow">
                    <td><SeverityPill severity={f.severity}>{formatUI(displayLabel(f.severity))}</SeverityPill><span className="rep__priority">{f.severity === 'critical' ? 'P0' : f.severity === 'high' ? 'P1' : f.severity === 'medium' ? 'P2' : 'P3'}</span></td>
                    <td>
                      <p className="rep__ftitle">{formatUI(f.title)}{formatUI(newFindingIds.has(f.id) && <StateTag tone="accent">{textUI("pen_rpt_fnd_new")}</StateTag>)}</p>
                      {pkg && <p className="mono rep__fvalue">{formatUI(f.value)}</p>}
                    </td>
                    <td className="mono">{formatUI(packageText)}</td>
                    <td>{formatUI(fixStatus ? fixLabel(fixStatus) : '-')}</td>
                    <td>{formatUI(asOf ? dayText(asOf) : '-')}</td>
                    <td className="rep__fexp">
                      <button type="button" className="rep__fexp-btn" aria-expanded={expanded} aria-label={textUI("pen_rpt_fnd_evidence")} onClick={() => setOpenRows(prev => ({ ...prev, [f.id]: !prev[f.id] }))}><ChevronDown size={14} aria-hidden="true"/></button>
                    </td>
                  </tr>
                  {expanded && <tr className="rep__fexp-row"><td colSpan={6}>
                    {(f.mitre ?? []).map(t => <p key={t.id} className="muted small">{richUI("m_446d40d8d310", [formatUI(t.basis), formatUI(t.id), formatUI(t.name), formatUI(t.reason)])}</p>)}
                    {rest.map(([key, value]) => <p key={key} className="rep__fev">{key === 'advisory_url' && typeof value === 'string' && value.startsWith('http') ? <a href={value} target="_blank" rel="noreferrer">{formatUI(key)}</a> : <><strong>{formatUI(key)}:</strong> {formatUI(String(value))}</>}</p>)}
                    {rest.length === 0 && (f.mitre ?? []).length === 0 && <p className="muted small">{textUI("pen_rpt_fnd_evidence")}: -</p>}
                  </td></tr>}
                </Fragment>;
              })}
              {findings.length > FINDINGS_PAGE && <tr className="rep__fload"><td colSpan={6}>
                <span className="muted small">{richUI("pen_rpt_fnd_shown", [formatUI(shown), formatUI(findings.length)])}</span>
                {shown < findings.length && <button type="button" className="btn btn--secondary btn--auto" onClick={() => setLimits(prev => ({ ...prev, [a.id]: (prev[a.id] ?? FINDINGS_PAGE) + FINDINGS_PAGE }))}>{textUI("pen_rpt_load_more")}</button>}
              </td></tr>}
              {a.methods.length > 0 && <tr className="rep__fmeta"><td colSpan={6}><details><summary>{textUI("m_c9d16df2aa67")}</summary>{a.methods.map((m, i) => <p key={i}>{richUI("m_2be959524133", [formatUI(m.action), formatUI(m.version), formatUI(m.target), formatUI(displayLabel(m.outcome)), formatUI(m.error), formatUI(m.mitre?.length ? ` · MITRE ATT&CK: ${m.mitre.join(', ')}` : '')])}</p>)}</details></td></tr>}
            </tbody>;
          })}
        </table>
      </PanelCard>
      <PanelCard title={textUI("m_5a0ee3452091")}>
        <p>{formatUI(displayLabel(demoText(s.remediation)))}</p>
        {advancedEnabled(me) && can(me, 'ai.read') && <Link to={`/organizations/${r.organization_id}/ai`}>{textUI("m_9f7b9389a0d3")}</Link>}
      </PanelCard>
      <section className="rep__pdf" aria-labelledby="pdf-title">
        <div className="rep__section-heading"><div><h2 id="pdf-title">{textUI('ui_report_document')}</h2><p className="muted small">{textUI('ui_report_document_caption')}</p></div><StatusBadge status={r.status}/></div>
        <div className="rep__actions">
          <a className="btn btn--secondary btn--auto" href={`/api/v1/reports/${id}/export/json`} download>{textUI('pen_rpt_action_json')}</a>
          {r.status === 'ready' ? <><a className="btn btn--primary btn--auto" href={`/api/v1/reports/${id}/export/pdf`} download>{textUI('pen_rpt_action_pdf')}</a><button type="button" className="btn btn--secondary btn--auto" aria-expanded={preview} onClick={() => setPreview(!preview)}>{textUI(preview ? 'ui_hide_preview' : 'pen_rpt_action_preview')}</button></> : <button className="btn btn--secondary btn--auto" disabled>{textUI('pen_rpt_action_pdf')}</button>}
        </div>
        {preview && r.status === 'ready' && <iframe className="pentest-pdf" title={textUI('hackbot_report')} src={`/api/v1/reports/${id}/export/pdf?preview=1`}/>}
      </section>
      <div className="report-relations">
        {can(me, 'organization.read') && <Link to={`/organizations/${r.organization_id}`}>{textUI("m_81acc0a4d1ff")}</Link>}
        {can(me, s.scan_id ? 'simulation.read' : 'assessment.read') && <Link to={s.scan_id ? `/simulations/${s.scan_id}` : `/assessments/${r.assessment_id}`}>{formatUI(s.scan_id ? textUI("m_b5e09d8b7d1e") : textUI("m_7cb1f270ab5b"))}</Link>}
      </div>
    </section>;
}

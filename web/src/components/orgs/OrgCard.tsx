import { ArrowRight, FileText, Play } from 'lucide-react';
import type { PointerEvent } from 'react';
import { Link } from 'react-router-dom';
import { t as textUI, useLocale, fv as formatUI } from '../../i18n/runtime';
import { relativeTime } from '../../lib/relativeTime';
import { healthOf, SEVERITIES, totalFindings, type Health, type Org, type OrgReport } from './useOrgData';

export type OrgMetrics = { assets?: number; agents?: number; online?: number };

const initialsOf = (name: string) => name.split(/\s+/).filter(Boolean).slice(0, 2).map((p) => p[0]).join('').toUpperCase() || '?';
const cap = (s: string) => (s ? s.charAt(0).toUpperCase() + s.slice(1) : s);
const HEALTH_LABEL = { attn: 'm_orgs_h_attn', some: 'm_orgs_h_some', ok: 'm_orgs_h_ok', none: 'm_orgs_h_none' } as const satisfies Record<Health, string>;

// Bilah segmen: tinggi tetap, lebar per tingkat sebanding jumlahnya. Angka ada di teks alternatif, bukan hanya warna.
export function SeverityBar({ by }: { by: Record<string, number> | undefined }) {
    useLocale();
    const parts = [...SEVERITIES, 'info' as const].map((s) => ({ s, n: by?.[s] ?? 0 }));
    const sum = parts.reduce((a, p) => a + p.n, 0);
    const label = sum === 0 ? textUI("m_orgs_sev_none") : parts.filter((p) => p.n).map((p) => `${p.s} ${p.n}`).join(', ');
    return (<div className="sevbar" role="img" aria-label={label} title={label}>
      {sum === 0 ? <span className="sevbar__seg sevbar__seg--empty" style={{ flex: 1 }}/> : parts.filter((p) => p.n).map((p) => <span key={p.s} className={`sevbar__seg sevbar__seg--${p.s}`} style={{ flex: p.n }}/>)}
    </div>);
}

export function OrgCard({ org, metrics, report, reportKnown, canReport, canRun }: {
    org: Org;
    metrics: OrgMetrics;
    report: OrgReport | undefined;
    reportKnown: boolean; // true bila laporan sudah dimuat (agar "Never scanned" tidak muncul saat masih memuat)
    canReport: boolean;
    canRun: boolean;
}) {
    useLocale();
    const health = healthOf(report);
    const open = totalFindings(report);
    // Sorotan mengikuti kursor di dalam kartu; dimatikan CSS saat gerak dikurangi.
    const move = (e: PointerEvent<HTMLElement>) => {
        const r = e.currentTarget.getBoundingClientRect();
        e.currentTarget.style.setProperty('--mx', `${e.clientX - r.left}px`);
        e.currentTarget.style.setProperty('--my', `${e.clientY - r.top}px`);
    };
    const last = report ? relativeTime(report.created_at) : '';
    return (<div className="oc" onPointerMove={move}>
      <div className="oc__top">
        <span className="oc__avatar" aria-hidden="true">{initialsOf(org.name)}</span>
        <div className="oc__id">
          <h3 className="oc__name"><Link className="oc__link" to={`/organizations/${org.id}`}>{formatUI(org.name)}</Link></h3>
          {org.industry && <span className="oc__chip">{formatUI(cap(org.industry))}</span>}
        </div>
      </div>
      <p className="oc__status">
        {canReport && reportKnown && <><span className={`oc__dot oc__dot--${health}`} aria-hidden="true"/><span className="oc__health">{textUI(HEALTH_LABEL[health])}</span><span className="oc__sep" aria-hidden="true">·</span></>}
        <span className="mono oc__last">{report ? textUI("m_orgs_last_scan", [last]) : canReport && reportKnown ? textUI("m_orgs_never") : '—'}</span>
      </p>
      <dl className="oc__metrics mono">
        <div><dt>{textUI("m_orgs_m_assets")}</dt><dd>{formatUI(metrics.assets ?? '—')}</dd></div>
        <div><dt>{textUI("m_orgs_m_agents")}</dt><dd>{metrics.agents === undefined ? '—' : textUI("m_orgs_m_online", [metrics.online ?? 0])}</dd></div>
        <div><dt>{textUI("m_orgs_m_findings")}</dt><dd>{reportKnown && report ? textUI("m_orgs_m_open", [open]) : '—'}</dd></div>
      </dl>
      <div className="oc__sev">{canReport && reportKnown && report && <SeverityBar by={report.bySeverity}/>}</div>
      <div className="oc__foot">
        <span className="oc__cta">{textUI("m_orgs_view")}<ArrowRight size={14} aria-hidden="true"/></span>
        <div className="oc__actions" role="group" aria-label={textUI("m_orgs_quick", [org.name])}>
          {canRun && <Link className="oc__act" to={`/simulations/new?org=${org.id}`}><Play size={12} aria-hidden="true"/>{textUI("m_orgs_run_check")}</Link>}
          {canReport && <Link className="oc__act" to={`/organizations/${org.id}?tab=reports`}><FileText size={12} aria-hidden="true"/>{textUI("m_orgs_view_report")}</Link>}
        </div>
      </div>
    </div>);
}

export function OrgCardSkeleton() {
    return (<li className="oc-wrap" aria-hidden="true"><div className="oc oc--skeleton"><span className="skeleton"/><span className="skeleton"/><span className="skeleton"/></div></li>);
}

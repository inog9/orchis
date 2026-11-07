import { t as textUI, useLocale, locale, fv as formatUI } from "../../i18n/runtime";
import { X } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import type { Finding } from '../../api/types';
import { CopyButton } from '../CopyButton';
import { SeverityBadge } from '../RunBadges';
import '../defense.css';
import { relativeTime } from '../../lib/relativeTime';
import { defenseOf, isNew } from '../../lib/findingsView';
import { FindingTriage } from './FindingTriage';

const FOCUSABLE = 'a[href], button:not([disabled]), [tabindex]:not([tabindex="-1"])';
const full = (iso: string) => { const d = new Date(iso); return Number.isNaN(d.getTime()) ? '—' : d.toLocaleString(locale(), { dateStyle: 'medium', timeStyle: 'short' }); };

// Panel rincian di kanan (layar penuh di ponsel). Esc menutup, Tab berputar di dalam, fokus kembali ke baris pemicu.
export function FindingDrawer({ finding, onClose, canAsset }: { finding: Finding; onClose: () => void; canAsset: boolean }) {
    useLocale();
    const [current, setCurrent] = useState(finding);
    const ref = useRef<HTMLDivElement>(null);
    const closeRef = useRef(onClose);
    closeRef.current = onClose;
    useEffect(() => {
        const previous = document.activeElement as HTMLElement | null;
        const node = ref.current;
        node?.querySelector<HTMLElement>('[data-autofocus]')?.focus();
        const onKey = (e: KeyboardEvent) => {
            if (e.key === 'Escape') { e.stopPropagation(); closeRef.current(); return; }
            if (e.key !== 'Tab' || !node) return;
            const items = [...node.querySelectorAll<HTMLElement>(FOCUSABLE)];
            if (!items.length) return;
            const first = items[0]!, last = items[items.length - 1]!;
            if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus(); }
            else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus(); }
        };
        document.addEventListener('keydown', onKey);
        return () => { document.removeEventListener('keydown', onKey); previous?.focus?.(); };
    }, []);
    const json = JSON.stringify(current.details ?? {}, null, 2);
    const def = defenseOf(current);
    const exposureText = (v: string) => (v === 'exposed' ? textUI("m_def_exp_exposed") : v === 'listening' ? textUI("m_def_exp_listening") : v === 'local' ? textUI("m_def_exp_local") : '—');
    const statusText = (v: string) => (v === 'fix_available' ? textUI("m_def_fixed_in", [def?.fixed ?? '']) : v === 'no_fix_yet' ? textUI("m_def_no_fix") : v === 'fix_requires_subscription' ? textUI("m_def_needs_sub") : v === 'fix_pending_reboot' ? textUI("m_def_pending_reboot") : '—');
    return (<div className="fnd-scrim" role="presentation" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <aside className="fnd-drawer" role="dialog" aria-modal="true" aria-labelledby="fnd-drawer-title" ref={ref}>
        <header className="fnd-drawer__head">
          <span className="fnd-drawer__kicker">{textUI("m_fnd_detail")}</span>
          <button type="button" className="fnd-icon-btn" data-autofocus aria-label={textUI("m_fnd_close")} onClick={onClose}><X size={16} aria-hidden="true"/></button>
        </header>
        <div className="fnd-drawer__body">
          <div className="fnd-drawer__badges"><SeverityBadge severity={current.severity}/>{formatUI(isNew(finding) && <span className="fnd-new">{textUI("m_fnd_new")}</span>)}</div>
          <FindingTriage finding={current} onChanged={setCurrent}/>
          <h2 id="fnd-drawer-title" className="fnd-drawer__title">{formatUI(current.title)}</h2>
          <code className="fnd-drawer__value mono">{current.value}</code>
          <dl className="fnd-meta">
            <div><dt>{textUI("m_fnd_asset")}</dt><dd>{formatUI(current.asset_id && canAsset ? <Link to={`/assets/${current.asset_id}`}>{formatUI(current.asset_name)}</Link> : current.asset_name || '—')}</dd></div>
            <div><dt>{textUI("m_fnd_org")}</dt><dd>{formatUI(current.organization_name || '—')}</dd></div>
            <div><dt>{textUI("m_fnd_first_seen")}</dt><dd>{full(current.first_seen_at)}</dd></div>
            <div><dt>{textUI("m_fnd_last_seen")}</dt><dd title={full(current.last_seen_at)}>{full(current.last_seen_at)} · {relativeTime(current.last_seen_at)}</dd></div>
            <div><dt>{textUI("m_fnd_occurrences")}</dt><dd className="mono">×{formatUI(current.occurrence_count)}</dd></div>
          </dl>
          {formatUI(def && (<section aria-labelledby="fnd-def-h"><h3 id="fnd-def-h">{textUI("m_def_section")}</h3>
            <p className="def-meta">{formatUI(def.kev && <span className="def-chip def-chip--kev">{textUI("m_def_kev")}</span>)}{formatUI(def.exposure === 'exposed' && <span className="def-chip def-chip--exposed">{textUI("m_def_exp_exposed")}</span>)}{formatUI(def.epss !== null && <span className="def-chip def-chip--muted">{textUI("m_def_epss", [(def.epss * 100).toFixed(1)])}</span>)}{formatUI(def.confirmedBy && <span className="def-chip def-chip--muted">{textUI("m_def_confirmed", [def.confirmedBy])}</span>)}</p>
            <dl className="def-dl">
              <dt>{textUI("m_def_advisory")}</dt><dd>{formatUI(def.url ? <a href={def.url} target="_blank" rel="noreferrer noopener">{def.cve}</a> : def.cve)}</dd>
              <dt>{textUI("m_def_installed")}</dt><dd className="mono">{formatUI(def.pkg)} {formatUI(def.installed)}</dd>
              <dt>{textUI("m_def_fixed_version")}</dt><dd className="mono">{formatUI(def.fixed || '—')}</dd>
              <dt>{textUI("m_def_status")}</dt><dd>{statusText(def.status)}</dd>
              <dt>{textUI("m_def_exposure")}</dt><dd>{exposureText(def.exposure)}{def.ports ? ` (${def.ports})` : ''}</dd>
              <dt>{textUI("m_def_sev_source")}</dt><dd>{formatUI(def.sevSource || '—')}</dd>
              <dt>{textUI("m_def_method")}</dt><dd className="mono">{formatUI(def.method)}</dd>
              <dt>{textUI("m_def_data_as_of")}</dt><dd>{formatUI(def.dataAsOf ? full(def.dataAsOf) : '—')}</dd>
            </dl></section>))}
          <section aria-labelledby="fnd-details-h">
            <div className="fnd-drawer__row"><h3 id="fnd-details-h">{textUI("m_fnd_details")}</h3><CopyButton value={json}/></div>
            <pre className="fnd-json mono" tabIndex={0}>{json}</pre>
          </section>
          {formatUI(current.asset_id && canAsset && <Link className="btn btn--secondary btn--auto fnd-drawer__link" to={`/assets/${current.asset_id}`}>{textUI("m_fnd_view_asset")}</Link>)}
        </div>
      </aside>
    </div>);
}

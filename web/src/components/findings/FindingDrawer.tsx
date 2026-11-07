import { t as textUI, useLocale, locale, fv as formatUI } from "../../i18n/runtime";
import { X } from 'lucide-react';
import { useEffect, useRef } from 'react';
import { Link } from 'react-router-dom';
import type { Finding } from '../../api/types';
import { CopyButton } from '../CopyButton';
import { SeverityBadge } from '../RunBadges';
import { relativeTime } from '../../lib/relativeTime';
import { isNew } from '../../lib/findingsView';

const FOCUSABLE = 'a[href], button:not([disabled]), [tabindex]:not([tabindex="-1"])';
const full = (iso: string) => { const d = new Date(iso); return Number.isNaN(d.getTime()) ? '—' : d.toLocaleString(locale(), { dateStyle: 'medium', timeStyle: 'short' }); };

// Panel rincian di kanan (layar penuh di ponsel). Esc menutup, Tab berputar di dalam, fokus kembali ke baris pemicu.
export function FindingDrawer({ finding, onClose, canAsset }: { finding: Finding; onClose: () => void; canAsset: boolean }) {
    useLocale();
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
    const json = JSON.stringify(finding.details ?? {}, null, 2);
    return (<div className="fnd-scrim" role="presentation" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <aside className="fnd-drawer" role="dialog" aria-modal="true" aria-labelledby="fnd-drawer-title" ref={ref}>
        <header className="fnd-drawer__head">
          <span className="fnd-drawer__kicker">{textUI("m_fnd_detail")}</span>
          <button type="button" className="fnd-icon-btn" data-autofocus aria-label={textUI("m_fnd_close")} onClick={onClose}><X size={16} aria-hidden="true"/></button>
        </header>
        <div className="fnd-drawer__body">
          <div className="fnd-drawer__badges"><SeverityBadge severity={finding.severity}/>{formatUI(isNew(finding) && <span className="fnd-new">{textUI("m_fnd_new")}</span>)}</div>
          <h2 id="fnd-drawer-title" className="fnd-drawer__title">{formatUI(finding.title)}</h2>
          <code className="fnd-drawer__value mono">{finding.value}</code>
          <dl className="fnd-meta">
            <div><dt>{textUI("m_fnd_asset")}</dt><dd>{formatUI(finding.asset_id && canAsset ? <Link to={`/assets/${finding.asset_id}`}>{formatUI(finding.asset_name)}</Link> : finding.asset_name || '—')}</dd></div>
            <div><dt>{textUI("m_fnd_org")}</dt><dd>{formatUI(finding.organization_name || '—')}</dd></div>
            <div><dt>{textUI("m_fnd_first_seen")}</dt><dd>{full(finding.first_seen_at)}</dd></div>
            <div><dt>{textUI("m_fnd_last_seen")}</dt><dd title={full(finding.last_seen_at)}>{full(finding.last_seen_at)} · {relativeTime(finding.last_seen_at)}</dd></div>
            <div><dt>{textUI("m_fnd_occurrences")}</dt><dd className="mono">×{formatUI(finding.occurrence_count)}</dd></div>
          </dl>
          <section aria-labelledby="fnd-details-h">
            <div className="fnd-drawer__row"><h3 id="fnd-details-h">{textUI("m_fnd_details")}</h3><CopyButton value={json}/></div>
            <pre className="fnd-json mono" tabIndex={0}>{json}</pre>
          </section>
          {formatUI(finding.asset_id && canAsset && <Link className="btn btn--secondary btn--auto fnd-drawer__link" to={`/assets/${finding.asset_id}`}>{textUI("m_fnd_view_asset")}</Link>)}
        </div>
      </aside>
    </div>);
}

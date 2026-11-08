import type { ReactNode } from 'react';

// Primitif bersama untuk halaman-halaman hasil redesain v2fixed.pen.
// Semua warna memakai token --v2-* yang sudah ada di styles/ui2.css (dark + light).
// Jangan duplikasi komponen ini di halaman; impor dari sini.

export type Severity = 'critical' | 'high' | 'medium' | 'low' | 'info';

// Pill severity: border 1px warna severity, latar transparan, label warna severity (bukan solid).
export function SeverityPill({ severity, children }: { severity: Severity | string; children: ReactNode }) {
    const tone = (['critical', 'high', 'medium', 'low', 'info'] as const).includes(severity as never) ? severity : 'info';
    return <span className={`pen-pill pen-pill--${tone}`}>{children}</span>;
}

// Tag status: border + latar soft + label berwarna (success = hijau, netral = text-3).
export function StateTag({ tone = 'neutral', children }: { tone?: 'success' | 'neutral' | 'accent' | 'warn'; children: ReactNode }) {
    return <span className={`pen-tag pen-tag--${tone}`}>{children}</span>;
}

// Bar proporsional by_severity: critical -> high -> medium -> low -> info.
export function SeverityBar({ bySeverity, total }: { bySeverity: Partial<Record<Severity, number>>; total?: number }) {
    const order: Severity[] = ['critical', 'high', 'medium', 'low', 'info'];
    const sum = total ?? order.reduce((acc, k) => acc + (bySeverity[k] ?? 0), 0);
    if (sum <= 0) return <div className="pen-sevbar pen-sevbar--empty" aria-hidden="true"/>;
    return (
        <div className="pen-sevbar" role="img" aria-label={order.map(k => `${k} ${bySeverity[k] ?? 0}`).join(', ')}>
            {order.map(k => (bySeverity[k] ?? 0) > 0 && <span key={k} className={`pen-sevbar__seg pen-sevbar__seg--${k}`} style={{ width: `${((bySeverity[k] ?? 0) / sum) * 100}%` }}/>)}
        </div>
    );
}

// Kartu KPI: label kecil, nilai besar (mono), caption opsional, bar severity opsional.
export function MetricCard({ label, value, caption, bar }: {
    label: ReactNode;
    value: ReactNode;
    caption?: ReactNode;
    bar?: { bySeverity: Partial<Record<Severity, number>>; total?: number };
}) {
    return (
        <div className="pen-metric">
            <p className="pen-metric__label">{label}</p>
            <p className="pen-metric__value">{value}</p>
            {bar && <SeverityBar bySeverity={bar.bySeverity} total={bar.total}/>}
            {caption && <p className="pen-metric__caption">{caption}</p>}
        </div>
    );
}

// Kartu panel generik dengan judul kecil (overline).
export function PanelCard({ title, children, actions }: { title: ReactNode; children: ReactNode; actions?: ReactNode }) {
    return (
        <section className="pen-panel">
            <header className="pen-panel__head">
                <h2 className="pen-panel__title">{title}</h2>
                {actions}
            </header>
            {children}
        </section>
    );
}

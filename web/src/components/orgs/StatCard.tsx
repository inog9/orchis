import type { ReactNode } from 'react';
import { useCountUp } from './useCountUp';

export function StatCard({ label, value, text, loading, children }: { label: string; value?: number; text?: string; loading?: boolean; children?: ReactNode }) {
    const n = useCountUp(value ?? 0);
    return (<div className="stat" role="group" aria-label={label}>
      <span className="stat__label">{label}</span>
      {loading ? <span className="skeleton stat__skeleton" aria-hidden="true"/> : <span className="stat__value mono" aria-label={String(value ?? text ?? '')}>{text ?? n}</span>}
      {!loading && children}
    </div>);
}

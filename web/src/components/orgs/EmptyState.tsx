import type { ReactNode } from 'react';
import { useLocale, fv as formatUI } from '../../i18n/runtime';
import { RavenMark } from '../RavenMark';

// Ilustrasi garis minimal: cincin orbit emas dan lambang gagak.
export function EmptyState({ title, body, children }: { title: string; body?: string; children?: ReactNode }) {
    useLocale();
    return (<div className="empty" role="status">
      <div className="empty__art" aria-hidden="true">
        <svg viewBox="0 0 200 200" width="168" height="168" fill="none">
          <circle cx="100" cy="100" r="96" stroke="#B8955F" strokeOpacity=".28" strokeDasharray="2 6"/>
          <circle cx="100" cy="100" r="72" stroke="#B8955F" strokeOpacity=".4"/>
          <circle cx="100" cy="100" r="48" stroke="#B8955F" strokeOpacity=".22"/>
          <circle cx="172" cy="100" r="3.5" fill="#B8955F"/>
          <circle cx="52" cy="152" r="2.5" fill="#6B5A3E"/>
        </svg>
        <RavenMark className="empty__raven" size={84}/>
      </div>
      <h2>{formatUI(title)}</h2>
      {body && <p>{formatUI(body)}</p>}
      {children && <div className="empty__cta">{children}</div>}
    </div>);
}

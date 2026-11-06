import { useId } from 'react';
import { t as textUI, tr as richUI, useLocale } from '../../i18n/runtime';

// Nilai di kartu adalah contoh tetap (bukan pengukuran nyata); kartu muncul saat hover atau fokus keyboard.
export function StatusPill() {
    useLocale();
    const id = useId();
    return (<div className="hero-pill" tabIndex={0} aria-describedby={id}>
      {richUI("m_fd77d85f4ce0", [<span className="hero-pill__dot" aria-hidden="true"><i/></span>])}
      <div id={id} role="tooltip" className="hero-pill__card">
        <dl>
          <div><dt>{textUI("m_hero_uptime")}</dt><dd>99.98%</dd></div>
          <div><dt>{textUI("m_hero_last_scan")}</dt><dd>{textUI("m_hero_ago", ['2'])}</dd></div>
          <div><dt>{textUI("m_hero_region")}</dt><dd>ID-JKT</dd></div>
          <div><dt>{textUI("m_hero_latency")}</dt><dd>24 ms</dd></div>
        </dl>
      </div>
    </div>);
}

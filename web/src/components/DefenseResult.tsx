import { t as textUI, fv as formatUI, useLocale } from '../i18n/runtime';
import type { DefenseSummary } from '../api/defense';
import { defenseNoteText, defenseWhyIncomplete, visibleNotes } from './DefenseNotes';
import './defense.css';

// Counts, an unmissable incomplete banner, and the localized coverage notes of one orchis-defense-agent run.
export function DefenseResult({ summary }: { summary: DefenseSummary }) {
    useLocale();
    return (<>
      <div className="def-stats">
        <div className="def-stat"><span className="def-stat__label">{textUI("m_def_stat_adv")}</span><span className="def-stat__value">{formatUI(summary.findings)}</span></div>
        <div className="def-stat"><span className="def-stat__label">{textUI("m_def_stat_kev")}</span><span className="def-stat__value">{formatUI(summary.known_exploited)}</span></div>
        <div className="def-stat"><span className="def-stat__label">{textUI("m_def_stat_exp")}</span><span className="def-stat__value">{formatUI(summary.exposed)}</span></div>
        <div className="def-stat"><span className="def-stat__label">{textUI("m_def_stat_sw")}</span><span className="def-stat__value">{formatUI(summary.software_checked)}</span></div>
        {formatUI(summary.software_unmapped > 0 && <div className="def-stat" title={textUI("m_def_stat_unmapped_hint")}><span className="def-stat__label">{textUI("m_def_stat_unmapped")}</span><span className="def-stat__value">{formatUI(summary.software_unmapped)}</span></div>)}
        {formatUI((summary.software_ignored ?? 0) > 0 && <div className="def-stat" title={textUI("m_def_stat_ignored_hint")}><span className="def-stat__label">{textUI("m_def_stat_ignored")}</span><span className="def-stat__value">{formatUI(summary.software_ignored ?? 0)}</span></div>)}
        {formatUI(summary.reboot_required > 0 && <div className="def-stat"><span className="def-stat__label">{textUI("m_def_stat_reboot")}</span><span className="def-stat__value">{formatUI(summary.reboot_required)}</span></div>)}
      </div>
      {formatUI(summary.disposition !== 'complete' && <p className="def-warn" role="status">{formatUI(defenseWhyIncomplete(summary.notes))}</p>)}
      {formatUI(visibleNotes(summary.notes).length > 0 && (<><h3 className="ed-h4">{textUI("m_def_notes")}</h3><ul className="def-notes">{formatUI(visibleNotes(summary.notes).map((n) => <li key={n.code}>{defenseNoteText(n)}</li>))}</ul></>))}
    </>);
}

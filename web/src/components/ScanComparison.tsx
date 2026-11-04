import {displayLabel} from '../i18n/display';
import { t as textUI, tr as richUI, useLocale, locale, fv as formatUI } from "../i18n/runtime";
import { useState } from 'react';
export type Change = 'new' | 'persistent' | 'resolved' | 'uncompared';
export type ChangeCounts = Record<Change, number>;
export type FindingChange = {
    asset_id: string;
    asset_name: string;
    finding_id: string;
    type: string;
    value: string;
    title: string;
    severity: string;
    change: Change;
    reason?: string;
    actions: string[];
};
export type Comparison = {
    version: number;
    totals: ChangeCounts;
    by_severity: Record<string, ChangeCounts>;
    rows: FindingChange[];
    checks: {
        asset_id: string;
        action: string;
        target: string;
        comparable: boolean;
        reason?: string;
        baseline_report_id?: string;
        baseline_at?: string;
    }[];
};
const labels: Record<Change, string> = { get new() {
        return textUI("m_18fdd549b2ed");
    }, get persistent() {
        return textUI("m_6bf85034bd50");
    }, get resolved() {
        return textUI("m_5be3c2c8354e");
    }, get uncompared() {
        return textUI("m_18e7c18acb71");
    } };
const reasons: Record<string, string> = { get identity_changed() {
        return textUI("m_150c9db2701a");
    }, get initial_coverage() {
        return textUI("m_2f38edf1c296");
    }, get coverage_changed() {
        return textUI("m_c4383d743cb6");
    }, get profile_changed() {
        return textUI("m_092e257f6fd6");
    }, get ambiguous_source() {
        return textUI("m_15f760956967");
    }, get scan_not_completed() { return textUI("m_1adc2dde39c8"); }, get previous_not_completed() {
        return textUI("m_bc5edc357a04");
    }, get no_comparable_baseline() {
        return textUI("m_27adc4b8adeb");
    } };
export function ScanComparison({ comparison, pending = false, filter, search = '', severity = '', type = '', assetID = '', onFilter }: {
    comparison?: Comparison;
    pending?: boolean;
    filter?: Change | '';
    search?: string;
    severity?: string;
    type?: string;
    assetID?: string;
    onFilter?: (v: Change | '') => void;
}) {
    useLocale();
    const [localFilter, setLocalFilter] = useState<Change | ''>('');
    const [shown, setShown] = useState(25);
    const change = filter ?? localFilter;
    if (!comparison)
        return <section className="org-card scan-comparison" aria-label={textUI("m_3b3b516a514f")}><h2>{textUI("m_3b3b516a514f")}</h2><p>{formatUI(pending ? textUI("m_a25c92fecc1f") : textUI("m_f197926abb82"))}</p></section>;
    const rows = comparison.rows.filter(r => (!change || r.change === change) && (!severity || r.severity === severity) && (!type || r.type === type) && (!assetID || r.asset_id === assetID) && (!search || `${r.title} ${r.value} ${r.asset_name}`.toLowerCase().includes(search.toLowerCase())));
    return <section className="org-card scan-comparison" aria-label={textUI("m_3b3b516a514f")}><h2>{textUI("m_3b3b516a514f")}</h2><p>{textUI("m_f76d303b325e")}</p><div className="severity-strip comparison-totals">{formatUI((Object.keys(labels) as Change[]).map(k => <p key={k}><strong>{formatUI(comparison.totals[k])}</strong> {formatUI(labels[k])}</p>))}</div><p className="muted small">{textUI("m_fb3c67af18ce")}</p><div className="ed-scroll" role="region" aria-label={textUI("m_8ecc02086cee")} tabIndex={0}><table className="org-table"><thead><tr><th>{textUI("m_5e9f98120dbe")}</th>{formatUI(Object.values(labels).map(label => <th key={label}>{formatUI(label)}</th>))}</tr></thead><tbody>{formatUI(['critical', 'high', 'medium', 'low', 'info', ...(comparison.by_severity.unknown ? ['unknown'] : [])].map(s => <tr key={s}><th scope="row">{formatUI(displayLabel(s))}</th>{formatUI((Object.keys(labels) as Change[]).map(k => <td key={k}>{formatUI(comparison.by_severity[s]?.[k] ?? 0)}</td>))}</tr>))}</tbody></table></div><label className="select">{richUI("m_71faa721de07", [<select aria-label={textUI("m_f7b28f45754e")} value={change} onChange={e => {
                    const v = e.target.value as Change | '';
                    setShown(25);
                    if (onFilter)
                        onFilter(v);
                    else
                        setLocalFilter(v);
                }}><option value="">{textUI("m_a0a8c325dbff")}</option>{formatUI((Object.keys(labels) as Change[]).map(k => <option key={k} value={k}>{formatUI(labels[k])}</option>))}</select>])}</label><ul className="stack-list">{formatUI(rows.slice(0, shown).map(r => <li key={`${r.asset_id}:${r.finding_id}:${r.change}`}><strong>{formatUI(r.title || r.value)}</strong><p>{formatUI(labels[r.change])} · {formatUI(displayLabel(r.severity))} · {formatUI(r.asset_name)} · {formatUI(r.actions.join(', '))}</p><p className="mono">{formatUI(r.value)}</p>{formatUI(r.reason && <p className="muted small">{formatUI(reasons[r.reason] ?? textUI("m_33296761937f"))}</p>)}</li>))}</ul>{formatUI(rows.length === 0 && <p>{textUI("m_28e4f2c609c8")}</p>)}{formatUI(rows.length > shown && <button className="btn btn--secondary btn--auto" onClick={() => setShown(n => n + 25)}>{textUI("m_2c7fc6a0ad9c")}</button>)}<details><summary>{textUI("m_1e28e384e38e")}</summary><ul className="stack-list">{formatUI(comparison.checks.map((c, i) => <li key={i}>{formatUI(c.action)} · {formatUI(c.target)} · {formatUI(c.comparable ? textUI("m_3dd02aae9f53") : reasons[c.reason ?? ''] ?? textUI("m_eb276c35e8f3"))}{formatUI(c.baseline_at ? ` · ${new Date(c.baseline_at).toLocaleString(locale())}` : '')}</li>))}</ul></details></section>;
}

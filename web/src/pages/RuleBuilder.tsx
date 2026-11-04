import { t as textUI, tr as richUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { Plus, Trash2 } from 'lucide-react';
import type { Filter, Rule } from '../api/types';
import { changeField, changeOperator, FIELDS, newFilter, OPERATOR_LABEL, takesList, takesNoValue } from '../lib/rule';
export function RuleBuilder({ rule, onChange }: {
    rule: Rule;
    onChange: (r: Rule) => void;
}) {
    useLocale();
    const set = (i: number, f: Filter) => onChange({ ...rule, filters: rule.filters.map((x, j) => (j === i ? f : x)) });
    const remove = (i: number) => onChange({ ...rule, filters: rule.filters.filter((_, j) => j !== i) });
    return (<fieldset className="rules">
      <legend>{textUI("m_4228aeb07c41")}</legend>
      {formatUI(rule.filters.length === 0 && <p className="muted small">{textUI("m_d038567a08de")}</p>)}
      {formatUI(rule.filters.length > 1 && (<div className="select select--inline">
          <label htmlFor="rule-mode">{textUI("m_03c0e806becb")}</label>
          <select id="rule-mode" value={rule.mode} onChange={(e) => onChange({ ...rule, mode: e.target.value as 'and' | 'or' })}>
            <option value="and">{textUI("m_9b41cc7d4034")}</option>
            <option value="or">{textUI("m_9b69fbdb3b90")}</option>
          </select>
        </div>))}
      <ul className="rules__list">
        {formatUI(rule.filters.map((f, i) => {
            const meta = FIELDS[f.field]!;
            const n = i + 1;
            return (<li key={i} className="rule">
              <select aria-label={textUI("m_21e0da50193b", [n])} value={f.field} onChange={(e) => set(i, changeField(f, e.target.value))}>
                {formatUI(Object.entries(FIELDS).map(([k, m]) => <option key={k} value={k}>{formatUI(m.label)}</option>))}
              </select>
              <select aria-label={textUI("m_845e5ec7ce35", [n])} value={f.operator} onChange={(e) => set(i, changeOperator(f, e.target.value))}>
                {formatUI(meta.operators.map((o) => <option key={o} value={o}>{formatUI(OPERATOR_LABEL[o])}</option>))}
              </select>
              {formatUI(takesNoValue(f.operator) ? <span className="muted small rule__none">{textUI("m_03b73e936b25")}</span> : meta.options && !takesList(f.operator) ? (<select aria-label={textUI("m_c70e907ea5f9", [n])} value={f.values[0] ?? ''} onChange={(e) => set(i, { ...f, values: [e.target.value] })}>
                  {formatUI(meta.options.map((o) => <option key={o.value} value={o.value}>{formatUI(o.label)}</option>))}
                </select>) : meta.boolean ? (<select aria-label={textUI("m_c70e907ea5f9", [n])} value={f.values[0] ?? 'true'} onChange={(e) => set(i, { ...f, values: [e.target.value] })}>
                  <option value="true">{textUI("m_85a39ab345d6")}</option><option value="false">{textUI("m_1ea442a134b2")}</option>
                </select>) : (<input aria-label={textUI("m_c70e907ea5f9", [n])} value={takesList(f.operator) ? f.values.join(', ') : (f.values[0] ?? '')} placeholder={takesList(f.operator) ? 'value, value, …' : 'value'} onChange={(e) => set(i, { ...f, values: takesList(f.operator) ? e.target.value.split(',').map((v) => v.trimStart()) : [e.target.value] })}/>))}
              <button type="button" className="icon-btn" aria-label={textUI("m_36b7b8923bd7", [n])} onClick={() => remove(i)}><Trash2 size={15}/></button>
            </li>);
        }))}
      </ul>
      <button type="button" className="btn btn--secondary btn--auto btn--sm" disabled={rule.filters.length >= 10} onClick={() => onChange({ ...rule, filters: [...rule.filters, newFilter()] })}>{richUI("m_de37b2d40132", [<Plus size={14} aria-hidden="true"/>])}</button>
    </fieldset>);
}

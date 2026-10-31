import { Plus, Trash2 } from 'lucide-react';
import type { Filter, Rule } from '../api/types';
import { changeField, changeOperator, FIELDS, newFilter, OPERATOR_LABEL, takesList, takesNoValue } from '../lib/rule';

export function RuleBuilder({ rule, onChange }: { rule: Rule; onChange: (r: Rule) => void }) {
  const set = (i: number, f: Filter) => onChange({ ...rule, filters: rule.filters.map((x, j) => (j === i ? f : x)) });
  const remove = (i: number) => onChange({ ...rule, filters: rule.filters.filter((_, j) => j !== i) });

  return (
    <fieldset className="rules">
      <legend>Rules</legend>
      {rule.filters.length === 0 && <p className="muted small">No rules: every asset matches.</p>}
      {rule.filters.length > 1 && (
        <div className="select select--inline">
          <label htmlFor="rule-mode">Match</label>
          <select id="rule-mode" value={rule.mode} onChange={(e) => onChange({ ...rule, mode: e.target.value as 'and' | 'or' })}>
            <option value="and">all of the rules</option>
            <option value="or">any of the rules</option>
          </select>
        </div>
      )}
      <ul className="rules__list">
        {rule.filters.map((f, i) => {
          const meta = FIELDS[f.field]!;
          const n = i + 1;
          return (
            <li key={i} className="rule">
              <select aria-label={`Rule ${n} field`} value={f.field} onChange={(e) => set(i, changeField(f, e.target.value))}>
                {Object.entries(FIELDS).map(([k, m]) => <option key={k} value={k}>{m.label}</option>)}
              </select>
              <select aria-label={`Rule ${n} operator`} value={f.operator} onChange={(e) => set(i, changeOperator(f, e.target.value))}>
                {meta.operators.map((o) => <option key={o} value={o}>{OPERATOR_LABEL[o]}</option>)}
              </select>
              {takesNoValue(f.operator) ? <span className="muted small rule__none">no value</span> : meta.options && !takesList(f.operator) ? (
                <select aria-label={`Rule ${n} value`} value={f.values[0] ?? ''} onChange={(e) => set(i, { ...f, values: [e.target.value] })}>
                  {meta.options.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
                </select>
              ) : meta.boolean ? (
                <select aria-label={`Rule ${n} value`} value={f.values[0] ?? 'true'} onChange={(e) => set(i, { ...f, values: [e.target.value] })}>
                  <option value="true">Yes</option><option value="false">No</option>
                </select>
              ) : (
                <input aria-label={`Rule ${n} value`} value={takesList(f.operator) ? f.values.join(', ') : (f.values[0] ?? '')}
                  placeholder={takesList(f.operator) ? 'value, value, …' : 'value'}
                  onChange={(e) => set(i, { ...f, values: takesList(f.operator) ? e.target.value.split(',').map((v) => v.trimStart()) : [e.target.value] })} />
              )}
              <button type="button" className="icon-btn" aria-label={`Remove rule ${n}`} onClick={() => remove(i)}><Trash2 size={15} /></button>
            </li>
          );
        })}
      </ul>
      <button type="button" className="btn btn--secondary btn--auto btn--sm" disabled={rule.filters.length >= 10} onClick={() => onChange({ ...rule, filters: [...rule.filters, newFilter()] })}>
        <Plus size={14} aria-hidden="true" /> Add rule
      </button>
    </fieldset>
  );
}

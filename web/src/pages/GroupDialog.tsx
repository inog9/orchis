import { t as textUI, tr as richUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { X } from 'lucide-react';
import { useEffect, useState, type FormEvent } from 'react';
import { ApiError } from '../api/client';
import { useRulePreview, useSaveGroup } from '../api/hooks';
import type { GroupDetail, Rule } from '../api/types';
import { describeError } from '../components/ErrorNote';
import { Field } from '../components/Field';
import { Modal } from '../components/Modal';
import { cleanRule } from '../lib/rule';
import { RuleBuilder } from './RuleBuilder';
function useDebounced<T>(value: T, ms: number) {
    const [v, setV] = useState(value);
    useEffect(() => { const t = setTimeout(() => setV(value), ms); return () => clearTimeout(t); }, [value, ms]);
    return v;
}
export function GroupDialog({ group, onClose, onSaved, onStale }: {
    group?: GroupDetail;
    onClose: () => void;
    onSaved: (g: GroupDetail) => void;
    onStale?: () => void;
}) {
    useLocale();
    const save = useSaveGroup(group?.id);
    const [name, setName] = useState(group?.name ?? '');
    const [description, setDescription] = useState(group?.description ?? '');
    const [rule, setRule] = useState<Rule>(group?.rule ?? { mode: 'and', filters: [] });
    const { rule: cleaned, problem } = cleanRule(rule);
    const debounced = useDebounced(JSON.stringify(cleaned), 350);
    const preview = useRulePreview(JSON.parse(debounced) as Rule, !problem);
    const err = save.error;
    const stale = err instanceof ApiError && err.status === 409;
    const fieldErr = (n: string) => (err instanceof ApiError ? err.fields.find((f) => f.name === n)?.message : undefined);
    const touch = () => save.isError && save.reset();
    const submit = (e: FormEvent) => {
        e.preventDefault();
        if (problem)
            return;
        save.mutate({ name, description: description.trim() || null, rule: cleaned, ...(group ? { version: group.version } : {}) }, { onSuccess: onSaved });
    };
    return (<Modal labelledBy="group-title" onClose={onClose}>
      <header className="dialog__head">
        <h2 id="group-title" className="dialog__title">{formatUI(group ? textUI("m_419c88caed43") : textUI("m_4b51cdcb651c"))}</h2>
        <button type="button" className="icon-btn" aria-label={textUI("m_7d9eb7acb13e")} onClick={onClose}><X size={18}/></button>
      </header>
      <form className="dialog__form" onSubmit={submit} noValidate>
        {formatUI(err && !(err instanceof ApiError && err.status === 422) && (<div className="alert" role="alert">
            <strong>{formatUI(stale ? textUI("m_33634bff9832") : textUI("m_5868d6cda776"))}</strong>
            <span>{formatUI(describeError(err).message)}{formatUI(describeError(err).errorId ? textUI("m_488d61c8380b", [describeError(err).errorId]) : '')}</span>
            {formatUI(stale && onStale && <button type="button" className="btn btn--secondary btn--auto btn--sm" onClick={onStale}>{textUI("m_a5c9bd568be6")}</button>)}
          </div>))}
        <Field label={textUI("m_dcd1d5223f73")} value={name} onChange={(e) => { touch(); setName(e.target.value); }} error={fieldErr('name')} placeholder={textUI("m_ba02e20e47f5")}/>
        <Field label={textUI("m_526e0087cc3f")} value={description} onChange={(e) => { touch(); setDescription(e.target.value); }} hint={textUI("m_a9aaf97cc9e9")}/>
        <RuleBuilder rule={rule} onChange={(r) => { touch(); setRule(r); }}/>
        {formatUI(fieldErr('rule') && <p className="field__error" role="alert">{formatUI(fieldErr('rule'))}</p>)}
        <div className="preview" role="status" aria-live="polite">
          {formatUI(problem ? <span className="muted">{formatUI(problem)}</span> : preview.isError ? <span className="field__error">{formatUI(describeError(preview.error).message)}</span>
            : preview.data ? (<>
              <strong>{richUI("m_9fea8201753a", [formatUI(preview.data.count), formatUI(preview.data.total)])}</strong>{" "}{textUI("m_80e07e577d01")}{formatUI(preview.data.sample.length > 0 && <span className="muted small"> · {formatUI(preview.data.sample.map((s) => s.name).join(', '))}{formatUI(preview.data.count > preview.data.sample.length ? '…' : '')}</span>)}
            </>) : <span className="muted">{textUI("m_ec963ffc911b")}</span>)}
        </div>
        <footer className="dialog__foot">
          <button type="button" className="btn btn--secondary btn--auto" onClick={onClose}>{textUI("m_19766ed6ccb2")}</button>
          <button type="submit" className="btn btn--primary btn--auto" disabled={save.isPending || !!problem}>{formatUI(save.isPending ? textUI("m_23e39291d613") : group ? textUI("m_dd0ae7a5cbcf") : textUI("m_35be9c541d68"))}</button>
        </footer>
      </form>
    </Modal>);
}

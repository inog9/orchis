import { t as textUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { X } from 'lucide-react';
import { useState, type FormEvent } from 'react';
import { ApiError } from '../api/client';
import { useCreateScenario } from '../api/hooks';
import { describeError } from '../components/ErrorNote';
import { Field } from '../components/Field';
import { Modal } from '../components/Modal';
export function NewScenarioDialog({ onClose, onCreated, title = textUI("m_dc2c70715f47") }: {
    onClose: () => void;
    onCreated: (id: string) => void;
    title?: string;
}) {
    useLocale();
    const create = useCreateScenario();
    const [name, setName] = useState('');
    const [description, setDescription] = useState('');
    const [severity, setSeverity] = useState('medium');
    const err = create.error;
    const fieldErr = (n: string) => (err instanceof ApiError ? err.fields.find((f) => f.name === n)?.message : undefined);
    const submit = (e: FormEvent) => {
        e.preventDefault();
        create.mutate({ name, description: description.trim() || null, severity }, { onSuccess: (s) => onCreated(s.id) });
    };
    return (<Modal labelledBy="new-scenario-title" onClose={onClose} width={520}>
      <>
        <header className="dialog__head">
          <h2 id="new-scenario-title">{formatUI(title)}</h2>
          <button type="button" className="icon-btn" aria-label={textUI("m_7d9eb7acb13e")} onClick={onClose}><X size={18}/></button>
        </header>
        <form className="dialog__form" onSubmit={submit} noValidate>
          {formatUI(err && !(err instanceof ApiError && err.status === 422) && <div className="alert" role="alert"><strong>{textUI("m_33e74537c75e")}</strong><span>{formatUI(describeError(err).message)}</span></div>)}
          <Field label={textUI("m_dcd1d5223f73")} value={name} onChange={(e) => { create.reset(); setName(e.target.value); }} error={fieldErr('name')} placeholder={textUI("m_0dd3e2fdc2ed")} autoFocus/>
          <div className="textarea">
            <label htmlFor="sc-desc">{textUI("m_f6cbe2f0c1f8")}</label>
            <textarea id="sc-desc" rows={3} value={description} onChange={(e) => setDescription(e.target.value)}/>
          </div>
          <div className="select">
            <label htmlFor="sc-sev">{textUI("m_5e9f98120dbe")}</label>
            <select id="sc-sev" value={severity} onChange={(e) => setSeverity(e.target.value)}>
              <option value="low">{textUI("m_f793de205ead")}</option><option value="medium">{textUI("m_8e588cd18774")}</option><option value="high">{textUI("m_c4ebc6d4a583")}</option><option value="critical">{textUI("m_427dd2969bd1")}</option>
            </select>
          </div>
          <footer className="dialog__foot">
            <button type="button" className="btn btn--secondary btn--auto" onClick={onClose}>{textUI("m_19766ed6ccb2")}</button>
            <button type="submit" className="btn btn--primary btn--auto" disabled={create.isPending}>{formatUI(create.isPending ? textUI("m_c79ed9492e3c") : textUI("m_4759498ac2a7"))}</button>
          </footer>
        </form>
      </>
    </Modal>);
}

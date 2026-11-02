import { X } from 'lucide-react';
import { useState, type FormEvent } from 'react';
import { ApiError } from '../api/client';
import { useCreateScenario } from '../api/hooks';
import { describeError } from '../components/ErrorNote';
import { Field } from '../components/Field';
import { Modal } from '../components/Modal';

export function NewScenarioDialog({ onClose, onCreated, title = 'New scenario' }: { onClose: () => void; onCreated: (id: string) => void; title?: string }) {
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
  return (
    <Modal labelledBy="new-scenario-title" onClose={onClose} width={520}>
      <>
        <header className="dialog__head">
          <h2 id="new-scenario-title">{title}</h2>
          <button type="button" className="icon-btn" aria-label="Close" onClick={onClose}><X size={18} /></button>
        </header>
        <form className="dialog__form" onSubmit={submit} noValidate>
          {err && !(err instanceof ApiError && err.status === 422) && <div className="alert" role="alert"><strong>We couldn’t create it</strong><span>{describeError(err).message}</span></div>}
          <Field label="Name" value={name} onChange={(e) => { create.reset(); setName(e.target.value); }} error={fieldErr('name')} placeholder="Ransomware kill chain" autoFocus />
          <div className="textarea">
            <label htmlFor="sc-desc">Description (optional)</label>
            <textarea id="sc-desc" rows={3} value={description} onChange={(e) => setDescription(e.target.value)} />
          </div>
          <div className="select">
            <label htmlFor="sc-sev">Severity</label>
            <select id="sc-sev" value={severity} onChange={(e) => setSeverity(e.target.value)}>
              <option value="low">Low</option><option value="medium">Medium</option><option value="high">High</option><option value="critical">Critical</option>
            </select>
          </div>
          <footer className="dialog__foot">
            <button type="button" className="btn btn--secondary btn--auto" onClick={onClose}>Cancel</button>
            <button type="submit" className="btn btn--primary btn--auto" disabled={create.isPending}>{create.isPending ? 'Creating…' : 'Create'}</button>
          </footer>
        </form>
      </>
    </Modal>
  );
}

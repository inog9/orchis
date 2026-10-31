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

export function GroupDialog({ group, onClose, onSaved, onStale }: { group?: GroupDetail; onClose: () => void; onSaved: (g: GroupDetail) => void; onStale?: () => void }) {
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
    if (problem) return;
    save.mutate({ name, description: description.trim() || null, rule: cleaned, ...(group ? { version: group.version } : {}) }, { onSuccess: onSaved });
  };

  return (
    <Modal labelledBy="group-title" onClose={onClose}>
      <header className="dialog__head">
        <h2 id="group-title" className="dialog__title">{group ? 'Edit asset group' : 'Create a new asset group'}</h2>
        <button type="button" className="icon-btn" aria-label="Close" onClick={onClose}><X size={18} /></button>
      </header>
      <form className="dialog__form" onSubmit={submit} noValidate>
        {err && !(err instanceof ApiError && err.status === 422) && (
          <div className="alert" role="alert">
            <strong>{stale ? 'This group changed' : 'We couldn’t save the group'}</strong>
            <span>{describeError(err).message}{describeError(err).errorId ? ` Error id: ${describeError(err).errorId}` : ''}</span>
            {stale && onStale && <button type="button" className="btn btn--secondary btn--auto btn--sm" onClick={onStale}>Reload the latest version</button>}
          </div>
        )}
        <Field label="Name" value={name} onChange={(e) => { touch(); setName(e.target.value); }} error={fieldErr('name')} placeholder="Crown jewels" />
        <Field label="Description" value={description} onChange={(e) => { touch(); setDescription(e.target.value); }} hint="Optional." />
        <RuleBuilder rule={rule} onChange={(r) => { touch(); setRule(r); }} />
        {fieldErr('rule') && <p className="field__error" role="alert">{fieldErr('rule')}</p>}
        <div className="preview" role="status" aria-live="polite">
          {problem ? <span className="muted">{problem}</span> : preview.isError ? <span className="field__error">{describeError(preview.error).message}</span>
            : preview.data ? (<>
              <strong>{preview.data.count} of {preview.data.total}</strong> assets match
              {preview.data.sample.length > 0 && <span className="muted small"> · {preview.data.sample.map((s) => s.name).join(', ')}{preview.data.count > preview.data.sample.length ? '…' : ''}</span>}
            </>) : <span className="muted">Checking…</span>}
        </div>
        <footer className="dialog__foot">
          <button type="button" className="btn btn--secondary btn--auto" onClick={onClose}>Cancel</button>
          <button type="submit" className="btn btn--primary btn--auto" disabled={save.isPending || !!problem}>{save.isPending ? 'Saving…' : group ? 'Save changes' : 'Create group'}</button>
        </footer>
      </form>
    </Modal>
  );
}

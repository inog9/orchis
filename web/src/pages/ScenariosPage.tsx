import { Plus, X } from 'lucide-react';
import { useState, type FormEvent } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import { ApiError } from '../api/client';
import { useCreateScenario, useMe, useScenarios } from '../api/hooks';
import { can } from '../api/types';
import { describeError } from '../components/ErrorNote';
import { Field } from '../components/Field';
import { Modal } from '../components/Modal';
import { ScenarioSeverity } from '../components/StatusPills';

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

export function ScenariosPage() {
  const { data: me } = useMe();
  const navigate = useNavigate();
  const q = useScenarios();
  const [params, setParams] = useSearchParams();
  const [creating, setCreating] = useState(params.get('new') === '1' && can(me, 'scenario.write'));
  const close = () => { setCreating(false); if (params.has('new')) setParams({}, { replace: true }); };
  const items = q.data ?? [];
  return (
    <section className="page" aria-labelledby="scenarios-title">
      <div className="page__head">
        <div>
          <div className="page__title-row"><h1 id="scenarios-title" className="page__title">Scenarios</h1>{q.data && <span className="badge">{items.length}</span>}</div>
          <p className="page__sub">A scenario is a chain of actions against a set of assets. Launch it to run a simulation.</p>
        </div>
        {can(me, 'scenario.write') && <button type="button" className="btn btn--primary btn--auto" onClick={() => setCreating(true)}><Plus size={16} aria-hidden="true" /> New scenario</button>}
      </div>
      {q.isPending ? (
        <ul className="cards" aria-label="Loading scenarios">{[0, 1, 2].map((i) => <li key={i} className="card"><span className="skeleton" /></li>)}</ul>
      ) : q.isError ? (
        <div className="state state--error" role="alert"><h2>Couldn’t load scenarios</h2><p>{describeError(q.error).message} Your data is safe.</p>
          <button type="button" className="btn btn--secondary btn--auto" onClick={() => q.refetch()}>Try again</button></div>
      ) : items.length === 0 ? (
        <div className="state"><h2>No scenarios yet</h2><p>Create a scenario, add its actions in the chain workspace, choose the assets it targets, and launch it.</p>
          {can(me, 'scenario.write') && <button type="button" className="btn btn--primary btn--auto" onClick={() => setCreating(true)}>Create the first scenario</button>}</div>
      ) : (
        <ul className="cards">
          {items.map((s) => (
            <li key={s.id} className="card">
              <div className="card__top"><Link className="card__name" to={`/scenarios/${s.id}`}>{s.name}</Link><span className={`status ${s.status === 'ready' ? 'status--ok' : ''}`}>{s.status === 'ready' ? 'Ready' : 'Draft'}</span></div>
              <p className="card__rule"><ScenarioSeverity severity={s.severity} /> · {s.step_count} {s.step_count === 1 ? 'action' : 'actions'} · {s.target_count} {s.target_count === 1 ? 'target' : 'targets'}</p>
              <p className="card__count"><strong>{s.simulations}</strong> {s.simulations === 1 ? 'simulation' : 'simulations'}</p>
            </li>
          ))}
        </ul>
      )}
      {creating && <NewScenarioDialog onClose={close} onCreated={(id) => navigate(`/scenarios/${id}`)} />}
    </section>
  );
}

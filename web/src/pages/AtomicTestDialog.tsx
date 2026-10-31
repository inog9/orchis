import { TestPrerequisites } from '../components/TestPrerequisites';
import { useQuery, useMutation } from '@tanstack/react-query';
import { SchemaFields, schemaDefaults } from '../components/SchemaFields';
import { X } from 'lucide-react';
import { useState, type FormEvent } from 'react';
import { api, ApiError } from '../api/client';
import { useContracts, useCreateAtomicTest, useTargetAssets } from '../api/hooks';
import type { AtomicTest } from '../api/types';
import { describeError } from '../components/ErrorNote';
import { Field } from '../components/Field';
import { Modal } from '../components/Modal';



export function AtomicTestDialog({ onClose, onCreated, initialAction }: { onClose: () => void; onCreated: (t: AtomicTest) => void; initialAction?: string }) {
  const contracts = useContracts();
  const assets = useTargetAssets();
  const create = useCreateAtomicTest();
  const [name, setName] = useState('');
  const [contractKey, setContractKey] = useState(initialAction ?? 'nmap.tcp_connect');
  const [target, setTarget] = useState('');
  const [params, setParams] = useState<Record<string, unknown>>({});
  const presets = useQuery({ queryKey: ['action-presets'], queryFn: () => api.get<{ items: { id: string; name: string; contract_key: string; params: Record<string, unknown> }[] }>('/api/v1/action-presets') });
  const savePreset = useMutation({ mutationFn: () => api.post('/api/v1/action-presets', { name: name.trim() + ' preset', contract_key: contractKey, params: { ...schemaDefaults(contract?.input_schema), ...params } }), onSuccess: () => presets.refetch() });

  const err = create.error;
  const touch = () => create.isError && create.reset();
  const fieldErr = (n: string) => (err instanceof ApiError ? err.fields.find((f) => f.name === n)?.message : undefined);
  const contract = contracts.data?.find((c) => c.key === contractKey);
  const needsAgent = !!contract?.needs_agent;
  const targets = (assets.data ?? []).filter((a) => !needsAgent || (a.agent_state && a.agent_state !== 'none'));

  const submit = (e: FormEvent) => {
    e.preventDefault();
    create.mutate({ name, contract_key: contractKey, params: { ...schemaDefaults(contract?.input_schema), ...params }, target_asset_id: target || null }, { onSuccess: onCreated });
  };

  return (
    <Modal labelledBy="atomic-title" onClose={onClose}>
      <>
        <header className="dialog__head">
          <h2 id="atomic-title">New single test</h2>
          <button type="button" className="icon-btn" aria-label="Close" onClick={onClose}><X size={18} /></button>
        </header>
        <form className="dialog__form" onSubmit={submit} noValidate>
          {err && !(err instanceof ApiError && err.status === 422) && (
            <div className="alert" role="alert"><strong>We couldn’t create the test</strong><span>{describeError(err).message}</span></div>
          )}
          <Field label="Name" value={name} onChange={(e) => { touch(); setName(e.target.value); }} error={fieldErr('name')} placeholder="Web server top ports" autoFocus />
          <div className="select">
            <label htmlFor="at-contract">Action</label>
            <select id="at-contract" value={contractKey} onChange={(e) => { touch(); setContractKey(e.target.value); setParams({}); }}>
              {(contracts.data ?? []).map((c) => <option key={c.key} value={c.key}>{c.name}</option>)}
              {!contracts.data && <option value="nmap.tcp_connect">Nmap TCP connect scan</option>}
            </select>
            {contract && <small className="field__hint">{contract.description}</small>}
          </div>
          <div className="select">
            <label htmlFor="at-target">Target asset</label>
            <select id="at-target" value={target} onChange={(e) => { touch(); setTarget(e.target.value); }} aria-describedby="at-target-hint">
              <option value="">Choose a target</option>
              {targets.map((a) => <option key={a.id} value={a.id}>{a.name}{needsAgent && a.agent_state === 'offline' ? ' (agent offline)' : ''}</option>)}
            </select>
            <small id="at-target-hint" className={fieldErr('target_asset_id') ? 'field__error' : 'field__hint'} role={fieldErr('target_asset_id') ? 'alert' : undefined}>
              {fieldErr('target_asset_id') ?? (assets.data && targets.length === 0 ? (needsAgent ? 'No authorized asset has an agent yet. Install one under Assets → Agents.' : 'No asset is authorized for testing yet. An administrator must authorize one.') : needsAgent ? 'Only authorized assets that have an agent are listed.' : 'Only assets authorized for testing are listed.')}
            </small>
          </div>
          <TestPrerequisites targetID={target} needsAgent={needsAgent} />
          <div className="select"><label htmlFor="action-preset">Saved preset</label><select id="action-preset" value="" onChange={e => { const p = presets.data?.items.find(p => p.id === e.target.value); if (p) { setContractKey(p.contract_key); setParams(p.params); } }}><option value="">Choose a preset</option>{presets.data?.items.map(p => <option key={p.id} value={p.id}>{p.name}</option>)}</select></div>
          <SchemaFields schema={contract?.input_schema} value={params} onChange={v => { touch(); setParams(v); }} error={fieldErr} />
          <button type="button" className="btn btn--secondary btn--auto" disabled={!name.trim() || savePreset.isPending} onClick={() => savePreset.mutate()}>Save settings as preset</button>
          {savePreset.isSuccess && <p role="status">Preset saved.</p>}{savePreset.error && <p role="alert">{describeError(savePreset.error).message}</p>}
          {fieldErr('params') && <p className="field__error" role="alert">{fieldErr('params')}</p>}
          <footer className="dialog__foot">
            <button type="button" className="btn btn--secondary btn--auto" onClick={onClose}>Cancel</button>
            <button type="submit" className="btn btn--primary btn--auto" disabled={create.isPending}>{create.isPending ? 'Creating…' : 'Create test'}</button>
          </footer>
        </form>
      </>
    </Modal>
  );
}

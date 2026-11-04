import { t as textUI, useLocale, fv as formatUI } from "../i18n/runtime";
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
export function AtomicTestDialog({ onClose, onCreated, initialAction }: {
    onClose: () => void;
    onCreated: (t: AtomicTest) => void;
    initialAction?: string;
}) {
    useLocale();
    const contracts = useContracts();
    const assets = useTargetAssets();
    const create = useCreateAtomicTest();
    const [name, setName] = useState('');
    const [contractKey, setContractKey] = useState(initialAction ?? 'nmap.tcp_connect');
    const [target, setTarget] = useState('');
    const [params, setParams] = useState<Record<string, unknown>>({});
    const presets = useQuery({ queryKey: ['action-presets'], queryFn: () => api.get<{
            items: {
                id: string;
                name: string;
                contract_key: string;
                params: Record<string, unknown>;
            }[];
        }>('/api/v1/action-presets') });
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
    return (<Modal labelledBy="atomic-title" onClose={onClose}>
      <>
        <header className="dialog__head">
          <h2 id="atomic-title">{textUI("m_1e3dc38138e5")}</h2>
          <button type="button" className="icon-btn" aria-label={textUI("m_7d9eb7acb13e")} onClick={onClose}><X size={18}/></button>
        </header>
        <form className="dialog__form" onSubmit={submit} noValidate>
          {formatUI(err && !(err instanceof ApiError && err.status === 422) && (<div className="alert" role="alert"><strong>{textUI("m_9456095efda3")}</strong><span>{formatUI(describeError(err).message)}</span></div>))}
          <Field label={textUI("m_dcd1d5223f73")} value={name} onChange={(e) => { touch(); setName(e.target.value); }} error={fieldErr('name')} placeholder={textUI("m_9c472e09ce17")} autoFocus/>
          <div className="select">
            <label htmlFor="at-contract">{textUI("m_64cff1319d2f")}</label>
            <select id="at-contract" value={contractKey} onChange={(e) => { touch(); setContractKey(e.target.value); setParams({}); }}>
              {formatUI((contracts.data ?? []).map((c) => <option key={c.key} value={c.key}>{formatUI(c.name)}</option>))}
              {formatUI(!contracts.data && <option value="nmap.tcp_connect">{textUI("m_21756b3bb1fd")}</option>)}
            </select>
            {formatUI(contract && <small className="field__hint">{formatUI(contract.description)}</small>)}
          </div>
          <div className="select">
            <label htmlFor="at-target">{textUI("m_c3681c7be914")}</label>
            <select id="at-target" value={target} onChange={(e) => { touch(); setTarget(e.target.value); }} aria-describedby="at-target-hint">
              <option value="">{textUI("m_a141a1cccae2")}</option>
              {formatUI(targets.map((a) => <option key={a.id} value={a.id}>{formatUI(a.name)}{formatUI(needsAgent && a.agent_state === 'offline' ? ' (agent offline)' : '')}</option>))}
            </select>
            <small id="at-target-hint" className={fieldErr('target_asset_id') ? 'field__error' : 'field__hint'} role={fieldErr('target_asset_id') ? 'alert' : undefined}>
              {formatUI(fieldErr('target_asset_id') ?? (assets.data && targets.length === 0 ? (needsAgent ? textUI("m_4efafe827a09") : textUI("m_e9c5dcc94d18")) : needsAgent ? textUI("m_9b358c35bfcb") : textUI("m_a80efb15b55f")))}
            </small>
          </div>
          <TestPrerequisites targetID={target} needsAgent={needsAgent}/>
          <div className="select"><label htmlFor="action-preset">{textUI("m_50edcc5b7e08")}</label><select id="action-preset" value="" onChange={e => { const p = presets.data?.items.find(p => p.id === e.target.value); if (p) {
        setContractKey(p.contract_key);
        setParams(p.params);
    } }}><option value="">{textUI("m_2f311e68c311")}</option>{formatUI(presets.data?.items.map(p => <option key={p.id} value={p.id}>{formatUI(p.name)}</option>))}</select></div>
          <SchemaFields schema={contract?.input_schema} value={params} onChange={v => { touch(); setParams(v); }} error={fieldErr}/>
          <button type="button" className="btn btn--secondary btn--auto" disabled={!name.trim() || savePreset.isPending} onClick={() => savePreset.mutate()}>{textUI("m_a569887a9070")}</button>
          {formatUI(savePreset.isSuccess && <p role="status">{textUI("m_c9f6e4c632bf")}</p>)}{formatUI(savePreset.error && <p role="alert">{formatUI(describeError(savePreset.error).message)}</p>)}
          {formatUI(fieldErr('params') && <p className="field__error" role="alert">{formatUI(fieldErr('params'))}</p>)}
          <footer className="dialog__foot">
            <button type="button" className="btn btn--secondary btn--auto" onClick={onClose}>{textUI("m_19766ed6ccb2")}</button>
            <button type="submit" className="btn btn--primary btn--auto" disabled={create.isPending}>{formatUI(create.isPending ? textUI("m_c79ed9492e3c") : textUI("m_f27270ed3ec3"))}</button>
          </footer>
        </form>
      </>
    </Modal>);
}

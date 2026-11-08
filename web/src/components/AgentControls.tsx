import { collectionSupported } from '../lib/collectionSupport';
import { t as textUI, tr as richUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { X } from 'lucide-react';
import { api } from '../api/client';
import type { AgentRow } from '../api/types';
import { describeError } from './ErrorNote';
import { Modal } from './Modal';
import { when } from './RunBadges';
type Rollout = {
    mode: 'automatic' | 'manual';
    version: number;
};
const diagnostics: Record<string, string> = {
    get update_failed() {
        return textUI("m_2f1ad42e0dee");
    },
    get release_key_missing() {
        return textUI("m_0aef6b6f5d66");
    },
    get collection_failed() {
        return textUI("m_58ce9deb3673");
    },
    get uninstall_failed() {
        return textUI("m_917641eae665");
    },
    get uninstall_unsupported() {
        return textUI("m_75b838d0d36d");
    },
};
export function AgentRollout() {
    useLocale();
    const qc = useQueryClient();
    const q = useQuery({ queryKey: ['agent-rollout'], queryFn: () => api.get<Rollout>('/api/v1/agents/rollout'), retry: false });
    const [draft, setDraft] = useState<Rollout | null>(null);
    const save = useMutation({ mutationFn: (v: Rollout) => api.put<Rollout>('/api/v1/agents/rollout', v), onSuccess: (v) => { qc.setQueryData(['agent-rollout'], v); setDraft(null); } });
    if (q.isPending)
        return <p role="status">{textUI("m_5e5d393eaf68")}</p>;
    if (q.isError)
        return <div className="alert" role="alert"><p>{formatUI(describeError(q.error).message)}</p><button className="btn btn--secondary btn--auto" onClick={() => q.refetch()}>{textUI("m_0b64d6ba1944")}</button></div>;
    const value = draft ?? q.data!;
    return <section className="panel" aria-labelledby="agent-rollout-title"><h2 id="agent-rollout-title">{textUI("m_7ff9bedd3746")}</h2>
  <p>{textUI("m_9d9b64d63156")}</p>
  <form className="row" onSubmit={e => { e.preventDefault(); save.mutate(value); }}>
   <div className="select"><label htmlFor="agent-rollout">{textUI("m_6368c76023e8")}</label><select id="agent-rollout" value={value.mode} disabled={save.isPending} onChange={e => setDraft({ ...value, mode: e.target.value as Rollout['mode'] })}><option value="automatic">{textUI("m_4d509e638775")}</option><option value="manual">{textUI("m_24c7004f4584")}</option></select></div>
   <button className="btn btn--primary btn--auto" disabled={!draft || save.isPending}>{formatUI(save.isPending ? textUI("m_23e39291d613") : textUI("m_ebba7e10f9f2"))}</button>
  </form>
  {formatUI(save.isError && <div className="alert" role="alert"><p>{formatUI(describeError(save.error).message)}</p><button className="btn btn--secondary btn--auto" onClick={() => { setDraft(null); save.reset(); void q.refetch(); }}>{textUI("m_0a3a847c876c")}</button></div>)}
 </section>;
}
export function AgentDiagnostics({ agent: a }: {
    agent: AgentRow;
}) {
    useLocale();
    return <><p className="muted small">{richUI("m_42cb0fc29197", [formatUI(a.version), formatUI(a.osquery_version || textUI("m_2087cb5f8b63")), formatUI(a.os), formatUI(a.arch), formatUI(when(a.last_heartbeat_at))])}</p>
  {formatUI(a.status !== 'revoked' && <p className="small">{formatUI(a.update_status === 'unknown' ? textUI("m_b3e0edc99680") : a.update_available ? a.requested_update_version ? textUI("m_e922e59506d3", [a.requested_update_version]) : textUI("m_41679342fca6") : textUI("m_3b1f080010ce"))}{formatUI(a.paused ? ' Agent paused: no new tasks or collections; a task already running may finish.' : '')}</p>)}
  {formatUI(a.diagnostic_code && diagnostics[a.diagnostic_code] && <p role="status" className="notice">{formatUI(diagnostics[a.diagnostic_code])} {formatUI(a.diagnostic_at ? when(a.diagnostic_at) : '')}</p>)}
  {formatUI(a.uninstall_status && a.uninstall_status !== 'none' && <p className="notice" role="status">{formatUI(a.uninstall_status === 'completed' ? textUI("m_86aafd03aef3") : a.uninstall_status === 'failed' ? textUI("m_a7484a1b0059") : a.uninstall_status === 'queued' ? textUI("m_fdb4b9bf8993") : textUI("m_233dbc78595c"))}</p>)}
 </>;
}
export function AgentControls({ agent: a }: {
    agent: AgentRow;
}) {
    useLocale();
    const qc = useQueryClient();
    const [confirming, setConfirming] = useState(false);
    const [typed, setTyped] = useState('');
    const [collected, setCollected] = useState(false);
    const control = useMutation({ mutationFn: (action: string) => api.post<AgentRow>(`/api/v1/agents/${a.id}/control`, { action, version: a.control_version ?? 1, ...(action === 'uninstall' ? { confirmation: typed } : {}) }), onSuccess: () => { setConfirming(false); setTyped(''); void qc.invalidateQueries({ queryKey: ['agents'] }); } });
    const collect = useMutation({ mutationFn: () => api.post<{
            execution_ids: string[];
        }>(`/api/v1/agents/${a.id}/collect`), onSuccess: () => { setCollected(true); void qc.invalidateQueries({ queryKey: ['asset-endpoint'] }); } });
    const uninstallPending = a.uninstall_status === 'queued' || a.uninstall_status === 'dispatched';
    const busy = control.isPending || collect.isPending;
    const unavailable = a.status === 'revoked' || uninstallPending;
    const collectSupported = collectionSupported(a.os, a.arch);
    // Setiap tombol yang mati harus punya alasan yang terlihat di dekatnya.
    const reasons: string[] = [];
    if (a.status !== 'revoked' && !unavailable && !a.update_available)
        reasons.push(textUI("m_ac_update_off"));
    if (a.status !== 'revoked' && !unavailable && collectSupported && a.paused)
        reasons.push(textUI("m_ac_collect_paused"));
    else if (a.status !== 'revoked' && !unavailable && collectSupported && a.status !== 'online')
        reasons.push(textUI("m_ac_collect_offline"));
    if (unavailable && a.status !== 'revoked')
        reasons.push(textUI("m_ac_busy"));
    return <div className="agent-controls">
  <AgentDiagnostics agent={a}/>
  {formatUI(a.status !== 'revoked' && <div className="row" style={{ flexWrap: 'wrap' }}>
   <button className="btn btn--secondary btn--sm btn--auto" disabled={busy || unavailable || !a.update_available} onClick={() => control.mutate('update')}>{textUI("m_63bf045213ce")}</button>
   <button className="btn btn--secondary btn--sm btn--auto" disabled={busy || unavailable} onClick={() => control.mutate(a.paused ? 'resume' : 'pause')}>{formatUI(a.paused ? textUI("m_0bb60c450191") : textUI("m_51ca75d39c87"))}</button>
   {formatUI(collectSupported && <button className="btn btn--secondary btn--sm btn--auto" disabled={busy || unavailable || !!a.paused || a.status !== 'online'} onClick={() => { setCollected(false); collect.mutate(); }}>{textUI("m_f084daccd993")}</button>)}
   <button className="btn btn--danger btn--sm btn--auto" disabled={busy || unavailable || a.control_protocol !== 1} onClick={() => { control.reset(); setTyped(''); setConfirming(true); }}>{textUI("m_ccc8b738a170")}</button>
  </div>)}
  {formatUI(reasons.length > 0 && <div className="muted small" role="note">{formatUI(reasons.map(r => <p key={r}>{formatUI(r)}</p>))}</div>)}
  {formatUI(a.status !== 'revoked' && a.control_protocol !== 1 && <p className="muted small">{textUI("m_34b487fcc0e4")}</p>)}
  {formatUI(collected && <p role="status">{textUI("m_86e72d6825d5")}</p>)}
  {formatUI((control.isError || collect.isError) && <div className="alert" role="alert"><p>{formatUI(describeError(control.error ?? collect.error).message)}</p><button className="btn btn--secondary btn--auto" onClick={() => { control.reset(); collect.reset(); void qc.invalidateQueries({ queryKey: ['agents'] }); }}>{textUI("m_c39ad7760707")}</button></div>)}
  {formatUI(confirming && <Modal labelledBy="uninstall-agent-title" onClose={() => {
                if (!busy)
                    setConfirming(false);
            }} width={640}><>
   <header className="dialog__head"><h2 id="uninstall-agent-title">{richUI("m_9345808c614e", [formatUI(a.instance_name || a.asset_name)])}</h2><button className="icon-btn" aria-label={textUI("m_7d9eb7acb13e")} disabled={busy} onClick={() => setConfirming(false)}><X size={18}/></button></header>
   <form className="dialog__form" onSubmit={e => {
                e.preventDefault();
                if (typed === a.id)
                    control.mutate('uninstall');
            }}>
    <p>{textUI("m_50c95616a492")}</p>
    <p className="mono" style={{ overflowWrap: 'anywhere' }}>{formatUI(a.id)}</p>
    <div className="field"><label htmlFor="uninstall-agent-id">{textUI("m_8ed5cfb45fcc")}</label><div className="field__input"><input id="uninstall-agent-id" autoComplete="off" value={typed} disabled={busy} onChange={e => setTyped(e.target.value)}/></div></div>
    {formatUI(control.isError && <p className="field__error" role="alert">{formatUI(describeError(control.error).message)}</p>)}
    <footer className="dialog__foot" style={{ flexWrap: 'wrap' }}><button className="btn btn--secondary btn--auto" type="button" disabled={busy} onClick={() => setConfirming(false)}>{textUI("m_19766ed6ccb2")}</button><button className="btn btn--danger btn--auto" disabled={busy || typed !== a.id}>{formatUI(busy ? textUI("m_37255b74c579") : textUI("m_98458b29ceb2"))}</button></footer>
   </form>
  </></Modal>)}
 </div>;
}

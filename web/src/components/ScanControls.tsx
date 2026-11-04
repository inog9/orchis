import { t as textUI, tr as richUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/client';
import { useMe } from '../api/hooks';
import { can } from '../api/types';
import { describeError } from './ErrorNote';
import { Modal } from './Modal';
type Window = {
    timezone?: string;
    days?: number[];
    start?: string;
    end?: string;
};
export type ScanControl = {
    paused: boolean;
    global_paused: boolean;
    paused_by?: string;
    global_paused_by?: string;
    window: Window;
    version: number;
    allowed_now: boolean;
    restriction?: string;
};
function path(org?: string) { return org ? `/api/v1/organizations/${org}/scan-control` : '/api/v1/scan-control'; }
export function useScanControl(org?: string, enabled = true) {
    const { data: me } = useMe();
    return useQuery({ queryKey: ['scan-control', org ?? 'global'], enabled: enabled && (org ? can(me, 'organization.read') : can(me, 'settings.manage')), queryFn: () => api.get<ScanControl>(path(org)), refetchInterval: 2000 });
}
export function ScanControls({ org, compact = false }: {
    org?: string;
    compact?: boolean;
}) {
    useLocale();
    const { data: me } = useMe();
    const q = useScanControl(org);
    const qc = useQueryClient();
    const [confirm, setConfirm] = useState(false);
    const [edit, setEdit] = useState(false);
    const change = useMutation({ mutationFn: (action: 'stop' | 'resume') => api.post<ScanControl>(`${path(org)}/${action}`, { version: q.data?.version }), onSuccess: async () => { setConfirm(false); await Promise.all(['scan-control', 'scans', 'home', 'simulation'].map(k => qc.invalidateQueries({ queryKey: [k] }))); } });
    const state = q.data;
    const admin = can(me, 'settings.manage');
    const actor = state?.global_paused ? state.global_paused_by : state?.paused_by;
    if (compact && state?.allowed_now && !q.error) return null;
    return <section className="org-card" aria-label={org ? textUI("m_e5d00a52fdbe") : textUI("m_83470fbd8cc4")}>
    {formatUI(!compact && <h2>{formatUI(org ? textUI("m_22ff6d55a906") : textUI("m_466ffcce3e2a"))}</h2>)}
    {formatUI(q.error && <p role="alert" className="alert">{formatUI(describeError(q.error).message)}</p>)}
    {formatUI(q.isPending && <p role="status">{textUI("m_c9a8253c0af4")}</p>)}
    {formatUI(state && <>
      {formatUI(!state.allowed_now && <div className="alert" role="status"><strong>{formatUI(state.global_paused || state.paused ? textUI("m_38627573a2b4") : textUI("m_6908d1985507"))}</strong><p>{formatUI(state.restriction)}{formatUI(actor ? textUI("m_99dab5cfbcc8", [actor]) : '')}</p>{formatUI(state.global_paused && org && <p>{textUI("m_2982fa3dcd01")}</p>)}</div>)}
      {formatUI(!compact && <p>{formatUI(state.window.timezone ? `${state.window.days?.map(d => [textUI("m_f40d7f51f69e"), textUI("m_d1eb39b09bf5"), textUI("m_58339f45df96"), textUI("m_7da11212ed34"), textUI("m_66dab40cea1d"), textUI("m_fdeb71b569e0"), textUI("m_db18f17fe532")][d - 1]).join(', ')} · ${state.window.start}–${state.window.end} · ${state.window.timezone}` : textUI("m_a1cce90aaaeb"))}</p>)}
      {formatUI(admin && !compact && <div className="row">{formatUI(org && <button className="btn btn--secondary btn--auto" onClick={() => setEdit(true)}>{textUI("m_a76aba534d4d")}</button>)}<button className="btn btn--danger btn--auto" disabled={change.isPending || state.paused} onClick={() => setConfirm(true)}>{textUI("m_a46f9648f1af")}</button>{formatUI(state.paused && <button className="btn btn--primary btn--auto" disabled={change.isPending} onClick={() => change.mutate('resume')}>{textUI("m_69334d5ad7cf")}</button>)}</div>)}
    </>)}
    {formatUI(change.error && <p className="alert" role="alert">{formatUI(describeError(change.error).message)}</p>)}
    {formatUI(confirm && <Modal labelledBy="scan-stop-title" onClose={() => {
                if (!change.isPending)
                    setConfirm(false);
            }}><div className="dialog__form"><h2 id="scan-stop-title">{textUI("m_0a81905fad31")}</h2><p>{richUI("m_70e9a2eaf49c", [formatUI(org ? textUI("m_f3eca6857685") : textUI("m_ba19359dabec"))])}</p><p>{textUI("m_89217925353b")}</p><footer className="dialog__foot" style={{ flexWrap: 'wrap' }}><button className="btn btn--secondary btn--auto" disabled={change.isPending} onClick={() => setConfirm(false)}>{textUI("m_7267f342e5aa")}</button><button className="btn btn--danger btn--auto" disabled={change.isPending} onClick={() => change.mutate('stop')}>{textUI("m_d8d46235aa34")}</button></footer></div></Modal>)}
    {formatUI(edit && state && org && <WindowDialog org={org} state={state} onClose={() => setEdit(false)}/>)}
  </section>;
}
function WindowDialog({ org, state, onClose }: {
    org: string;
    state: ScanControl;
    onClose: () => void;
}) {
    useLocale();
    const qc = useQueryClient();
    const [enabled, setEnabled] = useState(!!state.window.timezone);
    const [zone, setZone] = useState(state.window.timezone ?? Intl.DateTimeFormat().resolvedOptions().timeZone ?? 'UTC');
    const [days, setDays] = useState(state.window.days ?? [1, 2, 3, 4, 5]);
    const [start, setStart] = useState(state.window.start ?? '09:00');
    const [end, setEnd] = useState(state.window.end ?? '17:00');
    const save = useMutation({ mutationFn: () => api.put(path(org), { version: state.version, window: enabled ? { timezone: zone, days, start, end } : {} }), onSuccess: async () => { await qc.invalidateQueries({ queryKey: ['scan-control'] }); onClose(); } });
    return <Modal labelledBy="scan-window-title" onClose={onClose}><form className="dialog__form" onSubmit={e => { e.preventDefault(); save.mutate(); }}><h2 id="scan-window-title">{textUI("m_2b5555288be8")}</h2><label>{richUI("m_b75a8bf6561f", [<input type="checkbox" checked={enabled} onChange={e => setEnabled(e.target.checked)}/>])}</label>
    {formatUI(enabled && <><div className="field"><label htmlFor="scan-zone">{textUI("m_758f498e1574")}</label><div className="field__input"><input id="scan-zone" required maxLength={64} value={zone} onChange={e => setZone(e.target.value)} placeholder={textUI("m_3b647dcf9395")}/></div></div><fieldset><legend>{textUI("m_d7254b0958e4")}</legend><div className="row">{formatUI([textUI("m_6a00dfc1dc86"), textUI("m_7d8af1de1262"), textUI("m_c0a6cc829cd0"), textUI("m_fc2662062ffd"), textUI("m_e21f3f372a46"), textUI("m_dbe35c7363c2"), textUI("m_873fef760e5d")].map((name, i) => <label key={name}><input type="checkbox" checked={days.includes(i + 1)} onChange={e => setDays(e.target.checked ? [...days, i + 1].sort() : days.filter(d => d !== i + 1))}/> {formatUI(name)}</label>))}</div></fieldset><div className="field"><label htmlFor="scan-start">{textUI("m_5b5e633ee088")}</label><div className="field__input"><input id="scan-start" type="time" required value={start} onChange={e => setStart(e.target.value)}/></div></div><div className="field"><label htmlFor="scan-end">{textUI("m_152a4391bff1")}</label><div className="field__input"><input id="scan-end" type="time" required value={end} onChange={e => setEnd(e.target.value)}/></div></div><p className="field__hint">{textUI("m_26c2cc6525c6")}</p></>)}
    {formatUI(save.error && <p className="alert" role="alert">{formatUI(describeError(save.error).message)}</p>)}<footer className="dialog__foot" style={{ flexWrap: 'wrap' }}><button type="button" className="btn btn--secondary btn--auto" onClick={onClose}>{textUI("m_19766ed6ccb2")}</button><button className="btn btn--primary btn--auto" disabled={save.isPending || (enabled && !days.length)}>{textUI("m_999e6de1598a")}</button></footer></form></Modal>;
}

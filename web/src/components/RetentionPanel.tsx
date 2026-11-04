import { t as textUI, tr as richUI, useLocale, locale, fv as formatUI } from "../i18n/runtime";
import { useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/client';
import { describeError } from './ErrorNote';
export type RetentionPolicy = {
    findings_days: number;
    scan_days: number;
    report_days: number;
    endpoint_days: number;
    audit_days: number;
    version: number;
    last_sweep_at?: string;
    last_error_code?: string;
    cleanup_pending: number;
};
export type DataDeletion = {
    id: string;
    organization_name: string;
    status: 'completed' | 'external_cleanup_pending';
    cleanup_pending: number;
    completed_at?: string;
};
const fields = () => [['findings_days', textUI("m_e171c2ff25b5"), 1], ['scan_days', textUI("m_b5e09d8b7d1e"), 1], ['report_days', textUI("m_761568876601"), 1], ['endpoint_days', textUI("m_91af0e7bcd2a"), 1], ['audit_days', textUI("m_e4d36f9a4e22"), 90]] as const;
export function RetentionPanel() {
    useLocale();
    const qc = useQueryClient();
    const [params] = useSearchParams();
    const operation = params.get('deletion');
    const q = useQuery({ queryKey: ['retention'], queryFn: () => api.get<RetentionPolicy>('/api/v1/retention'), refetchInterval: 5000 });
    const deletion = useQuery({ queryKey: ['data-deletion', operation], enabled: !!operation, queryFn: () => api.get<DataDeletion>(`/api/v1/data-deletions/${encodeURIComponent(operation!)}`), refetchInterval: query => query.state.data?.status === 'completed' ? false : 2000 });
    const [draft, setDraft] = useState<RetentionPolicy>();
    const [saved, setSaved] = useState(false);
    const save = useMutation({ mutationFn: () => api.put<RetentionPolicy>('/api/v1/retention', draft!), onSuccess: async () => { setDraft(undefined); setSaved(true); await qc.invalidateQueries({ queryKey: ['retention'] }); } });
    const policy = draft ?? q.data;
    return <section className="org-card" aria-labelledby="retention-title"><h2 id="retention-title">{textUI("m_71f7f6993a05")}</h2><p>{textUI("m_94573cacc5ea")}</p>
 {formatUI(operation && <section aria-label={textUI("m_5569a97ae17c")}>{formatUI(deletion.isPending ? <p role="status">{textUI("m_6416c5a47450")}</p> : deletion.data ? <p role="status">{formatUI(deletion.data.status === 'completed' ? textUI("m_a8fd4898eb1c", [deletion.data.organization_name]) : textUI("m_36517c9fa019", [deletion.data.organization_name, deletion.data.cleanup_pending]))}</p> : <p role="alert">{formatUI(describeError(deletion.error).message)}</p>)}</section>)}
 {formatUI(q.error && <p className="alert" role="alert">{formatUI(describeError(q.error).message)}</p>)}{formatUI(!policy ? <p role="status">{textUI("m_f32a49e03a2e")}</p> : <form onSubmit={e => { e.preventDefault(); setSaved(false); save.mutate(); }}>
 <div className="org-grid">{formatUI(fields().map(([key, label, min]) => <div className="field" key={key}><label htmlFor={`retention-${key}`}>{richUI("m_0ba9ba9f27e3", [formatUI(label)])}</label><div className="field__input"><input id={`retention-${key}`} type="number" min={min} max={3650} step={1} required value={Number.isFinite(policy[key]) ? policy[key] : ''} onChange={e => { setSaved(false); setDraft({ ...policy, [key]: e.target.valueAsNumber }); }}/></div><p className="field__hint">{formatUI(min === 90 ? textUI("m_b46b17b32188") : textUI("m_8d59e54c3ecd"))}</p></div>))}</div>
 <p className="field__hint">{textUI("m_79c711466703")}</p>
 {formatUI(policy.last_sweep_at && <p className="small">{richUI("m_83ead328651f", [formatUI(new Date(policy.last_sweep_at).toLocaleString(locale())), formatUI(q.data?.cleanup_pending ?? 0)])}</p>)}
 {formatUI(q.data?.last_error_code && <p role="alert" className="alert">{textUI("m_ea2f2bce5e91")}</p>)}
 {formatUI(save.error && <p className="alert" role="alert">{richUI("m_439234e10a9e", [formatUI(describeError(save.error).message)])}</p>)}{formatUI(saved && <p role="status">{textUI("m_edba66b65bfb")}</p>)}
 <div className="row"><button className="btn btn--primary btn--auto" disabled={!draft || save.isPending}>{textUI("m_0370bcb8f79e")}</button><button type="button" className="btn btn--secondary btn--auto" disabled={save.isPending} onClick={() => { setDraft(undefined); save.reset(); void q.refetch(); }}>{textUI("m_5a3ff62bce40")}</button></div></form>)}
 </section>;
}

import { t as textUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { api } from '../api/client';
import { describeError } from './ErrorNote';
type Policy = {
    offline_minutes: number;
    version: number;
};
export function NotificationPolicy() {
    useLocale();
    const qc = useQueryClient();
    const q = useQuery({ queryKey: ['notification-policy'], queryFn: () => api.get<Policy>('/api/v1/alerts/policy'), retry: false });
    const [draft, setDraft] = useState<Policy | null>(null);
    const save = useMutation({ mutationFn: (v: Policy) => api.put<Policy>('/api/v1/alerts/policy', v), onSuccess: v => { qc.setQueryData(['notification-policy'], v); setDraft(null); } });
    if (q.isPending)
        return <p role="status">{textUI("m_a3a5fbf83e64")}</p>;
    if (q.isError)
        return <div className="alert" role="alert"><p>{formatUI(describeError(q.error).message)}</p><button className="btn btn--secondary btn--auto" onClick={() => q.refetch()}>{textUI("m_59c15a3bb0a0")}</button></div>;
    const value = draft ?? q.data!;
    return <section className="panel" aria-labelledby="notification-policy-title"><h2 id="notification-policy-title">{textUI("m_3fb949be7dea")}</h2>
  <p>{textUI("m_e02fd045c60c")}</p>
  <form className="row" onSubmit={e => { e.preventDefault(); save.mutate(value); }}>
   <div className="field"><label htmlFor="offline-minutes">{textUI("m_a1f800468f37")}</label><div className="field__input"><input id="offline-minutes" type="number" min={2} max={10080} required value={Number.isFinite(value.offline_minutes) ? value.offline_minutes : ''} disabled={save.isPending} onChange={e => setDraft({ ...value, offline_minutes: e.target.valueAsNumber })}/></div></div>
   <button className="btn btn--primary btn--auto" disabled={!draft || !Number.isInteger(value.offline_minutes) || value.offline_minutes < 2 || value.offline_minutes > 10080 || save.isPending}>{textUI("m_69e0ea73a469")}</button>
  </form>
  {formatUI(save.isError && <div className="alert" role="alert"><p>{formatUI(describeError(save.error).message)}</p><button className="btn btn--secondary btn--auto" onClick={() => { setDraft(null); save.reset(); void q.refetch(); }}>{textUI("m_bf8e93c67414")}</button></div>)}
 </section>;
}

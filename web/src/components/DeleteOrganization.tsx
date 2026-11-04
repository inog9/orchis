import { t as textUI, useLocale, locale, fv as formatUI } from "../i18n/runtime";
import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/client';
import { Modal } from './Modal';
import { describeError } from './ErrorNote';
import type { DataDeletion } from './RetentionPanel';
export type DeletePreview = {
    id: string;
    name: string;
    version: number;
    counts: Record<'assets' | 'agents' | 'findings' | 'reports' | 'endpoint_datasets' | 'scans' | 'executions', number>;
    blockers: string[];
};
const labels = { get assets() {
        return textUI("m_bd12731d7bc9");
    }, get agents() {
        return textUI("m_279b44d2ab4b");
    }, get findings() {
        return textUI("m_e171c2ff25b5");
    }, get reports() {
        return textUI("m_dacca3cba3f3");
    }, get endpoint_datasets() {
        return textUI("m_91af0e7bcd2a");
    }, get scans() {
        return textUI("m_d128917a9540");
    }, get executions() {
        return textUI("m_93af2616998c");
    } };
function DeleteDialog({ org, onClose }: {
    org: string;
    onClose: () => void;
}) {
    useLocale();
    const navigate = useNavigate();
    const qc = useQueryClient();
    const [name, setName] = useState('');
    const q = useQuery({ queryKey: ['organization-deletion-preview', org], queryFn: () => api.get<DeletePreview>(`/api/v1/organizations/${org}/deletion-preview`) });
    const remove = useMutation({ mutationFn: () => api.del<DataDeletion>(`/api/v1/organizations/${org}`, { name, version: q.data!.version, counts: q.data!.counts }), onSuccess: async (result) => { await qc.invalidateQueries({ queryKey: ['organizations'], refetchType: 'none' }); navigate(`/settings?deletion=${result.id}`); } });
    return <Modal labelledBy="delete-organization-title" onClose={() => { if (!remove.isPending)
        onClose(); }}><form className="dialog__form" onSubmit={e => { e.preventDefault(); if (q.data && name === q.data.name && !q.data.blockers.length && !q.isFetching)
        remove.mutate(); }}><h2 id="delete-organization-title">{textUI("m_4eb0666076d2")}</h2>
 <p>{textUI("m_364cd11e20b3")}</p>
 {formatUI(q.isPending ? <p role="status">{textUI("m_180d8e4d12ee")}</p> : q.data && <><h3>{formatUI(q.data.name)}</h3><dl>{formatUI(Object.entries(q.data.counts).map(([key, value]) => <div className="row" key={key}><dt>{formatUI(labels[key as keyof typeof labels])}</dt><dd>{formatUI(value.toLocaleString(locale()))}</dd></div>))}</dl>{formatUI(q.data.blockers.map(reason => <p key={reason} className="alert" role="alert">{formatUI(reason)}</p>))}<div className="field"><label htmlFor="delete-organization-name">{textUI("m_581a2efd1843")}</label><div className="field__input"><input id="delete-organization-name" required autoComplete="off" value={name} onChange={e => setName(e.target.value)} disabled={remove.isPending}/></div></div><p className="field__hint">{textUI("m_fae62684d4e7")}</p></>)}
 {formatUI((q.error || remove.error) && <p className="alert" role="alert">{formatUI(describeError(q.error || remove.error).message)}</p>)}
 <footer className="dialog__foot row"><button type="button" className="btn btn--secondary btn--auto" disabled={remove.isPending} onClick={onClose}>{textUI("m_19766ed6ccb2")}</button><button type="button" className="btn btn--secondary btn--auto" disabled={remove.isPending || q.isFetching} onClick={() => { remove.reset(); void q.refetch(); }}>{textUI("m_f32e8dcbb75e")}</button><button className="btn btn--danger btn--auto" disabled={!q.data || !!q.data.blockers.length || name !== q.data.name || q.isFetching || remove.isPending}>{formatUI(remove.isPending ? textUI("m_43b5894c3e4e") : textUI("m_03bd5e5f96a9"))}</button></footer>
 </form></Modal>;
}
export function DeleteOrganization({ org }: {
    org: string;
}) { useLocale(); const [open, setOpen] = useState(false); return <section className="org-card" aria-labelledby="organization-delete-heading"><h2 id="organization-delete-heading">{textUI("m_03bd5e5f96a9")}</h2><p>{textUI("m_9daa08330fa8")}</p><button className="btn btn--danger btn--auto" onClick={() => setOpen(true)}>{textUI("m_07608d098a97")}</button>{formatUI(open && <DeleteDialog org={org} onClose={() => setOpen(false)}/>)}</section>; }

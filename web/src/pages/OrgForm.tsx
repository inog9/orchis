import { useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/client';
import { Modal } from '../components/Modal';
import { describeError } from '../components/ErrorNote';
import { t as textUI, useLocale, fv as formatUI } from '../i18n/runtime';

export type Org = {
    id: string;
    name: string;
    slug: string;
    description: string;
    industry: string;
    contact: string;
    version: number;
};
export function OrgForm({ current, onClose }: {
    current?: Org;
    onClose: () => void;
}) {
    useLocale();
    const qc = useQueryClient();
    const [slugEdited, setSlugEdited] = useState(!!current);
    const [form, setForm] = useState({ name: current?.name ?? '', slug: current?.slug ?? '', description: current?.description ?? '', industry: current?.industry ?? '', contact: current?.contact ?? '' });
    const save = useMutation({ mutationFn: () => current ? api.patch<Org>(`/api/v1/organizations/${current.id}`, { ...form, version: current.version }) : api.post<Org>('/api/v1/organizations', form), onSuccess: async () => { await Promise.all(['organizations', 'setup-organizations', 'home'].map(key => qc.invalidateQueries({ queryKey: [key] }))); onClose(); } });
    return <Modal labelledBy="org-form-title" onClose={onClose}><form className="dialog__form" onSubmit={e => { e.preventDefault(); save.mutate(); }}><h2 id="org-form-title">{formatUI(current ? textUI("m_e735f16f97b6") : textUI("m_1c3b4bf6ffd9"))}</h2>
    {formatUI((['name', 'slug', 'industry', 'contact', 'description'] as const).map(key => <div className="field" key={key}><label htmlFor={`org-${key}`}>{formatUI(key.charAt(0).toUpperCase() + key.slice(1))}</label><div className="field__input"><input id={`org-${key}`} required={key === 'name' || key === 'slug'} maxLength={key === 'description' ? 4000 : key === 'contact' ? 500 : key === 'slug' ? 100 : 200} value={form[key]} onChange={e => { if (key === 'slug')
        setSlugEdited(true); setForm({ ...form, [key]: e.target.value, ...(key === 'name' && !slugEdited ? { slug: e.target.value.toLowerCase().normalize('NFKD').replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '') } : {}) }); }}/></div></div>))}
    <p className="field__hint">{textUI("m_505233cf57b9")}</p>{save.error && <p className="alert" role="alert">{formatUI(describeError(save.error).message)}</p>}<footer className="dialog__foot"><button type="button" className="btn btn--secondary btn--auto" onClick={onClose}>{textUI("m_19766ed6ccb2")}</button><button className="btn btn--primary btn--auto" disabled={save.isPending}>{textUI("m_f55d1a1c73b8")}</button></footer></form></Modal>;
}

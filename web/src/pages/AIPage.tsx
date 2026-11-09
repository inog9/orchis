import { t as textUI, tr as richUI, useLocale, locale, fv as formatUI } from "../i18n/runtime";
import { useRef, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/client';
import { useMe } from '../api/hooks';
import { can } from '../api/types';
import { describeError } from '../components/ErrorNote';
import { Modal } from '../components/Modal';
export type AIPolicy = {
    is_demo?: boolean;
    version: number;
    enabled: boolean;
    planning: boolean;
    remediation: boolean;
    auto_approve?: boolean;
    planning_model: string;
    remediation_model: string;
    max_input_bytes: number;
    max_output_tokens: number;
    daily_tokens: number;
    daily_calls: number;
    daily_cost_micros: number;
    input_rate_micros: number;
    output_rate_micros: number;
    has_keys: boolean;
    configured: boolean;
};
type AIOutput = {
    summary: string;
    remediations: {
        finding_id: string;
        recommendation: string;
        validation: string;
    }[];
    actions: {
        asset_id: string;
        contract_key: string;
        version: number;
        params: unknown;
        reason: string;
    }[];
};
type Job = {
    is_demo?: boolean;
    id: string;
    organization_id: string;
    report_id: string;
    kind: string;
    provider: string;
    model: string;
    status: string;
    version: number;
    prompt_version: number;
    instructions: string;
    input: {
        assets: {
            id: string;
            findings: {
                id: string;
                type: string;
                severity: string;
                cves: string[];
            }[];
        }[];
    };
    output: AIOutput | null;
    error: string;
    reserved_tokens: number;
    reserved_cost_micros: number;
    references?: {
        assets: Record<string, string>;
        findings: Record<string, string>;
        asset_labels: Record<string, string>;
        finding_labels: Record<string, string>;
        finding_targets: Record<string, string>;
    };
};
type Template = {
    is_demo?: boolean;
    id: string;
    kind: string;
    version: number;
    instructions: string;
};
function ErrorText({ error }: {
    error: unknown;
}) { useLocale(); return error ? <p role="alert" className="alert">{formatUI(describeError(error).message)}</p> : null; }
export function AIPolicyForm({ value, org }: {
    value: AIPolicy;
    org: string;
}) {
    useLocale();
    const qc = useQueryClient();
    const [form, setForm] = useState(value);
    const [deepseek, setDeepseek] = useState('');
    const [openai, setOpenai] = useState('');
    const [consent, setConsent] = useState(false);
    const save = useMutation({ mutationFn: () => api.put(`/api/v1/organizations/${org}/ai`, { ...form, deepseek_key: deepseek, openai_key: openai, consent }), onSuccess: async () => { setDeepseek(''); setOpenai(''); setConsent(false); await qc.invalidateQueries({ queryKey: ['ai-policy', org] }); await qc.invalidateQueries({ queryKey: ['ai-jobs', org] }); } });
    if (value.is_demo)
        return <p className="notice">{textUI("m_3c426930ccc5")}</p>;
    return <form className="panel dialog__form ai-policy" onSubmit={e => { e.preventDefault(); save.mutate(); }} aria-label={textUI("m_5d0c94144be1")}><h2>{textUI("m_943d59944793")}</h2><p>{textUI("m_33d8a01454f3")}</p>
 {formatUI(!value.configured && <p className="notice">{textUI("m_bc818e7c973f")}</p>)}
 <label className="checkbox">{richUI("m_067fb71becbe", [<input type="checkbox" checked={form.enabled} onChange={e => setForm({ ...form, enabled: e.target.checked })}/>])}</label>
 <label className="checkbox">{richUI("m_5cff46dfb20f", [<input type="checkbox" checked={form.planning} onChange={e => setForm({ ...form, planning: e.target.checked })}/>])}</label>
 <label className="checkbox">{richUI("m_f19725c1bfc3", [<input type="checkbox" checked={form.remediation} onChange={e => setForm({ ...form, remediation: e.target.checked })}/>])}</label>
    <label className="checkbox">{richUI("m_a9f01c2e7b3d", [<input type="checkbox" checked={!!form.auto_approve} onChange={e => setForm({ ...form, auto_approve: e.target.checked })}/>])}</label>
 <div className="org-grid">{formatUI((['planning_model', 'remediation_model'] as const).map(k => <label className="field" key={k}>{formatUI(k === 'planning_model' ? textUI("m_d2a9a398bcb6") : textUI("m_0b66798d2e62"))}<input value={form[k]} maxLength={100} required={form[k === 'planning_model' ? 'planning' : 'remediation']} onChange={e => setForm({ ...form, [k]: e.target.value })}/></label>))}</div>
 <div className="org-grid"><label className="field">{richUI("m_38e7bb021752", [<input type="password" autoComplete="new-password" value={deepseek} maxLength={512} onChange={e => setDeepseek(e.target.value)}/>])}</label><label className="field">{richUI("m_a026b37a0a83", [<input type="password" autoComplete="new-password" value={openai} maxLength={512} onChange={e => setOpenai(e.target.value)}/>])}</label></div>
 <p className="muted small">{textUI("m_8de07201c08e")}</p>
 <div className="org-grid">{formatUI(([{ key: 'max_input_bytes', label: textUI("m_936bbf213187"), max: 64000, min: 1024 }, { key: 'max_output_tokens', label: textUI("m_58300b3d2930"), max: 4096, min: 128 }, { key: 'daily_tokens', label: textUI("m_014f76f105c1"), max: 1000000, min: 128 }, { key: 'daily_calls', label: textUI("m_60192e2ca954"), max: 1000, min: 1 }] as const).map(f => <label className="field" key={f.key}>{formatUI(f.label)}<input type="number" required min={f.min} max={f.max} value={form[f.key]} onChange={e => setForm({ ...form, [f.key]: Number(e.target.value) })}/></label>))}</div>
 <div className="org-grid">{formatUI(([{ key: 'daily_cost_micros', label: textUI("m_8de20a1a8c71"), max: 100 }, { key: 'input_rate_micros', label: textUI("m_988227b6151d"), max: 1000 }, { key: 'output_rate_micros', label: textUI("m_05af5837c979"), max: 1000 }] as const).map(f => <label className="field" key={f.key}>{formatUI(f.label)}<input type="number" step="0.000001" required min={0} max={f.max} value={form[f.key] / 1000000} onChange={e => setForm({ ...form, [f.key]: Math.round(Number(e.target.value) * 1000000) })}/></label>))}</div>
 <p className="muted small">{textUI("m_a013462b5ef8")}</p>
 <label className="checkbox">{richUI("m_f3cba9962f84", [<input type="checkbox" checked={consent} onChange={e => setConsent(e.target.checked)}/>])}</label>
 <ErrorText error={save.error}/><button className="btn btn--primary btn--auto" disabled={save.isPending || (form.enabled && (!consent || !value.configured))}>{textUI("m_953eeb5f1fd9")}</button></form>;
}
export function AIPage() {
    useLocale();
    const { id = '' } = useParams();
    const { data: me } = useMe();
    const qc = useQueryClient();
    const [report, setReport] = useState('');
    const [kind, setKind] = useState('planning');
    const [template, setTemplate] = useState('');
    const [preview, setPreview] = useState(false);
    const [cursor, setCursor] = useState('');
    const key = useRef(crypto.randomUUID());
    const policy = useQuery({ queryKey: ['ai-policy', id], queryFn: () => api.get<AIPolicy>(`/api/v1/organizations/${id}/ai`) });
    const reports = useQuery({ queryKey: ['ai-reports', id], queryFn: () => api.get<{
            items: {
                id: string;
                created_at: string;
            }[];
        }>(`/api/v1/reports?organization_id=${id}`) });
    const jobs = useQuery({ queryKey: ['ai-jobs', id, cursor], queryFn: () => api.get<{
            items: Job[];
            next_cursor: string;
        }>(`/api/v1/organizations/${id}/ai/jobs?cursor=${encodeURIComponent(cursor)}`), refetchInterval: 3000 });
    const templates = useQuery({ queryKey: ['ai-templates'], queryFn: () => api.get<{
            items: Template[];
            builtin_instructions: string;
            builtin_version: number;
        }>('/api/v1/ai/templates') });
    const data = useQuery({ queryKey: ['ai-preview', id, report], enabled: preview && !!report && can(me, 'ai.generate'), queryFn: () => api.get(`/api/v1/organizations/${id}/ai/preview?report_id=${report}`) });
    const create = useMutation({ mutationFn: () => api.postIdempotent<Job>(`/api/v1/organizations/${id}/ai/jobs`, { report_id: report, kind, template_id: template || null }, key.current), onSuccess: async () => { key.current = crypto.randomUUID(); await qc.invalidateQueries({ queryKey: ['ai-jobs', id] }); } });
    const [text, setText] = useState('');
    const version = useMutation({ mutationFn: () => api.post('/api/v1/ai/templates', { kind, instructions: text }), onSuccess: async () => { setText(''); await qc.invalidateQueries({ queryKey: ['ai-templates'] }); } });
    if (policy.isPending)
        return <p role="status">{textUI("m_b99ce098c55c")}</p>;
    if (!policy.data)
        return <ErrorText error={policy.error}/>;
    const pol = policy.data;
    return <section className="page ai-page" aria-labelledby="ai-title"><Link to={`/organizations/${id}`}>{textUI("m_7f371c34a81e")}</Link><h1 id="ai-title" className="page__title">{textUI("m_22a65461bf9b")}</h1><p className="page__sub">{textUI("m_93a58bc3a5b7")}</p>
 <p className="notice">{richUI("m_8dd696a9de8f", [formatUI(pol.enabled ? textUI("m_8ae925d43ba2") : 'disabled')])}</p>
 {formatUI(pol.is_demo ? <p className="notice">{textUI("m_c0262f46a853")}</p> : can(me, 'ai.manage') ? <details className="panel" open={!pol.enabled}><summary>{textUI("m_f5d9c6cf9ac5")}</summary><AIPolicyForm key={pol.version} value={pol} org={id}/></details> : <p className="muted">{textUI("m_cfc47d9ff0a0")}</p>)}
 {formatUI(can(me, 'ai.generate') && <section className="panel"><h2>{textUI("m_98cea0ccbe57")}</h2><p>{textUI("m_705546ff12ae")}</p><div className="org-grid"><label className="field">{richUI("m_9e4d5655fef9", [<select aria-label={textUI("m_279598d48f17")} value={report} onChange={e => { setReport(e.target.value); setPreview(false); key.current = crypto.randomUUID(); }}><option value="">{textUI("m_3c5a1b10f785")}</option>{formatUI(reports.data?.items.map(r => <option key={r.id} value={r.id}>{formatUI(new Date(r.created_at).toLocaleString(locale()))} · {formatUI(r.id.slice(0, 8))}</option>))}</select>])}</label><label className="field">{richUI("m_d7639f09c682", [<select aria-label={textUI("m_d4e8830a71c7")} value={kind} onChange={e => { setKind(e.target.value); setTemplate(''); key.current = crypto.randomUUID(); }}><option value="planning">{textUI("m_7f89e6c4a108")}</option><option value="remediation">{textUI("m_8ff11cb17c84")}</option></select>])}</label><label className="field">{richUI("m_79a822f340e5", [<select aria-label={textUI("m_ad38cdd3cf6b")} value={template} onChange={e => { setTemplate(e.target.value); key.current = crypto.randomUUID(); }}><option value="">{textUI("m_62c629bb525e")}</option>{formatUI(templates.data?.items.filter(t => t.kind === kind).map(t => <option key={t.id} value={t.id}>{richUI("m_2f6e868a0e76", [formatUI(t.is_demo ? textUI("m_8a2cc0673b1c") : textUI("m_494ca78f7374")), formatUI(t.version)])}</option>))}</select>])}</label></div>
 {formatUI(!reports.isPending && reports.data?.items.length === 0 && <p>{richUI("m_252a85155756", [<Link to={`/organizations/${id}`}>{textUI("m_6f47043dd57e")}</Link>])}</p>)}
 <ErrorText error={reports.error}/><ErrorText error={templates.error}/><ErrorText error={data.error}/><ErrorText error={create.error}/><div className="report-relations"><button className="btn btn--secondary btn--auto" disabled={!report} onClick={() => setPreview(true)}>{textUI("m_83115d387962")}</button><button className="btn btn--primary btn--auto" disabled={pol.is_demo || !report || !pol.enabled || !pol.configured || !(kind === 'planning' ? pol.planning : pol.remediation) || create.isPending} onClick={() => create.mutate()}>{richUI("m_31a405e16fbe", [formatUI(kind === 'planning' ? textUI("m_0648d2111ec0") : textUI("m_8b7d1a3187ab"))])}</button></div>{formatUI(preview && data.isPending && <p role="status">{textUI("m_c4cf2b2ccb5d")}</p>)}{formatUI(preview && data.data !== undefined && <details open><summary>{textUI("m_032a74b6da59")}</summary><pre className="ai-json" tabIndex={0}>{JSON.stringify(data.data, null, 2)}</pre></details>)}</section>)}
 <section className="panel"><h2>{textUI("m_504b99bcd7b8")}</h2><ErrorText error={jobs.error}/>{formatUI(jobs.isPending ? <p role="status">{textUI("m_70d1028b2a5e")}</p> : jobs.data?.items.length === 0 ? <p>{textUI("m_c1007849790d")}</p> : <ul className="mini-list">{formatUI(jobs.data?.items.map(j => <li key={j.id}><Link to={`/ai/jobs/${j.id}`}>{formatUI(j.is_demo ? textUI("m_4a6e0dc942e8") : '')}{formatUI(j.kind)} · {formatUI(j.provider)} · {formatUI(j.status)}</Link><span>{formatUI(j.is_demo ? textUI("m_59615086fb88") : <>{textUI("m_3385ffe67470")}{" "}{formatUI(j.reserved_tokens)}{" "}{textUI("m_80ed2e93fe53")}{formatUI((j.reserved_cost_micros / 1000000).toFixed(4))}</>)}</span></li>))}</ul>)}<div className="report-relations"><button className="btn btn--secondary btn--auto" disabled={!cursor} onClick={() => setCursor('')}>{textUI("m_a2630913e7a5")}</button><button className="btn btn--secondary btn--auto" disabled={!jobs.data?.next_cursor} onClick={() => setCursor(jobs.data!.next_cursor)}>{textUI("m_40e1af5dd932")}</button></div></section>
 <details className="panel"><summary>{textUI("m_7d04c8348373")}</summary><p>{textUI("m_563a8d7192e2")}</p><pre className="ai-json" tabIndex={0}>{templates.data?.builtin_instructions}</pre>{formatUI(templates.data?.items.map(t => <article key={t.id}><h3>{richUI("m_d8d76e73f9b9", [formatUI(t.kind), formatUI(t.version)])}</h3><p>{formatUI(t.instructions)}</p></article>))}{formatUI(can(me, 'ai.manage') && !pol.is_demo && <form onSubmit={e => { e.preventDefault(); version.mutate(); }}><label className="field">{richUI("m_24800e1fcf50", [formatUI(kind), <textarea required maxLength={6000} value={text} onChange={e => setText(e.target.value)}/>])}</label><ErrorText error={version.error}/><button className="btn btn--secondary btn--auto" disabled={version.isPending}>{textUI("m_6a4c468d8325")}</button></form>)}</details></section>;
}
export function AIJobPage() {
    useLocale();
    const { id = '' } = useParams();
    const { data: me } = useMe();
    const qc = useQueryClient();
    const [confirm, setConfirm] = useState(false);
    const query = useQuery({ queryKey: ['ai-job', id], queryFn: () => api.get<Job>(`/api/v1/ai/jobs/${id}`), refetchInterval: 2000 });
    const j = query.data;
    const action = useMutation({ mutationFn: (op: string) => api.post(`/api/v1/ai/jobs/${id}/${op}`, { version: j?.version }), onSuccess: async () => { setConfirm(false); await qc.invalidateQueries({ queryKey: ['ai-job', id] }); } });
    if (query.isPending)
        return <p role="status">{textUI("m_1f8adee9dc33")}</p>;
    if (!j)
        return <ErrorText error={query.error}/>;
    return <section className="page ai-page" aria-labelledby="ai-job-title"><Link to={`/organizations/${j.organization_id}/ai`}>{textUI("m_8034d44292eb")}</Link><h1 id="ai-job-title" className="page__title">{textUI("m_51184e1a2725")}</h1><p className="page__sub">{formatUI(j.provider)} · {formatUI(j.model)} · {formatUI(j.kind)} · {formatUI(j.status)}</p>{formatUI(j.is_demo && <p className="notice">{richUI("m_49e6e850cdda", [<strong>{textUI("m_46022ab89908")}</strong>])}</p>)}<p className="notice">{textUI("m_a002b2756c85")}</p><Link to={`/reports/${j.report_id}`}>{textUI("m_a076f6c896d2")}</Link><ErrorText error={query.error}/><ErrorText error={action.error}/>{formatUI(j.error && <p role="alert">{formatUI(j.error)}</p>)}
 <div className="report-relations">{formatUI(!j.is_demo && (['queued', 'running', 'review'].includes(j.status) || (j.status === 'approved' && can(me, 'atomic.run'))) && can(me, 'ai.generate') && <button className="btn btn--secondary btn--auto" disabled={action.isPending} onClick={() => action.mutate('cancel')}>{formatUI(j.status === 'approved' ? textUI("m_e56cb570b3d3") : textUI("m_5603cee8be45"))}</button>)}{formatUI(!j.is_demo && j.status === 'review' && can(me, 'ai.approve') && can(me, 'atomic.run') && can(me, 'atomic.write') && <button className="btn btn--primary btn--auto" onClick={() => setConfirm(true)}>{textUI("m_432f31c9f6a5")}</button>)}{formatUI(j.output && ['pdf', 'json'].map(f => <a key={f} className="btn btn--secondary btn--auto" href={`/api/v1/ai/jobs/${id}/export/${f}`}>{richUI("m_0c7425ad9d5f", [formatUI(f.toUpperCase())])}</a>))}</div>
 {formatUI(j.kind === 'planning' && j.status === 'completed' && j.output?.actions.length === 0 && <p>{textUI("m_b72d8229fec8")}</p>)}{formatUI(j.status === 'queued' && <p>{textUI("m_bb1037803a2f")}</p>)}{formatUI(j.status === 'running' && <p role="status">{textUI("m_7e4968cd6c72")}</p>)}{formatUI(j.status === 'approved' && <p>{richUI("m_a6bbcb40cc07", [<Link to="/atomic">{textUI("m_0cad6d8beb6c")}</Link>])}</p>)}
 {formatUI(j.output && <><h2>{textUI("m_305f5e5463e1")}</h2><p>{formatUI(j.output.summary)}</p><div className="org-grid">{formatUI(j.output.remediations.map(r => <article className="org-card" key={r.finding_id}><h3>{formatUI(j.references?.finding_labels?.[r.finding_id] || r.finding_id)}</h3><p className="muted small">{richUI("m_4e05751314e3", [formatUI(r.finding_id), formatUI(j.references?.finding_targets?.[r.finding_id])])}</p><p>{formatUI(r.recommendation)}</p><p>{richUI("m_0efe38309f39", [formatUI(r.validation)])}</p></article>))}</div><h2>{richUI("m_7a0066dabd0a", [formatUI(j.output.actions.length)])}</h2>{formatUI(j.output.actions.map((a, i) => <article className="panel" key={i}><h3>{richUI("m_d8d76e73f9b9", [formatUI(a.contract_key), formatUI(a.version)])}</h3><p>{richUI("m_751567dd4076", [formatUI(a.asset_id), formatUI(j.references?.asset_labels?.[a.asset_id] && <> · {formatUI(j.references.asset_labels[a.asset_id])}</>), formatUI(j.references?.assets[a.asset_id] && <> · <Link to={`/assets/${j.references.assets[a.asset_id]}`}>{textUI("m_ea95c2b37ca8")}</Link></>)])}</p><p>{formatUI(a.reason)}</p><pre className="ai-json" tabIndex={0}>{JSON.stringify(a.params, null, 2)}</pre></article>))}</>)}
 <details className="panel"><summary>{textUI("m_46a0f07809ca")}</summary><p>{richUI("m_e80a20a5b463", [formatUI(j.prompt_version), formatUI(j.reserved_tokens), formatUI((j.reserved_cost_micros / 1000000).toFixed(4))])}</p><pre className="ai-json" tabIndex={0}>{j.instructions}</pre><pre className="ai-json" tabIndex={0}>{JSON.stringify(j.input, null, 2)}</pre></details>
 {formatUI(confirm && <Modal labelledBy="ai-approval-title" onClose={() => setConfirm(false)}><h2 id="ai-approval-title">{textUI("m_45f833214b33")}</h2><p>{richUI("m_47f8161f5d27", [formatUI(j.output?.actions.length ?? 0)])}</p><ErrorText error={action.error}/><div className="dialog__foot"><button className="btn btn--secondary btn--auto" onClick={() => setConfirm(false)}>{textUI("m_9b50f21a5bad")}</button><button className="btn btn--primary btn--auto" disabled={action.isPending} onClick={() => action.mutate('approve')}>{textUI("m_4b4c69cb34ed")}</button></div></Modal>)}</section>;
}

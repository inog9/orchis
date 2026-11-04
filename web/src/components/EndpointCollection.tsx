import {language} from '../i18n/runtime';
import {endpointCopy} from '../i18n/endpoint';
import { t as textUI, tr as richUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/client';
import { useEndpointCatalog, useOrgOsquery } from '../api/hooks';
import { describeError } from './ErrorNote';
import { Modal } from './Modal';
export function EndpointCollection({ org, platform, arch, canManage }: {
    org: string;
    platform: string;
    arch: string;
    canManage: boolean;
}) {
    useLocale();
    const cat = useEndpointCatalog();
    const settings = useOrgOsquery(org);
    const qc = useQueryClient();
    const [open, setOpen] = useState(false);
    const [selected, setSelected] = useState<string[]>([]);
    const [versions, setVersions] = useState<Record<string, number>>({});
    const [accepted, setAccepted] = useState(false);
    const supported = platform === 'linux' && (arch === 'x86_64' || arch === 'amd64');
    const queries = cat.data?.queries.filter(q => q.os.includes('linux')) ?? [];
    const save = useMutation({ mutationFn: () => api.put(`/api/v1/organizations/${org}/endpoint-collection`, { platform: 'linux', keys: selected, versions, consent: { version: cat.data!.consent.version, sha256: cat.data!.consent.sha256 } }), onSuccess: async () => { await Promise.all([qc.invalidateQueries({ queryKey: ['osquery-org', org] }), qc.invalidateQueries({ queryKey: ['asset-endpoint'] })]); setOpen(false); } });
    const review = () => { const current = settings.data!.items; const states = new Map(current.map(q => [q.key, q])); const hasCurrent = current.some(q => q.consent_version === cat.data!.consent.version && q.consent_sha256 === cat.data!.consent.sha256); setSelected(queries.filter(q => !hasCurrent || states.get(q.key)?.enabled).map(q => q.key)); setVersions(Object.fromEntries(queries.map(q => [q.key, states.get(q.key)?.version ?? 0]))); setAccepted(false); save.reset(); setOpen(true); };
    return <section className="org-card" aria-label={textUI("m_6836bbf4c02c")}><h3>{textUI("m_5237f7a69c42")}</h3>{formatUI(supported ? <><p>{richUI("m_c27aead27749", [formatUI((cat.data?.collection_interval_seconds ?? 900) / 60)])}</p><p className="muted">{textUI("m_aff06ec9d4dd")}</p>{formatUI(canManage && <button className="btn btn--secondary btn--auto" disabled={!cat.data || !settings.data || cat.isError || settings.isError} onClick={review}>{textUI("m_b1b05980900e")}</button>)}</> : <p className="notice">{richUI("m_e2626da97752", [formatUI(platform === 'macos' || platform === 'darwin' ? 'macOS' : platform), formatUI(arch)])}</p>)}{formatUI((cat.error || settings.error) && <p role="alert">{formatUI(describeError(cat.error || settings.error).message)}</p>)}{formatUI(open && cat.data && <Modal labelledBy="collection-consent-title" onClose={() => setOpen(false)} width={640}><form className="collection-consent" onSubmit={e => { e.preventDefault(); if (accepted)
        save.mutate(); }}><h2 id="collection-consent-title">{textUI("m_44837487680d")}</h2><p>{textUI("m_8a2ae8f3faa1")}</p><fieldset className="fieldset collection-query-list" tabIndex={0}><legend>{textUI("m_861ad5df2648")}</legend>{formatUI(queries.map(q => <label className="check" key={q.key}><input type="checkbox" checked={selected.includes(q.key)} onChange={e => setSelected(s => e.target.checked ? [...s, q.key] : s.filter(key => key !== q.key))}/><span className="check__box" aria-hidden="true"/><span>{formatUI(endpointCopy(q.title))} · {formatUI(q.sensitivity)} · {formatUI(q.tables.join(', '))}</span></label>))}</fieldset><div className="ed-consent-text" role="region" aria-label={textUI("m_b15830f5aab9")} tabIndex={0}>{formatUI(cat.data.consent.translations?.[language()] ?? cat.data.consent.text)}</div><label className="check"><input type="checkbox" checked={accepted} onChange={e => setAccepted(e.target.checked)}/><span className="check__box" aria-hidden="true"/><span>{richUI("m_f7ca60612058", [formatUI(cat.data.consent.version)])}</span></label>{formatUI(save.error && <p className="alert" role="alert">{formatUI(describeError(save.error).message)}</p>)}<footer className="dialog__foot"><button className="btn btn--secondary btn--auto" type="button" onClick={() => setOpen(false)}>{textUI("m_19766ed6ccb2")}</button><button className="btn btn--primary btn--auto" disabled={!accepted || save.isPending}>{textUI("m_7c8f6db3fcd4")}</button></footer></form></Modal>)}</section>;
}

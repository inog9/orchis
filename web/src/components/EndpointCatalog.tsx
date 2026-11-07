import { catalogOS, collectionSupported } from '../lib/collectionSupport';
import {displayLabel} from '../i18n/display';
import { language } from '../i18n/runtime';
import { endpointCopy } from '../i18n/endpoint';
import { t as textUI, tr as richUI, useLocale, locale, fv as formatUI } from "../i18n/runtime";
import { useState } from 'react';
import { useEndpointCatalog, useOrgOsquery, useSetOrgOsquery, useSetOrgPack } from '../api/hooks';
import type { EndpointPack, EndpointQueryMeta, EndpointConsent } from '../api/types';
import { describeError } from './ErrorNote';
import { Modal } from './Modal';
const SENSITIVITY_TEXT = { get low() {
        return textUI("m_91578bea4a1b");
    }, get medium() {
        return textUI("m_f501a9327f6a");
    }, get high() {
        return textUI("m_51227b743205");
    } } as const;
const COL_LIMIT = 5;
function Sev({ s }: {
    s: string;
}) {
    useLocale();
    return <span className={`sev sev--${s}`}>{formatUI(s)}</span>;
}
function Example({ q }: {
    q: EndpointQueryMeta;
}) {
    useLocale();
    const cols = q.columns.slice(0, COL_LIMIT);
    return (<div className="ed-example">
      <h4 className="ed-h4">{textUI("m_acfd94342f9e")}</h4>
      <div className="ed-scroll" role="region" aria-label={textUI("m_ae1ee451e297", [q.title])} tabIndex={0}>
        <table className="org-table">
          <thead><tr>{formatUI(cols.map((c) => <th key={c}>{formatUI(c)}</th>))}</tr></thead>
          <tbody>{formatUI(q.example.map((row, i) => <tr key={i}>{formatUI(cols.map((c) => <td key={c} className="mono">{formatUI(row[c] ?? '')}</td>))}</tr>))}</tbody>
        </table>
      </div>
      {formatUI(q.example_findings.length > 0 ? (<>
          <h4 className="ed-h4">{textUI("m_006a543139cc")}</h4>
          <ul className="stack-list">{formatUI(q.example_findings.map((f) => <li key={f.rule + f.title}><Sev s={f.severity}/> {formatUI(endpointCopy(f.title))}</li>))}</ul>
        </>) : <p className="muted small">{textUI("m_ff182b54cbe1")}</p>)}
    </div>);
}
function Card({ q, state, canManage, onToggle }: {
    q: EndpointQueryMeta;
    state?: {
        enabled: boolean;
        explicit: boolean;
    };
    canManage: boolean;
    onToggle?: (q: EndpointQueryMeta, enabled: boolean) => void;
}) {
    useLocale();
    const orgMode = state !== undefined;
    const status = !orgMode ? null : state.enabled ? (state.explicit ? textUI("m_103e0206bb60") : textUI("m_e1589716e19c")) : (state.explicit ? textUI("m_57b7498527cb") : textUI("m_db0e960b68b5"));
    return (<article className="org-card ed-card" aria-label={q.title}>
      <div className="ed-card__head">
        <div>
          <h3 className="ed-title">{formatUI(endpointCopy(q.title))}</h3>
          <p className="muted">{formatUI(endpointCopy(q.description))}</p>
        </div>
        {formatUI(orgMode && (<div className="ed-state">
            <span className={`status ${state.enabled ? 'status--ok' : ''}`}>{formatUI(status)}</span>
            {formatUI(canManage && onToggle && (<button type="button" className="btn btn--secondary btn--sm btn--auto" onClick={() => onToggle(q, !state.enabled)}>{formatUI(state.enabled ? textUI("m_06f0e210b27d") : textUI("m_29bbb3551e65"))}</button>))}
          </div>))}
      </div>
      <p className="ed-badges">
        <span className="tag">{formatUI(SENSITIVITY_TEXT[q.sensitivity])}</span>
        <span className="tag">{richUI("m_37d552e3393f", [formatUI(q.load)])}</span>
        <span className="tag">{formatUI(q.os.includes('linux') ? textUI("m_808e000d1465") : q.os.join(', '))}</span>
        <span className="tag">{textUI("m_9def9e814293")}</span>
        {formatUI(q.needs_root && <span className="tag">{textUI("m_2e6f0f0f43f5")}</span>)}
      </p>
      <details className="ed-details">
        <summary>{textUI("m_e158914d575b")}</summary>
        <div className="ed-grid">
          <div>
            <h4 className="ed-h4">{textUI("m_8fd994bd9a88")}</h4>
            <p className="ed-tables">{formatUI(q.tables.map((t) => <code key={t} className="tag">{t}</code>))}</p>
            <p className="muted small">{textUI("m_e6631dd965c6")}</p>
            <h4 className="ed-h4">{textUI("m_376c95dea204")}</h4>
            <p className="ed-tables">{formatUI(q.attributes.map((a) => <code key={a} className="tag">{a}</code>))}</p>
          </div>
          <div>
            <h4 className="ed-h4">{textUI("m_351c1047ee00")}</h4>
            {formatUI(q.findings.length === 0 ? <p className="muted small">{textUI("m_ca93fc561904")}</p> : (<ul className="stack-list">{formatUI(q.findings.map((f) => <li key={f.rule}><Sev s={f.severity}/> {formatUI(endpointCopy(f.title))}<br /><span className="muted small">{formatUI(endpointCopy(f.explain))}</span></li>))}</ul>))}
            <h4 className="ed-h4">{textUI("m_0259f2cc705f")}</h4>
            <ul className="stack-list">{formatUI(q.mitre.map((m) => <li key={m.id}><strong>{formatUI(m.id)}</strong> <span className="muted small">{formatUI(endpointCopy(m.reason))}</span></li>))}</ul>
            <p className="muted small">{richUI("m_fbc8e4f3a981", [formatUI(displayLabel(q.report_section))])}</p>
          </div>
        </div>
        <Example q={q}/>
      </details>
    </article>);
}
const PACK_STATUS = { get runnable() {
        return textUI("m_848f54e89660");
    }, get sensitive() {
        return textUI("m_7c8635e8d58b");
    }, get not_reviewed() {
        return textUI("m_af3b623c7996");
    }, get other_os() {
        return textUI("m_264d6a8e7f4a");
    }, get unverified() {
        return textUI("m_0dc3a036ae5f");
    } } as const;
const PACK_ORDER = { runnable: 0, sensitive: 1, not_reviewed: 2, unverified: 3, other_os: 4 } as const;
function PackCard({ pack, enabledCount, canManage, orgMode, onToggle }: {
    pack: EndpointPack;
    enabledCount: number;
    canManage: boolean;
    orgMode: boolean;
    onToggle: (p: EndpointPack, enabled: boolean) => void;
}) {
    useLocale();
    const runs = pack.queries.filter((q) => q.status === 'runnable').length;
    const skipped = pack.queries.length - runs;
    const on = runs > 0 && enabledCount === runs;
    const state = runs === 0 ? textUI("m_02420fe5f66a") : on ? textUI("m_103e0206bb60") : enabledCount > 0 ? textUI("m_7b7410fc2e9b") : textUI("m_db0e960b68b5");
    return (<article className="org-card ed-card" aria-label={textUI("m_553de41c1aae", [pack.name])}>
      <div className="ed-card__head">
        <div>
          <h3 className="ed-title">{richUI("m_553de41c1aae", [formatUI(pack.name)])}</h3>
          <p className="muted">{formatUI(endpointCopy(pack.description))}</p>
        </div>
        {formatUI(orgMode && (<div className="ed-state">
            <span className={`status ${on ? 'status--ok' : ''}`}>{formatUI(state)}</span>
            {formatUI(canManage && runs > 0 && (<button type="button" className="btn btn--secondary btn--sm btn--auto" onClick={() => onToggle(pack, !on)}>{formatUI(on ? textUI("m_06f0e210b27d") : textUI("m_29bbb3551e65"))}</button>))}
          </div>))}
      </div>
      <p className="ed-badges">
        <span className="tag">{formatUI(runs)} {formatUI(runs === 1 ? textUI("m_55d808d06666") : textUI("m_9913ded4c50d"))}</span>
        <span className="tag">{richUI("m_fca74a57564e", [formatUI(skipped)])}</span>
        <span className="tag">{textUI("m_1177d7682106")}</span>
      </p>
      <details className="ed-details">
        <summary>{textUI("m_2e90d8215a5a")}</summary>
        <h4 className="ed-h4">{textUI("m_8fd994bd9a88")}</h4>
        <p className="ed-tables">{formatUI(pack.tables_read.length === 0 ? <span className="muted small">{textUI("m_903ea500e7cc")}</span> : pack.tables_read.map((t) => <code key={t} className="tag">{t}</code>))}</p>
        <p className="muted small">{richUI("m_c0b258b7fa7f", [formatUI(pack.file), formatUI(pack.sha256.slice(0, 12))])}</p>
        <div className="ed-scroll" role="region" aria-label={textUI("m_8a1c4f1dc0e9", [pack.name])} tabIndex={0}>
          <table className="org-table">
            <thead><tr><th>{textUI("m_9d60841e0a78")}</th><th>{textUI("m_920e413c7d41")}</th><th>{textUI("m_d3ae14a50bfe")}</th></tr></thead>
            <tbody>{formatUI([...pack.queries].sort((a, b) => PACK_ORDER[a.status] - PACK_ORDER[b.status] || a.name.localeCompare(b.name)).map((q) => <tr key={q.name}><td className="mono">{formatUI(q.name)}</td><td>{formatUI(PACK_STATUS[q.status])}</td><td className="muted small">{formatUI(locale() === 'en-US' ? q.reason : q.status === 'runnable' ? textUI("m_7c0b9d892365", [q.tables.join(', ')]) : displayLabel(({sensitive:'Sensitive tables are excluded.',not_reviewed:'These tables have not been reviewed.',other_os:'This platform has not been verified.',unverified:'This query has not been verified.'} as const)[q.status]))}</td></tr>))}</tbody>
          </table>
        </div>
      </details>
    </article>);
}
function ConsentReview({ doc }: {
    doc: EndpointConsent;
}) {
    useLocale();
    return <section aria-label={textUI("m_ae70d67c25ff")}><h3 className="ed-h4">{richUI("m_e4886204b0d6", [formatUI(doc.version)])}</h3><p className="muted small">{textUI("m_373c4c2636fd")}</p><div className="ed-consent-text" role="region" aria-label={textUI("m_1dbaed740846")} tabIndex={0}>{formatUI(doc.translations?.[language()] ?? doc.text)}</div><p className="muted small">{richUI("m_f4b4a1fab410", [<code>{doc.sha256}</code>])}</p></section>;
}
export function EndpointCatalog({ orgId, canManage = false, platform = 'linux', arch = 'x86_64' }: {
    orgId?: string;
    canManage?: boolean;
    platform?: string;
    arch?: string;
}) {
    useLocale();
    const cat = useEndpointCatalog();
    const org = useOrgOsquery(orgId ?? '', !!orgId);
    const set = useSetOrgOsquery(orgId ?? '');
    const setPack = useSetOrgPack(orgId ?? '');
    const [pending, setPending] = useState<EndpointQueryMeta | null>(null);
    const [pendingPack, setPendingPack] = useState<EndpointPack | null>(null);
    const [reviewVersions, setReviewVersions] = useState<Record<string, number>>({});
    if (cat.isPending)
        return <span className="skeleton"/>;
    if (cat.isError || !cat.data)
        return <div className="state state--error" role="alert"><h2>{textUI("m_9403b65bc150")}</h2><p>{formatUI(describeError(cat.error).message)}</p></div>;
    const runnable = collectionSupported(platform, arch);
    const consent = { version: cat.data.consent.version, sha256: cat.data.consent.sha256 };
    const states = new Map((org.data?.items ?? []).map((i) => [i.key, i]));
    const toggle = (q: EndpointQueryMeta, enabled: boolean) => {
        set.reset();
        const version = states.get(q.key)?.version ?? 0;
        if (enabled) {
            setReviewVersions({ [q.key]: version });
            setPending(q);
        }
        else
            set.mutate({ key: q.key, enabled, version });
    };
    const togglePack = (pk: EndpointPack, enabled: boolean) => {
        setPack.reset();
        const versions = Object.fromEntries(pk.queries.filter(q => q.key && q.status === 'runnable').map(q => [q.key!, states.get(q.key!)?.version ?? 0]));
        if (enabled) {
            setReviewVersions(versions);
            setPendingPack(pk);
        }
        else
            setPack.mutate({ pack: pk.name, enabled: false, versions });
    };
    const reloadChoices = () => { setPending(null); setPendingPack(null); set.reset(); setPack.reset(); void org.refetch(); };
    return (<section className="ed" aria-labelledby="ed-title">
      <h2 id="ed-title" className="panel__heading">{richUI("m_2dc7c296cf7d", [formatUI(platform)])}</h2>{formatUI(!runnable && <p className="notice">{textUI("m_4241370aff9f")}</p>)}
      <p className="muted">{richUI("m_bd51166f4215", [formatUI(cat.data.osquery_version), formatUI(orgId ? textUI("m_7a9161d44b9d") : textUI("m_cdd4ac134f2e"))])}</p>
      {formatUI(set.isError && <div className="alert" role="alert"><strong>{textUI("m_f3ad8557ee44")}</strong><span>{formatUI(describeError(set.error).message)}</span></div>)}
      {formatUI(orgId && org.isError && <p role="alert">{formatUI(describeError(org.error).message)}</p>)}
      <div className="ed-list">
        {formatUI(cat.data.queries.filter((q) => !q.pack && q.category === 'DefenseBot' && q.os.includes(catalogOS(platform))).map((q) => <Card key={q.key} q={q} canManage={canManage && runnable && (!orgId || org.isSuccess)} onToggle={toggle} state={orgId && states.get(q.key) ? { enabled: states.get(q.key)!.enabled, explicit: states.get(q.key)!.explicit } : undefined}/>))}
      </div>
      <h2 id="ed-packs-title" className="panel__heading">{textUI("m_2864afe9cc79")}</h2>
      <p className="muted">{textUI("m_ba2b56c711a0")}</p>
      {formatUI(setPack.isError && <div className="alert" role="alert"><strong>{textUI("m_f3ad8557ee44")}</strong><span>{formatUI(describeError(setPack.error).message)}</span></div>)}
      <div className="ed-list" aria-labelledby="ed-packs-title">
        {formatUI(cat.data.packs.filter(p => !p.platform || p.platform === 'all' || p.platform === platform).map(p => ({ ...p, queries: p.queries.filter(q => !q.platform || q.platform === 'all' || q.platform === platform || (q.platform === 'posix' && (platform === 'linux' || platform === 'darwin'))) })).filter(p => p.queries.length > 0).map((p) => <PackCard key={p.name} pack={p} orgMode={!!orgId} canManage={canManage && runnable && (!orgId || org.isSuccess)} enabledCount={p.queries.filter((q) => q.key && states.get(q.key)?.enabled).length} onToggle={togglePack}/>))}
      </div>
      {formatUI(orgId && <details className="ed-details"><summary>{textUI("m_875ac7287660")}</summary><ul>{formatUI((org.data?.items ?? []).filter(i => i.consented_at).map(i => <li key={i.key}>{richUI("m_3f66b2c4cf4a", [formatUI(i.key), formatUI(i.consent_version), formatUI(i.consented_by), formatUI(new Date(i.consented_at!).toLocaleString(locale())), formatUI(i.enabled ? textUI("m_b389d56c1fce") : textUI("m_05f03f55edcd"))])}</li>))}</ul><p className="muted small">{textUI("m_fa0e3351b85e")}</p></details>)}
      {formatUI(pendingPack && (<Modal labelledBy="ed-approve-pack-title" onClose={() => setPendingPack(null)} width={520}>
          <h2 id="ed-approve-pack-title" className="dialog__title">{richUI("m_c7e22f22f60f", [formatUI(pendingPack.name)])}</h2>
          <p className="subtitle">{richUI("m_610e78c47f70", [formatUI(pendingPack.queries.filter((q) => q.status === 'runnable').length)])}</p>
          <p className="ed-tables">{formatUI(pendingPack.tables_read.map((t) => <code key={t} className="tag">{t}</code>))}</p>
          <ConsentReview doc={cat.data.consent}/>{formatUI(setPack.isError && <div className="alert" role="alert"><p>{formatUI(describeError(setPack.error).message)}</p><button type="button" className="btn btn--secondary btn--auto" onClick={reloadChoices}>{textUI("m_5037c4f222b4")}</button></div>)}<footer className="dialog__foot">
            <button type="button" className="btn btn--secondary btn--auto" onClick={() => setPendingPack(null)}>{textUI("m_19766ed6ccb2")}</button>
            <button type="button" className="btn btn--primary btn--auto" disabled={setPack.isPending} onClick={() => setPack.mutate({ pack: pendingPack.name, enabled: true, versions: reviewVersions, consent }, { onSuccess: () => setPendingPack(null) })}>{formatUI(setPack.isPending ? textUI("m_23e39291d613") : textUI("m_56ee9ae4863d"))}</button>
          </footer>
        </Modal>))}
      {formatUI(pending && (<Modal labelledBy="ed-approve-title" onClose={() => setPending(null)} width={480}>
          <h2 id="ed-approve-title" className="dialog__title">{richUI("m_dacdc66696ea", [formatUI(pending.title)])}</h2>
          <p className="subtitle">{richUI("m_7fac5ddda686", [formatUI(SENSITIVITY_TEXT[pending.sensitivity])])}</p>
          <p className="ed-tables">{formatUI(pending.tables.map((t) => <code key={t} className="tag">{t}</code>))}</p>
          <ConsentReview doc={cat.data.consent}/>{formatUI(set.isError && <div className="alert" role="alert"><p>{formatUI(describeError(set.error).message)}</p><button type="button" className="btn btn--secondary btn--auto" onClick={reloadChoices}>{textUI("m_5037c4f222b4")}</button></div>)}<footer className="dialog__foot">
            <button type="button" className="btn btn--secondary btn--auto" onClick={() => setPending(null)}>{textUI("m_19766ed6ccb2")}</button>
            <button type="button" className="btn btn--primary btn--auto" disabled={set.isPending} onClick={() => set.mutate({ key: pending.key, enabled: true, version: reviewVersions[pending.key]!, consent }, { onSuccess: () => setPending(null) })}>{formatUI(set.isPending ? textUI("m_23e39291d613") : textUI("m_6007acbe30b2"))}</button>
          </footer>
        </Modal>))}
    </section>);
}

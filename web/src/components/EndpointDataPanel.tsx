import { endpointCopy } from '../i18n/endpoint';
import { t as textUI, tr as richUI, useLocale, locale, fv as formatUI } from "../i18n/runtime";
import { useState } from 'react';
import { Modal } from './Modal';
import { Link } from 'react-router-dom';
import { useAssetEndpoint, useRefreshEndpoint, useDeleteEndpointData } from '../api/hooks';
import type { EndpointDataset } from '../api/types';
import { describeError } from './ErrorNote';
const when = (iso: string | null) => (iso ? new Date(iso).toLocaleString(locale(), { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }) : '—');
function statusOf(d: EndpointDataset): {
    text: string;
    cls: string;
} {
    if (!d.enabled && d.status !== 'ok')
        return { text: textUI("m_8d42b28714be"), cls: '' };
    switch (d.status) {
        case 'ok': return { text: textUI("m_1533d367e933"), cls: 'status--ok' };
        case 'skipped': return { text: textUI("m_83c85578f3e4", [d.reason]), cls: '' };
        case 'failed': return { text: textUI("m_03154105c765", [d.reason]), cls: 'status--bad' };
        default: return { text: textUI("m_bda9882d14ca"), cls: '' };
    }
}
function summary(d: EndpointDataset): string {
    const a = d.attributes;
    const parts: string[] = [];
    if (a.os_name)
        parts.push(`${a.os_name} ${a.os_version ?? ''}`.trim());
    if (a.kernel)
        parts.push(textUI("m_2ca2e9840840", [String(a.kernel)]));
    if (a.cpu_logical_cores)
        parts.push(textUI("m_9fe27be74ea6", [String(a.cpu_logical_cores)]));
    if (a.physical_memory)
        parts.push(textUI("m_a70cf66c07a9", [Math.round(Number(a.physical_memory) / 1073741824)]));
    if (a.package_count !== undefined)
        parts.push(textUI("m_713e2681c497", [String(a.package_count)]));
    if (a.local_account_count !== undefined)
        parts.push(textUI("m_0576f2c91bad", [String(a.local_account_count)]));
    if (a.listening_port_count !== undefined)
        parts.push(textUI("m_a9fbc840c677", [String(a.listening_port_count)]));
    if (a.iptables_rules !== undefined)
        parts.push(textUI("m_c16dc6f02748", [String(a.iptables_rules)]));
    return parts.join(' · ');
}
function RowsTable({ d }: {
    d: EndpointDataset;
}) {
    useLocale();
    if (!d.rows || d.rows.length === 0)
        return null;
    const cols = Object.keys(d.rows[0]!).sort();
    return (<details className="ed-details">
      <summary>{richUI("m_e2af31cd6b1e", [formatUI(Math.min(d.rows.length, 10)), formatUI(d.rows.length), formatUI(d.truncated ? '+' : '')])}</summary>
      <div className="ed-scroll" role="region" aria-label={textUI("m_8f710eeb1666", [d.title])} tabIndex={0}>
        <table className="org-table">
          <thead><tr>{formatUI(cols.map((c) => <th key={c}>{formatUI(c)}</th>))}</tr></thead>
          <tbody>{formatUI(d.rows.slice(0, 10).map((r, i) => <tr key={i}>{formatUI(cols.map((c) => <td key={c} className="mono">{formatUI(r[c])}</td>))}</tr>))}</tbody>
        </table>
      </div>
    </details>);
}
export function EndpointDataPanel({ assetID, canRun, allowed, canDelete = false }: {
    assetID: string;
    canRun: boolean;
    allowed: boolean;
    canDelete?: boolean;
}) {
    useLocale();
    const q = useAssetEndpoint(assetID, allowed);
    const refresh = useRefreshEndpoint(assetID);
    const remove = useDeleteEndpointData(assetID);
    const [confirmDelete, setConfirmDelete] = useState(false);
    return (<section className="panel ed-panel" aria-labelledby="ed-asset-h">
      <h2 id="ed-asset-h" className="panel__title">{textUI("m_598037bae080")}</h2>
      <p className="muted small">{textUI("m_cec34285e425")}</p>
      {formatUI(!allowed ? <p className="muted">{textUI("m_d0cecb60890e")}</p> : q.isPending ? <span className="skeleton"/> : q.isError || !q.data ? <p role="alert">{formatUI(describeError(q.error).message)}</p> : (<>
          {formatUI(q.data.supported === false && q.data.has_agent && <p className="notice">{textUI("m_0792ed61b4fd")}</p>)}
          {formatUI(!q.data.has_agent && <p className="muted">{richUI("m_95aa34854904", [<Link to="/assets/agents">{textUI("m_703e4ff0e367")}</Link>])}</p>)}
          {formatUI(q.data.organization_id && <p><Link to={`/organizations/${q.data.organization_id}?tab=agents&asset=${assetID}`}>{textUI("m_c78beab0b1be")}</Link></p>)}
          {formatUI(refresh.isError && <div className="alert" role="alert"><strong>{textUI("m_1aa6f68b7d75")}</strong><span>{formatUI(describeError(refresh.error).message)}</span></div>)}
          {formatUI(canDelete && !q.data.read_only && <button className="btn btn--secondary btn--auto" type="button" onClick={() => setConfirmDelete(true)}>{textUI("m_e39b83b0b4b3")}</button>)}
          {formatUI(remove.isError && <p role="alert">{formatUI(describeError(remove.error).message)}</p>)}
          {formatUI(confirmDelete && <Modal labelledBy="delete-endpoint-data-title" onClose={() => setConfirmDelete(false)} width={480}><h2 id="delete-endpoint-data-title" className="dialog__title">{textUI("m_c3623ff5d9f4")}</h2><p>{textUI("m_6562710a3dbf")}</p><footer className="dialog__foot"><button className="btn btn--secondary btn--auto" type="button" onClick={() => setConfirmDelete(false)}>{textUI("m_19766ed6ccb2")}</button><button className="btn btn--primary btn--auto" type="button" disabled={remove.isPending} onClick={() => remove.mutate(q.data!.version, { onSuccess: () => setConfirmDelete(false) })}>{formatUI(remove.isPending ? textUI("m_43b5894c3e4e") : textUI("m_3566e099aa40"))}</button></footer></Modal>)}
          <ul className="ed-rows">
            {formatUI((q.data.items ?? []).filter((d) => !d.key.startsWith('osquery.pack.') || d.enabled || d.status !== 'never').map((d) => {
                const st = statusOf(d);
                return (<li key={d.key} className="ed-row">
                  <div className="ed-row__main">
                    <strong>{formatUI(endpointCopy(d.title))}</strong>
                    <span className={`status ${st.cls}`}>{formatUI(st.text)}</span>
                    {formatUI(d.synthetic && <span className="tag">{textUI("m_71ddd59aa6f1")}</span>)}
                    <span className="muted small">{formatUI(d.collected_at ? textUI("m_f40e87b2c4ff", [when(d.collected_at), d.osquery_version ? ` · osquery ${d.osquery_version}` : '']) : '')}</span>
                    {formatUI(summary(d) && <p className="muted small">{formatUI(summary(d))}</p>)}
                    <RowsTable d={d}/>
                  </div>
                  {formatUI(canRun && q.data.has_agent && (<button type="button" className="btn btn--secondary btn--sm btn--auto" disabled={refresh.isPending || !d.enabled || d.supported === false} aria-label={textUI("m_61f1b8bae027", [d.title])} onClick={() => refresh.mutate(d.key)}>{textUI("m_0991397702fa")}</button>))}
                </li>);
            }))}
          </ul>
          <h3 className="ed-h4">{textUI("m_74edc5d7939b")}</h3>
          {formatUI((q.data.findings ?? []).length === 0 ? <p className="muted small">{textUI("m_df9622651de4")}</p> : (<ul className="stack-list">{formatUI((q.data.findings ?? []).map((f) => <li key={f.id}><span className={`sev sev--${f.severity}`}>{formatUI(f.severity)}</span> {formatUI(f.title)}</li>))}</ul>))}
        </>))}
    </section>);
}

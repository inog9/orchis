import { useState } from 'react';
import { Modal } from './Modal';
import { Link } from 'react-router-dom';
import { useAssetEndpoint, useRefreshEndpoint, useDeleteEndpointData } from '../api/hooks';
import type { EndpointDataset } from '../api/types';
import { describeError } from './ErrorNote';

const when = (iso: string | null) => (iso ? new Date(iso).toLocaleString('en-US', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }) : '—');

function statusOf(d: EndpointDataset): { text: string; cls: string } {
  if (!d.enabled && d.status !== 'ok') return { text: 'Not approved for this organisation', cls: '' };
  switch (d.status) {
    case 'ok': return { text: 'Collected', cls: 'status--ok' };
    case 'skipped': return { text: `Skipped: ${d.reason}`, cls: '' };
    case 'failed': return { text: `Failed: ${d.reason}`, cls: 'status--bad' };
    default: return { text: 'Not collected yet', cls: '' };
  }
}

function summary(d: EndpointDataset): string {
  const a = d.attributes;
  const parts: string[] = [];
  if (a.os_name) parts.push(`${a.os_name} ${a.os_version ?? ''}`.trim());
  if (a.kernel) parts.push(`kernel ${a.kernel}`);
  if (a.cpu_logical_cores) parts.push(`${a.cpu_logical_cores} CPUs`);
  if (a.physical_memory) parts.push(`${Math.round(Number(a.physical_memory) / 1073741824)} GB RAM`);
  if (a.package_count !== undefined) parts.push(`${a.package_count} packages`);
  if (a.local_account_count !== undefined) parts.push(`${a.local_account_count} accounts`);
  if (a.listening_port_count !== undefined) parts.push(`${a.listening_port_count} listening ports`);
  if (a.iptables_rules !== undefined) parts.push(`${a.iptables_rules} iptables rules`);
  return parts.join(' · ');
}

function RowsTable({ d }: { d: EndpointDataset }) {
  if (!d.rows || d.rows.length === 0) return null;
  const cols = Object.keys(d.rows[0]!).sort();
  return (
    <details className="ed-details">
      <summary>Show {Math.min(d.rows.length, 10)} of {d.rows.length}{d.truncated ? '+' : ''} rows</summary>
      <div className="ed-scroll" role="region" aria-label={`${d.title} rows`} tabIndex={0}>
        <table className="org-table">
          <thead><tr>{cols.map((c) => <th key={c}>{c}</th>)}</tr></thead>
          <tbody>{d.rows.slice(0, 10).map((r, i) => <tr key={i}>{cols.map((c) => <td key={c} className="mono">{r[c]}</td>)}</tr>)}</tbody>
        </table>
      </div>
    </details>
  );
}

export function EndpointDataPanel({ assetID, canRun, allowed, canDelete = false }: { assetID: string; canRun: boolean; allowed: boolean; canDelete?: boolean }) {
  const q = useAssetEndpoint(assetID, allowed);
  const refresh = useRefreshEndpoint(assetID);
  const remove = useDeleteEndpointData(assetID);
  const [confirmDelete, setConfirmDelete] = useState(false);
  return (
    <section className="panel ed-panel" aria-labelledby="ed-asset-h">
      <h2 id="ed-asset-h" className="panel__title">Endpoint data</h2>
      <p className="muted small">What the agent saw from inside this machine (osquery), as opposed to what a scan sees from outside. Approved checks update periodically; an authorised user can also request a refresh.</p>
      {!allowed ? <p className="muted">Your role cannot read asset data.</p> : q.isPending ? <span className="skeleton" /> : q.isError || !q.data ? <p role="alert">{describeError(q.error).message}</p> : (
        <>
          {q.data.supported===false&&q.data.has_agent&&<p className="notice">Endpoint collection is not supported on this agent yet. Only Linux amd64 has a pinned osquery artifact; macOS, Windows and other architectures are not runtime tested.</p>}
          {!q.data.has_agent && <p className="muted">This asset has no agent, so no endpoint data can be collected. <Link to="/assets/agents">Install an agent</Link> to see inventory, accounts and hardening here.</p>}
          {q.data.organization_id&&<p><Link to={`/organizations/${q.data.organization_id}?tab=agents&asset=${assetID}`}>Review collection consent and schedules for this endpoint</Link></p>}
          {refresh.isError && <div className="alert" role="alert"><strong>Couldn’t start the check</strong><span>{describeError(refresh.error).message}</span></div>}
          {canDelete && !q.data.read_only && <button className="btn btn--secondary btn--auto" type="button" onClick={()=>setConfirmDelete(true)}>Delete collected data</button>}
          {remove.isError && <p role="alert">{describeError(remove.error).message}</p>}
          {confirmDelete && <Modal labelledBy="delete-endpoint-data-title" onClose={()=>setConfirmDelete(false)} width={480}><h2 id="delete-endpoint-data-title" className="dialog__title">Delete collected endpoint data?</h2><p>This deletes the latest datasets for this asset and discards in-flight collection results. Findings, frozen reports, audit events and backups remain. Queued, periodic and future manual checks stay enabled; turn them off separately in the organisation.</p><footer className="dialog__foot"><button className="btn btn--secondary btn--auto" type="button" onClick={()=>setConfirmDelete(false)}>Cancel</button><button className="btn btn--primary btn--auto" type="button" disabled={remove.isPending} onClick={()=>remove.mutate(q.data!.version,{onSuccess:()=>setConfirmDelete(false)})}>{remove.isPending?'Deleting…':'Delete data'}</button></footer></Modal>}
          <ul className="ed-rows">
            {(q.data.items ?? []).filter((d) => !d.key.startsWith('osquery.pack.') || d.enabled || d.status !== 'never').map((d) => {
              const st = statusOf(d);
              return (
                <li key={d.key} className="ed-row">
                  <div className="ed-row__main">
                    <strong>{d.title}</strong>
                    <span className={`status ${st.cls}`}>{st.text}</span>
                    {d.synthetic && <span className="tag">Demo data</span>}
                    <span className="muted small">{d.collected_at ? `Collected ${when(d.collected_at)}${d.osquery_version ? ` · osquery ${d.osquery_version}` : ''}` : ''}</span>
                    {summary(d) && <p className="muted small">{summary(d)}</p>}
                    <RowsTable d={d} />
                  </div>
                  {canRun && q.data.has_agent && (
                    <button type="button" className="btn btn--secondary btn--sm btn--auto" disabled={refresh.isPending||!d.enabled||d.supported===false} aria-label={`Run ${d.title} now`} onClick={() => refresh.mutate(d.key)}>Run now</button>
                  )}
                </li>
              );
            })}
          </ul>
          <h3 className="ed-h4">Findings from endpoint data</h3>
          {(q.data.findings ?? []).length === 0 ? <p className="muted small">None yet. Findings appear when a check finds something worth attention.</p> : (
            <ul className="stack-list">{(q.data.findings ?? []).map((f) => <li key={f.id}><span className={`sev sev--${f.severity}`}>{f.severity}</span> {f.title}</li>)}</ul>
          )}
        </>
      )}
    </section>
  );
}

import { useState } from 'react';
import { useEndpointCatalog, useOrgOsquery, useSetOrgOsquery, useSetOrgPack } from '../api/hooks';
import type { EndpointPack, EndpointQueryMeta, EndpointConsent } from '../api/types';
import { describeError } from './ErrorNote';
import { Modal } from './Modal';

const SENSITIVITY_TEXT = { low: 'Low sensitivity', medium: 'Medium sensitivity', high: 'High sensitivity' } as const;
const COL_LIMIT = 5;

function Sev({ s }: { s: string }) {
  return <span className={`sev sev--${s}`}>{s}</span>;
}

function Example({ q }: { q: EndpointQueryMeta }) {
  const cols = q.columns.slice(0, COL_LIMIT);
  return (
    <div className="ed-example">
      <h4 className="ed-h4">Real example from a Linux test machine</h4>
      <div className="ed-scroll" role="region" aria-label={`${q.title} example rows`} tabIndex={0}>
        <table className="org-table">
          <thead><tr>{cols.map((c) => <th key={c}>{c}</th>)}</tr></thead>
          <tbody>{q.example.map((row, i) => <tr key={i}>{cols.map((c) => <td key={c} className="mono">{row[c] ?? ''}</td>)}</tr>)}</tbody>
        </table>
      </div>
      {q.example_findings.length > 0 ? (
        <>
          <h4 className="ed-h4">Findings this example would produce</h4>
          <ul className="stack-list">{q.example_findings.map((f) => <li key={f.rule + f.title}><Sev s={f.severity} /> {f.title}</li>)}</ul>
        </>
      ) : <p className="muted small">This example does not trigger any finding.</p>}
    </div>
  );
}

function Card({ q, state, canManage, onToggle }: { q: EndpointQueryMeta; state?: { enabled: boolean; explicit: boolean }; canManage: boolean; onToggle?: (q: EndpointQueryMeta, enabled: boolean) => void }) {
  const orgMode = state !== undefined;
  const status = !orgMode ? null : state.enabled ? (state.explicit ? 'Approved by an administrator' : 'On by default') : (state.explicit ? 'Turned off by an administrator' : 'Needs approval');
  return (
    <article className="org-card ed-card" aria-label={q.title}>
      <div className="ed-card__head">
        <div>
          <h3 className="ed-title">{q.title}</h3>
          <p className="muted">{q.description}</p>
        </div>
        {orgMode && (
          <div className="ed-state">
            <span className={`status ${state.enabled ? 'status--ok' : ''}`}>{status}</span>
            {canManage && onToggle && (
              <button type="button" className="btn btn--secondary btn--sm btn--auto" onClick={() => onToggle(q, !state.enabled)}>{state.enabled ? 'Turn off' : 'Review and approve'}</button>
            )}
          </div>
        )}
      </div>
      <p className="ed-badges">
        <span className="tag">{SENSITIVITY_TEXT[q.sensitivity]}</span>
        <span className="tag">{q.load} load</span>
        <span className="tag">{q.os.includes('linux') ? 'Linux: tested' : q.os.join(', ')}</span>
        <span className="tag">macOS and Windows: not proven yet</span>
        {q.needs_root && <span className="tag">Needs administrator rights</span>}
      </p>
      <details className="ed-details">
        <summary>What it reads and where the data goes</summary>
        <div className="ed-grid">
          <div>
            <h4 className="ed-h4">Reads these osquery tables</h4>
            <p className="ed-tables">{q.tables.map((t) => <code key={t} className="tag">{t}</code>)}</p>
            <p className="muted small">Never read: browser history, SSH keys, process command lines, file contents, password hashes.</p>
            <h4 className="ed-h4">Asset details it fills in</h4>
            <p className="ed-tables">{q.attributes.map((a) => <code key={a} className="tag">{a}</code>)}</p>
          </div>
          <div>
            <h4 className="ed-h4">Findings it can raise</h4>
            {q.findings.length === 0 ? <p className="muted small">None. It only records data.</p> : (
              <ul className="stack-list">{q.findings.map((f) => <li key={f.rule}><Sev s={f.severity} /> {f.title}<br /><span className="muted small">{f.explain}</span></li>)}</ul>
            )}
            <h4 className="ed-h4">MITRE ATT&amp;CK</h4>
            <ul className="stack-list">{q.mitre.map((m) => <li key={m.id}><strong>{m.id}</strong> <span className="muted small">{m.reason}</span></li>)}</ul>
            <p className="muted small">Shown in reports under “{q.report_section}”.</p>
          </div>
        </div>
        <Example q={q} />
      </details>
    </article>
  );
}

const PACK_STATUS = { runnable: 'Runs', sensitive: 'Skipped: sensitive data', not_reviewed: 'Skipped: table not reviewed', other_os: 'Not for Linux', unverified: 'Skipped: not verified' } as const;

const PACK_ORDER = { runnable: 0, sensitive: 1, not_reviewed: 2, unverified: 3, other_os: 4 } as const;

function PackCard({ pack, enabledCount, canManage, orgMode, onToggle }: { pack: EndpointPack; enabledCount: number; canManage: boolean; orgMode: boolean; onToggle: (p: EndpointPack, enabled: boolean) => void }) {
  const runs = pack.queries.filter((q) => q.status === 'runnable').length;
  const skipped = pack.queries.length - runs;
  const on = runs > 0 && enabledCount === runs;
  const state = runs === 0 ? 'Nothing here can run' : on ? 'Approved by an administrator' : enabledCount > 0 ? 'Partly approved' : 'Needs approval';
  return (
    <article className="org-card ed-card" aria-label={`${pack.name} pack`}>
      <div className="ed-card__head">
        <div>
          <h3 className="ed-title">{pack.name} pack</h3>
          <p className="muted">{pack.description}</p>
        </div>
        {orgMode && (
          <div className="ed-state">
            <span className={`status ${on ? 'status--ok' : ''}`}>{state}</span>
            {canManage && runs > 0 && (
              <button type="button" className="btn btn--secondary btn--sm btn--auto" onClick={() => onToggle(pack, !on)}>{on ? 'Turn off' : 'Review and approve'}</button>
            )}
          </div>
        )}
      </div>
      <p className="ed-badges">
        <span className="tag">{runs} {runs === 1 ? 'check runs' : 'checks run'}</span>
        <span className="tag">{skipped} skipped</span>
        <span className="tag">Frozen from the official osquery pack</span>
      </p>
      <details className="ed-details">
        <summary>What this pack reads and what it skips</summary>
        <h4 className="ed-h4">Reads these osquery tables</h4>
        <p className="ed-tables">{pack.tables_read.length === 0 ? <span className="muted small">None.</span> : pack.tables_read.map((t) => <code key={t} className="tag">{t}</code>)}</p>
        <p className="muted small">Source file {pack.file} (SHA-256 {pack.sha256.slice(0, 12)}…). It is not downloaded while running. Checks that touch sensitive tables are always skipped, whatever the pack says.</p>
        <div className="ed-scroll" role="region" aria-label={`${pack.name} checks`} tabIndex={0}>
          <table className="org-table">
            <thead><tr><th>Check</th><th>Status</th><th>Why</th></tr></thead>
            <tbody>{[...pack.queries].sort((a, b) => PACK_ORDER[a.status] - PACK_ORDER[b.status] || a.name.localeCompare(b.name)).map((q) => <tr key={q.name}><td className="mono">{q.name}</td><td>{PACK_STATUS[q.status]}</td><td className="muted small">{q.reason || (q.tables.length ? `Reads ${q.tables.join(', ')}.` : '')}</td></tr>)}</tbody>
          </table>
        </div>
      </details>
    </article>
  );
}

function ConsentReview({ doc }: { doc: EndpointConsent }) {
 return <section aria-label="Endpoint collection consent"><h3 className="ed-h4">Collection consent · {doc.version}</h3><p className="muted small">Scroll within the document to review data, retention, and how to stop or delete collection.</p><div className="ed-consent-text" role="region" aria-label="Collection document" tabIndex={0}>{doc.text}</div><p className="muted small">Approving records your identity, the time, and document SHA-256 <code>{doc.sha256}</code>.</p></section>;
}

export function EndpointCatalog({ orgId, canManage = false }: { orgId?: string; canManage?: boolean }) {
  const cat = useEndpointCatalog();
  const org = useOrgOsquery(orgId ?? '', !!orgId);
  const set = useSetOrgOsquery(orgId ?? '');
  const setPack = useSetOrgPack(orgId ?? '');
  const [pending, setPending] = useState<EndpointQueryMeta | null>(null);
  const [pendingPack, setPendingPack] = useState<EndpointPack | null>(null);
  if (cat.isPending) return <span className="skeleton" />;
  if (cat.isError || !cat.data) return <div className="state state--error" role="alert"><h2>Couldn’t load the endpoint data catalogue</h2><p>{describeError(cat.error).message}</p></div>;
  const consent = { version: cat.data.consent.version, sha256: cat.data.consent.sha256 };
  const states = new Map((org.data?.items ?? []).map((i) => [i.key, i]));
  const toggle = (q: EndpointQueryMeta, enabled: boolean) => {
    if (enabled) setPending(q);
    else set.mutate({ key: q.key, enabled });
  };
  return (
    <section className="ed" aria-labelledby="ed-title">
      <h2 id="ed-title" className="panel__heading">Endpoint data</h2>
      <p className="muted">What an installed agent can read from inside a machine, using osquery {cat.data.osquery_version}. Each check is read-only, runs once, and only reads the tables listed here. {orgId ? 'Low-sensitivity checks are on by default. Others need an administrator’s approval.' : 'Approval is decided per organisation.'}</p>
      {set.isError && <div className="alert" role="alert"><strong>Couldn’t save this choice</strong><span>{describeError(set.error).message}</span></div>}
      {orgId && org.isError && <p role="alert">{describeError(org.error).message}</p>}
      <div className="ed-list">
        {cat.data.queries.filter((q) => !q.pack).map((q) => <Card key={q.key} q={q} canManage={canManage} onToggle={toggle} state={orgId && states.get(q.key) ? { enabled: states.get(q.key)!.enabled, explicit: states.get(q.key)!.explicit } : undefined} />)}
      </div>
      <h2 id="ed-packs-title" className="panel__heading">Official packs</h2>
      <p className="muted">Collections of checks published with osquery, frozen in this release. Approving a pack approves only the checks marked “Runs”. Everything else stays skipped.</p>
      {setPack.isError && <div className="alert" role="alert"><strong>Couldn’t save this choice</strong><span>{describeError(setPack.error).message}</span></div>}
      <div className="ed-list" aria-labelledby="ed-packs-title">
        {cat.data.packs.map((p) => <PackCard key={p.name} pack={p} orgMode={!!orgId} canManage={canManage} enabledCount={p.queries.filter((q) => q.key && states.get(q.key)?.enabled).length} onToggle={(pk, enabled) => (enabled ? setPendingPack(pk) : setPack.mutate({ pack: pk.name, enabled: false }))} />)}
      </div>
      {orgId && <details className="ed-details"><summary>Recorded collection approvals</summary><ul>{(org.data?.items ?? []).filter(i=>i.consented_at).map(i=><li key={i.key}>{i.key}: document {i.consent_version}, approved by {i.consented_by} at {new Date(i.consented_at!).toLocaleString()}. {i.enabled?'Enabled.':'Currently disabled.'}</li>)}</ul><p className="muted small">Revocations are recorded in the audit log. Approvals for an older document need renewal.</p></details>}
      {pendingPack && (
        <Modal labelledBy="ed-approve-pack-title" onClose={() => setPendingPack(null)} width={520}>
          <h2 id="ed-approve-pack-title" className="dialog__title">Approve the {pendingPack.name} pack?</h2>
          <p className="subtitle">Agents in this organisation will run {pendingPack.queries.filter((q) => q.status === 'runnable').length} checks and read these osquery tables. The approval is recorded with the pack version.</p>
          <p className="ed-tables">{pendingPack.tables_read.map((t) => <code key={t} className="tag">{t}</code>)}</p>
          <ConsentReview doc={cat.data.consent} /><footer className="dialog__foot">
            <button type="button" className="btn btn--secondary btn--auto" onClick={() => setPendingPack(null)}>Cancel</button>
            <button type="button" className="btn btn--primary btn--auto" disabled={setPack.isPending} onClick={() => setPack.mutate({ pack: pendingPack.name, enabled: true, consent }, { onSuccess: () => setPendingPack(null) })}>{setPack.isPending ? 'Saving…' : 'Approve pack'}</button>
          </footer>
        </Modal>
      )}
      {pending && (
        <Modal labelledBy="ed-approve-title" onClose={() => setPending(null)} width={480}>
          <h2 id="ed-approve-title" className="dialog__title">Approve “{pending.title}”?</h2>
          <p className="subtitle">Agents in this organisation will read these osquery tables from their machines. {SENSITIVITY_TEXT[pending.sensitivity]}.</p>
          <p className="ed-tables">{pending.tables.map((t) => <code key={t} className="tag">{t}</code>)}</p>
          <ConsentReview doc={cat.data.consent} /><footer className="dialog__foot">
            <button type="button" className="btn btn--secondary btn--auto" onClick={() => setPending(null)}>Cancel</button>
            <button type="button" className="btn btn--primary btn--auto" disabled={set.isPending} onClick={() => set.mutate({ key: pending.key, enabled: true, consent }, { onSuccess: () => setPending(null) })}>{set.isPending ? 'Saving…' : 'Approve'}</button>
          </footer>
        </Modal>
      )}
    </section>
  );
}

import { TestPrerequisites } from '../components/TestPrerequisites';
import { ChevronRight, Play, Square, Trash2 } from 'lucide-react';
import { useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { useAtomicTest, useCancelExecution, useDeleteAtomicTest, useExecution, useMe, useRunAtomicTest } from '../api/hooks';
import { can, type NmapParams, type NucleiParams } from '../api/types';
import { describeError } from '../components/ErrorNote';
import { RunBadge, SeverityBadge, when } from '../components/RunBadges';
import { ConfirmDialog } from './ConfirmDialog';

function describeParams(key: string, p: NmapParams | NucleiParams | Record<string, unknown>): string {
  if (key.startsWith('agent.')) return 'runs on the endpoint, no settings';
  if (key !== 'nmap.tcp_connect' && !(p as NucleiParams).severity) return 'default settings';
  if (key === 'nmap.tcp_connect') {
    const n = p as NmapParams;
    const ports = n.profile === 'custom' ? `ports ${n.ports}` : n.profile === 'web' ? 'web ports' : `top ${n.profile.replace('top', '')} ports`;
    return `${ports}${n.service_detection ? ', service detection' : ''}`;
  }
  const n = p as NucleiParams;
  return `severity ${n.severity.join(', ')}${n.tags?.length ? `, tags ${n.tags.join(', ')}` : ''}`;
}

export function AtomicTestDetailPage() {
  const { id = '' } = useParams();
  const { data: me } = useMe();
  const navigate = useNavigate();
  const q = useAtomicTest(id);
  const run = useRunAtomicTest(id);
  const cancel = useCancelExecution();
  const del = useDeleteAtomicTest(id);
  const [selected, setSelected] = useState<string | undefined>();
  const [confirmDelete, setConfirmDelete] = useState(false);
  const latest = q.data?.executions?.[0]?.id;
  // Setelah dijalankan, tampilkan eksekusi terbaru; pengguna masih bisa memilih yang lama dari riwayat.
  useEffect(() => { if (latest) setSelected(latest); }, [latest]);
  const exec = useExecution(selected);

  if (q.isPending) return <section className="page"><span className="skeleton" /></section>;
  if (q.isError || !q.data) {
    return (
      <section className="page"><div className="state state--error" role="alert">
        <h2>Couldn’t load this test</h2><p>{describeError(q.error).message}</p>
        <Link className="btn btn--secondary btn--auto" to="/atomic">Back to single tests</Link>
      </div></section>
    );
  }
  const t = q.data;
  const active = t.last_execution && t.last_execution.state !== 'ended';
  const e = exec.data;

  return (
    <section className="page" aria-labelledby="atomic-detail-title">
      <div className="page__head">
        <div>
          <nav className="crumbs" aria-label="Breadcrumb"><Link to="/atomic">Single tests</Link><ChevronRight size={12} aria-hidden="true" /><span className="crumbs__current">{t.name}</span></nav>
          <div className="page__title-row">
            <h1 id="atomic-detail-title" className="page__title">{t.name}</h1>
            <RunBadge run={t.last_execution} />
          </div>
          <p className="page__sub">{t.contract_name} on <strong>{t.target_asset_name ?? 'no target'}</strong> · {describeParams(t.contract_key, t.params)}</p>
        </div>
        <div className="page__actions">
          {can(me, 'atomic.run') && !active && (
            <button type="button" className="btn btn--primary btn--auto" onClick={() => run.mutate()} disabled={run.isPending}><Play size={15} aria-hidden="true" /> {run.isPending ? 'Starting…' : 'Run'}</button>
          )}
          {can(me, 'atomic.run') && active && t.last_execution && (
            <button type="button" className="btn btn--secondary btn--auto" onClick={() => cancel.mutate(t.last_execution!.id)} disabled={cancel.isPending}><Square size={14} aria-hidden="true" /> Stop</button>
          )}
          {can(me, 'atomic.delete') && (
            <button type="button" className="btn btn--danger btn--auto" onClick={() => setConfirmDelete(true)}><Trash2 size={15} aria-hidden="true" /> Delete</button>
          )}
        </div>
      </div>
      {e?.is_demo&&<p className="notice" role="status"><strong>Demo / synthetic result.</strong> No scanner process was run. Use a new authorized local run to collect actual evidence.</p>}
      {run.isError && <div className="alert" role="alert"><strong>Couldn’t start the test</strong><span>{describeError(run.error).message}</span></div>}
      {cancel.isError && <div className="alert" role="alert"><strong>Couldn’t stop the test</strong><span>{describeError(cancel.error).message}</span></div>}

      <TestPrerequisites targetID={t.target_asset_id} needsAgent={t.contract_key.startsWith('agent.')} />
      <div className="split">
        <div className="panel">
          <h2 className="panel__title">Runs</h2>
          {(t.executions ?? []).length === 0 ? <p className="muted pad">This test has not run yet.</p> : (
            <ul className="runs">
              {t.executions!.map((r) => (
                <li key={r.id}>
                  <button type="button" className={`runs__item ${selected === r.id ? 'is-on' : ''}`} onClick={() => setSelected(r.id)} aria-pressed={selected === r.id}>
                    <span>{when(r.created_at)}</span>
                    <RunBadge run={r} />
                    <span className="muted small">{r.finding_count} {r.finding_count === 1 ? 'finding' : 'findings'}</span>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>

        <div className="panel" aria-live="polite">
          {!e ? <p className="muted pad">{selected ? 'Loading…' : 'Run the test to see results here.'}</p> : (
            <>
              <h2 className="panel__title">Result <RunBadge run={e} /></h2>
              <dl className="kv">
                <div><dt>Target</dt><dd className="mono">{e.target_address}</dd></div>
                <div><dt>Started</dt><dd>{when(e.started_at ?? e.created_at)}</dd></div>
                <div><dt>Finished</dt><dd>{when(e.ended_at)}</dd></div>
              </dl>
              {e.error && <div className="alert" role="alert"><strong>The run did not complete</strong><span>{e.error}</span></div>}
              <h3 className="panel__sub">Findings ({e.findings.length})</h3>
              {e.state !== 'ended' ? <p className="muted pad">Waiting for results…</p> : e.findings.length === 0 ? (
                <p className="muted pad">{e.outcome === 'success' ? 'The scan finished and found nothing.' : 'No findings were recorded.'}</p>
              ) : (
                <ul className="finds">
                  {e.findings.map((f) => (
                    <li key={f.id}><SeverityBadge severity={f.severity} /><span className="finds__title">{f.title}</span><code className="mono small">{f.value}</code></li>
                  ))}
                </ul>
              )}
              <h3 className="panel__sub">Log</h3>
              <ol className="log">
                {e.logs.map((l, i) => <li key={i} className={`log--${l.level}`}><time>{when(l.at)}</time> {l.message}</li>)}
              </ol>
            </>
          )}
        </div>
      </div>
      {confirmDelete && (
        <ConfirmDialog title="Delete this single test?" body="The test and its run history are removed. Findings it discovered stay in Findings." confirmLabel="Delete test"
          busy={del.isPending} error={del.isError ? describeError(del.error).message : undefined}
          onConfirm={() => del.mutate(undefined, { onSuccess: () => navigate('/atomic') })} onClose={() => setConfirmDelete(false)} />
      )}
    </section>
  );
}

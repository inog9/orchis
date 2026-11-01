import { useMutation } from '@tanstack/react-query';
import { api } from '../api/client';
import { ChevronRight, Pause, Play, Square } from 'lucide-react';
import { useState } from 'react';
import { Link, useParams, useSearchParams } from 'react-router-dom';
import { useAttackPath, useMe, useSimulation, useSimulationAction, useSimulationFindings } from '../api/hooks';
import { can, type AttackPath, type PathNode } from '../api/types';
import { describeError } from '../components/ErrorNote';
import { SeverityBadge, when } from '../components/RunBadges';
import { SimBadge, StepBadge, elapsed } from '../components/StatusPills';
import { ConfirmDialog } from './ConfirmDialog';

const COL_W = 250, NODE_W = 210, NODE_H = 64, ROW_H = 80;
const COL_TITLES = ['Actions', 'Targets', 'Findings', 'Follow-up actions'];

export function AttackPathView({ path }: { path: AttackPath }) {
  if (path.nodes.length === 0) return <p className="muted pad">Nothing to draw yet.</p>;
  const rows = new Map<number, number>();
  const placed = new Map<string, { n: PathNode; x: number; y: number }>();
  for (const n of path.nodes) {
    const r = rows.get(n.column) ?? 0;
    rows.set(n.column, r + 1);
    placed.set(n.id, { n, x: n.column * COL_W, y: 28 + r * ROW_H });
  }
  const cols = Math.max(...path.nodes.map((n) => n.column)) + 1;
  const width = cols * COL_W, height = 28 + Math.max(...rows.values()) * ROW_H;
  return (
    <div className="pathwrap" role="group" tabIndex={0} aria-label={`Attack path with ${path.nodes.length} nodes and ${path.edges.length} connections`}>
      <div className="pathgraph" style={{ width, height }}>
        {COL_TITLES.slice(0, cols).map((t, i) => <span key={t} className="pathcol" style={{ left: i * COL_W }}>{t}</span>)}
        <svg width={width} height={height} className="canvas__edges" aria-hidden="true">
          {path.edges.map((e, i) => {
            const a = placed.get(e.from), b = placed.get(e.to);
            if (!a || !b) return null;
            const x1 = a.x + NODE_W, y1 = a.y + NODE_H / 2, x2 = b.x, y2 = b.y + NODE_H / 2, mx = (x1 + x2) / 2;
            const back = x2 <= x1; // tepi ke kolom yang sama/lebih kiri digambar melengkung di atas
            const d = back ? `M${x1},${y1} C${x1 + 40},${y1 - 50} ${x2 - 40},${y2 - 50} ${x2},${y2}` : `M${x1},${y1} C${mx},${y1} ${mx},${y2} ${x2},${y2}`;
            return <path key={i} d={d} className={`canvas__edge pathedge--${e.kind}`} />;
          })}
        </svg>
        {[...placed.values()].map(({ n, x, y }) => (
          <div key={n.id} className={`pathnode pathnode--${n.kind} ${n.severity ? `pathnode--${n.severity}` : ''}`} style={{ left: x, top: y, width: NODE_W, height: NODE_H }} title={n.label}>
            <strong>{n.label}</strong>
            <span className="muted small">{n.status ? `${n.sub ?? ''} · ${n.status}` : n.severity ? `${n.sub?.replace('_', ' ')} · ${n.severity}` : n.sub ?? ''}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

function FindingsTab({ id }: { id: string }) {
  const res = useSimulationFindings(id);
  const items = res.data?.pages.flatMap((p) => p.items) ?? [];
  if (res.isPending) return <span className="skeleton" />;
  if (res.isError) return <div className="state state--error" role="alert"><h2>Couldn’t load findings</h2><p>{describeError(res.error).message}</p></div>;
  if (items.length === 0) return <p className="muted pad">No findings yet in this simulation.</p>;
  return (
    <>
      <ul className="list finding-list" aria-label="Simulation findings" tabIndex={0}>
        {items.map((f) => (
          <li key={f.id} className="finding-row">
            <SeverityBadge severity={f.severity} />
            <div className="finding-row__main"><strong>{f.title}</strong><span className="muted small">{f.asset_id ? <Link to={`/assets/${f.asset_id}`}>{f.asset_name}</Link> : 'No asset'} · <code className="mono">{f.value}</code></span></div>
            <span className="muted small">{when(f.last_seen_at)}</span>
          </li>
        ))}
      </ul>
      {res.hasNextPage && <div className="more"><button type="button" className="btn btn--secondary btn--auto" onClick={() => res.fetchNextPage()}>Load more</button></div>}
    </>
  );
}

export function SimulationDetailPage() {
  const { id = '' } = useParams();
  const { data: me } = useMe();
  const [params, setParams] = useSearchParams();
  const tab = params.get('tab') ?? 'overview';
  const q = useSimulation(id);
  const act = useSimulationAction(id);
  const path = useAttackPath(id, q.data?.status);
  const dryRun=useMutation({mutationFn:()=>api.post<{executed:boolean;items:{asset_id:string;step:string;status:string;reason?:string;recorded_reason?:string;mapping_error?:string;inputs?:unknown}[]}>(`/api/v1/simulations/${id}/dry-run`)});
  const [cancelling, setCancelling] = useState(false);

  if (q.isPending) return <section className="page"><span className="skeleton" /></section>;
  if (q.isError || !q.data) {
    return <section className="page"><div className="state state--error" role="alert"><h2>Couldn’t load this simulation</h2><p>{describeError(q.error).message}</p><Link className="btn btn--secondary btn--auto" to="/simulations">Back to simulations</Link></div></section>;
  }
  const s = q.data;
  const live = s.status === 'running' || s.status === 'paused';
  const pct = s.step_count === 0 ? 0 : Math.round((s.steps_done / s.step_count) * 100);
  const canRun = can(me, 'simulation.run') && !s.is_demo;
  const TabLink = ({ k, label }: { k: string; label: string }) => (
    <button type="button" role="tab" aria-selected={tab === k} className={`tab ${tab === k ? 'is-active' : ''}`} onClick={() => setParams(k === 'overview' ? {} : { tab: k })}>{label}</button>
  );

  return (
    <section className="page" aria-labelledby="sim-title">
      <div className="page__head">
        <div>
          <nav className="crumbs" aria-label="Breadcrumb"><Link to="/simulations">Simulations</Link><ChevronRight size={12} aria-hidden="true" /><span className="crumbs__current">{s.code}</span></nav>
          <div className="page__title-row"><h1 id="sim-title" className="page__title">{s.name}</h1><SimBadge status={s.status} /></div>
          <p className="page__sub">{s.code} · {s.scenario_id ? <Link to={`/scenarios/${s.scenario_id}`}>{s.scenario_name}</Link> : 'scenario removed'} · {s.target_count} {s.target_count === 1 ? 'target' : 'targets'}</p>
        </div>
        {canRun && live && (
          <div className="page__actions">
            {s.status === 'running' ? <button type="button" className="btn btn--secondary btn--auto" disabled={act.isPending} onClick={() => act.mutate({ action: 'pause' })}><Pause size={15} aria-hidden="true" /> Pause</button>
              : <button type="button" className="btn btn--primary btn--auto" disabled={act.isPending} onClick={() => act.mutate({ action: 'resume' })}><Play size={15} aria-hidden="true" /> Resume</button>}
            <button type="button" className="btn btn--danger btn--auto" onClick={() => setCancelling(true)}><Square size={14} aria-hidden="true" /> Stop</button>
          </div>
        )}
      </div>
      {s.is_demo && <div className="notice" role="status"><strong>Demo / synthetic history.</strong> These states illustrate the interface. No checks were executed; this record cannot be resumed or dispatched.</div>}
      {act.isError && <div className="alert" role="alert"><strong>That didn’t work</strong><span>{describeError(act.error).message}</span></div>}
      {s.cancel_reason && <div className="notice" role="status"><strong>{s.status === 'failed' ? 'Stopped automatically.' : 'Stopped.'}</strong> {s.cancel_reason}</div>}

      <div className="tabs" role="tablist" aria-label="Simulation sections"><TabLink k="overview" label="Overview" /><TabLink k="findings" label={`Findings (${s.finding_count})`} /><TabLink k="path" label="Attack path" /></div>

      {tab === 'overview' && (
        <>
          <div className="kpis kpis--3">
            <div className="kpi"><div className="kpi__link"><span className="kpi__label">Progress</span><strong className="kpi__value">{pct}%</strong><span className="kpi__note">{s.steps_done} of {s.step_count} steps finished</span></div></div>
            <div className="kpi"><div className="kpi__link"><span className="kpi__label">{live ? 'Running for' : 'Took'}</span><strong className="kpi__value">{elapsed(s.started_at, s.ended_at)}</strong><span className="kpi__note">Started {when(s.started_at)}</span></div></div>
            <div className="kpi"><div className="kpi__link"><span className="kpi__label">Findings</span><strong className="kpi__value">{s.finding_count}</strong><span className="kpi__note">&nbsp;</span></div></div>
          </div>
          <div className="progress" role="progressbar" aria-valuenow={pct} aria-valuemin={0} aria-valuemax={100} aria-label="Simulation progress"><span style={{ width: `${pct}%` }} /></div>
          <div className="list" role="table" aria-label="Steps" tabIndex={0}>
            <div className="list__head step-row" role="row"><span role="columnheader">STEP</span><span role="columnheader">STATUS</span><span role="columnheader">STARTS</span><span role="columnheader">RUNS</span><span role="columnheader">FINDINGS</span></div>
            {s.steps.map((st) => (
              <div key={st.key} className="asset-row step-row" role="row">
                <span role="cell" className="asset-row__text"><strong>{st.title}</strong><span className="muted small">{st.contract_name}</span></span>
                <span role="cell"><StepBadge status={st.status} /></span>
                <span role="cell" className="small">{st.trigger_mode === 'on_finding' ? `On ${st.trigger?.type?.replace('_', ' ')}${st.trigger?.values?.length ? ` ${st.trigger.values.join(', ')}` : ''}` : 'After parents'}{st.delay_seconds ? ` · +${st.delay_seconds}s` : ''}</span>
                <span role="cell" className="small">{st.executions.total ? `${st.executions.success ?? 0}/${st.executions.total} ok` : '—'}</span>
                <span role="cell">{st.finding_count}</span>
              </div>
            ))}
          </div>
        </>
      )}
      {tab === 'findings' && <FindingsTab id={id} />}
      {tab === 'path' && (
        <div className="panel">
          <h2 className="panel__heading">Attack path</h2>
          <p className="muted small">Actions run against targets, targets yield findings, and findings can start follow-up actions.</p>
          {path.isPending ? <span className="skeleton" /> : path.isError ? <p role="alert">{describeError(path.error).message}</p> : <AttackPathView path={path.data} />}
          {path.data?.truncated && <p className="muted small">Showing the most severe findings only.</p>}
        </div>
      )}

      {can(me,'finding.read')&&can(me,'asset.read')&&<section className="panel"><h2 className="panel__heading">Decisions and mapping preview</h2><p className="muted small">Evaluate current parent results without starting any checks.</p><button className="btn btn--secondary btn--auto" disabled={dryRun.isPending} onClick={()=>dryRun.mutate()}>Dry-run conditions and mapping</button>{dryRun.error&&<p role="alert">{describeError(dryRun.error).message}</p>}{dryRun.data?.items.map((row,i)=><article className="org-card" key={i}><h3>{row.step} · {row.status}</h3><p>{row.recorded_reason||row.reason||'Legacy dependency rules'}</p><p className="muted small">Asset {row.asset_id}</p>{row.mapping_error?<p>{row.mapping_error}</p>:<pre className="mono">{JSON.stringify(row.inputs,null,2)}</pre>}</article>)}</section>}
      {cancelling && (
        <ConfirmDialog title="Stop this simulation?" body="Queued and running actions are stopped. Findings already recorded are kept. This can’t be undone." confirmLabel="Stop simulation"
          busy={act.isPending} error={act.isError ? describeError(act.error).message : undefined}
          onConfirm={() => act.mutate({ action: 'cancel', reason: 'Stopped by user' }, { onSuccess: () => setCancelling(false) })} onClose={() => setCancelling(false)} />
      )}
    </section>
  );
}

import { t as textUI, tr as richUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { advancedEnabled } from '../components/navigation';
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
const COL_TITLES = () => [textUI("m_ff8059dc6752"), textUI("m_27445f6ab61d"), textUI("m_e171c2ff25b5"), textUI("m_c7883a6fd408")];
export function AttackPathView({ path }: {
    path: AttackPath;
}) {
    useLocale();
    if (path.nodes.length === 0)
        return <p className="muted pad">{textUI("m_ecdd1cda6894")}</p>;
    const rows = new Map<number, number>();
    const placed = new Map<string, {
        n: PathNode;
        x: number;
        y: number;
    }>();
    for (const n of path.nodes) {
        const r = rows.get(n.column) ?? 0;
        rows.set(n.column, r + 1);
        placed.set(n.id, { n, x: n.column * COL_W, y: 28 + r * ROW_H });
    }
    const cols = Math.max(...path.nodes.map((n) => n.column)) + 1;
    const width = cols * COL_W, height = 28 + Math.max(...rows.values()) * ROW_H;
    return (<div className="pathwrap" role="group" tabIndex={0} aria-label={textUI("m_d6de7a2d2dfc", [path.nodes.length, path.edges.length])}>
      <div className="pathgraph" style={{ width, height }}>
        {formatUI(COL_TITLES().slice(0, cols).map((t, i) => <span key={t} className="pathcol" style={{ left: i * COL_W }}>{formatUI(t)}</span>))}
        <svg width={width} height={height} className="canvas__edges" aria-hidden="true">
          {formatUI(path.edges.map((e, i) => {
            const a = placed.get(e.from), b = placed.get(e.to);
            if (!a || !b)
                return null;
            const x1 = a.x + NODE_W, y1 = a.y + NODE_H / 2, x2 = b.x, y2 = b.y + NODE_H / 2, mx = (x1 + x2) / 2;
            const back = x2 <= x1; // tepi ke kolom yang sama/lebih kiri digambar melengkung di atas
            const d = back ? `M${x1},${y1} C${x1 + 40},${y1 - 50} ${x2 - 40},${y2 - 50} ${x2},${y2}` : `M${x1},${y1} C${mx},${y1} ${mx},${y2} ${x2},${y2}`;
            return <path key={i} d={d} className={`canvas__edge pathedge--${e.kind}`}/>;
        }))}
        </svg>
        {formatUI([...placed.values()].map(({ n, x, y }) => (<div key={n.id} className={`pathnode pathnode--${n.kind} ${n.severity ? `pathnode--${n.severity}` : ''}`} style={{ left: x, top: y, width: NODE_W, height: NODE_H }} title={n.label}>
            <strong>{formatUI(n.label)}</strong>
            <span className="muted small">{formatUI(n.status ? `${n.sub ?? ''} · ${n.status}` : n.severity ? `${n.sub?.replace('_', ' ')} · ${n.severity}` : n.sub ?? '')}</span>
          </div>)))}
      </div>
    </div>);
}
function FindingsTab({ id }: {
    id: string;
}) {
    useLocale();
    const res = useSimulationFindings(id);
    const items = res.data?.pages.flatMap((p) => p.items) ?? [];
    if (res.isPending)
        return <span className="skeleton"/>;
    if (res.isError)
        return <div className="state state--error" role="alert"><h2>{textUI("m_8891e771bd5c")}</h2><p>{formatUI(describeError(res.error).message)}</p></div>;
    if (items.length === 0)
        return <p className="muted pad">{textUI("m_a52cbc53c05f")}</p>;
    return (<>
      <ul className="list finding-list" aria-label={textUI("m_1cc0aafc4a1d")} tabIndex={0}>
        {formatUI(items.map((f) => (<li key={f.id} className="finding-row">
            <SeverityBadge severity={f.severity}/>
            <div className="finding-row__main"><strong>{formatUI(f.title)}</strong><span className="muted small">{formatUI(f.asset_id ? <Link to={`/assets/${f.asset_id}`}>{formatUI(f.asset_name)}</Link> : textUI("m_bedb7e61a067"))} · <code className="mono">{f.value}</code></span></div>
            <span className="muted small">{formatUI(when(f.last_seen_at))}</span>
          </li>)))}
      </ul>
      {formatUI(res.hasNextPage && <div className="more"><button type="button" className="btn btn--secondary btn--auto" onClick={() => res.fetchNextPage()}>{textUI("m_ac8991ef0101")}</button></div>)}
    </>);
}
export function SimulationDetailPage() {
    useLocale();
    const { id = '' } = useParams();
    const { data: me } = useMe();
    const [params, setParams] = useSearchParams();
    const tab = params.get('tab') ?? 'overview';
    const q = useSimulation(id);
    const act = useSimulationAction(id);
    const path = useAttackPath(id, q.data?.status, advancedEnabled(me));
    const dryRun = useMutation({ mutationFn: () => api.post<{
            executed: boolean;
            items: {
                asset_id: string;
                step: string;
                status: string;
                reason?: string;
                recorded_reason?: string;
                mapping_error?: string;
                inputs?: unknown;
            }[];
        }>(`/api/v1/simulations/${id}/dry-run`) });
    const [cancelling, setCancelling] = useState(false);
    if (q.isPending)
        return <section className="page"><span className="skeleton"/></section>;
    if (q.isError || !q.data) {
        return <section className="page"><div className="state state--error" role="alert"><h2>{textUI("m_f28ec5c24c6f")}</h2><p>{formatUI(describeError(q.error).message)}</p><Link className="btn btn--secondary btn--auto" to="/simulations">{textUI("m_aabb9a3ec8d7")}</Link></div></section>;
    }
    const s = q.data;
    const live = s.status === 'running' || s.status === 'paused';
    const pct = s.step_count === 0 ? 0 : Math.round((s.steps_done / s.step_count) * 100);
    const canRun = can(me, 'simulation.run') && !s.is_demo;
    const TabLink = ({ k, label }: {
        k: string;
        label: string;
    }) => (<button type="button" role="tab" aria-selected={tab === k} className={`tab ${tab === k ? 'is-active' : ''}`} onClick={() => setParams(k === 'overview' ? {} : { tab: k })}>{formatUI(label)}</button>);
    return (<section className="page" aria-labelledby="sim-title">
      <div className="page__head">
        <div>
          <nav className="crumbs" aria-label={textUI("m_2bd873d6c734")}><Link to="/simulations">{textUI("m_078a864ea0f1")}</Link><ChevronRight size={12} aria-hidden="true"/><span className="crumbs__current">{formatUI(s.code)}</span></nav>
          <div className="page__title-row"><h1 id="sim-title" className="page__title">{formatUI(s.name)}</h1><SimBadge status={s.status}/></div>
          <p className="page__sub">{formatUI(s.code)} · {formatUI(s.scenario_id && advancedEnabled(me) ? <Link to={`/chains/${s.scenario_id}/setup`}>{formatUI(s.scenario_name)}</Link> : textUI("m_643f23b836de"))} · {formatUI(s.target_count)} {formatUI(s.target_count === 1 ? 'target' : 'targets')}</p>
        </div>
        {formatUI(canRun && live && (<div className="page__actions">
            {formatUI(s.status === 'running' ? <button type="button" className="btn btn--secondary btn--auto" disabled={act.isPending} onClick={() => act.mutate({ action: 'pause' })}>{richUI("m_df41de5d946f", [<Pause size={15} aria-hidden="true"/>])}</button>
                : <button type="button" className="btn btn--primary btn--auto" disabled={act.isPending} onClick={() => act.mutate({ action: 'resume' })}>{richUI("m_cb7328d44e70", [<Play size={15} aria-hidden="true"/>])}</button>)}
            <button type="button" className="btn btn--danger btn--auto" onClick={() => setCancelling(true)}>{richUI("m_e0aa95bd3a1c", [<Square size={14} aria-hidden="true"/>])}</button>
          </div>))}
      </div>
      {formatUI(s.is_demo && <div className="notice" role="status">{richUI("m_77bb9f0652cc", [<strong>{textUI("m_ae1af496e91c")}</strong>])}</div>)}
      {formatUI(act.isError && <div className="alert" role="alert"><strong>{textUI("m_a12d349fae9f")}</strong><span>{formatUI(describeError(act.error).message)}</span></div>)}
      {formatUI(s.cancel_reason && <div className="notice" role="status"><strong>{formatUI(s.status === 'failed' ? textUI("m_48e9e23f27bc") : textUI("m_f8ec77e7a3a0"))}</strong> {formatUI(s.cancel_reason)}</div>)}

      <div className="tabs" role="tablist" aria-label={textUI("m_97dddbacabd6")}><TabLink k="overview" label={textUI("m_d4b1ea5708dd")}/><TabLink k="findings" label={textUI("m_bbd683ae7fb9", [s.finding_count])}/>{formatUI(advancedEnabled(me) && <TabLink k="path" label={textUI("m_9dcc7a59b4b9")}/>)}</div>

      {formatUI(tab === 'overview' && (<>
          <div className="kpis kpis--3">
            <div className="kpi"><div className="kpi__link"><span className="kpi__label">{textUI("m_4664827f8e89")}</span><strong className="kpi__value">{formatUI(pct)}%</strong><span className="kpi__note">{richUI("m_b3c682a70d10", [formatUI(s.steps_done), formatUI(s.step_count)])}</span></div></div>
            <div className="kpi"><div className="kpi__link"><span className="kpi__label">{formatUI(live ? textUI("m_2958ac7c7c1b") : textUI("m_168d36c2d331"))}</span><strong className="kpi__value">{formatUI(elapsed(s.started_at, s.ended_at))}</strong><span className="kpi__note">{richUI("m_d9debe686819", [formatUI(when(s.started_at))])}</span></div></div>
            <div className="kpi"><div className="kpi__link"><span className="kpi__label">{textUI("m_e171c2ff25b5")}</span><strong className="kpi__value">{formatUI(s.finding_count)}</strong><span className="kpi__note">&nbsp;</span></div></div>
          </div>
          <div className="progress" role="progressbar" aria-valuenow={pct} aria-valuemin={0} aria-valuemax={100} aria-label={textUI("m_3b225a3b32b8")}><span style={{ width: `${pct}%` }}/></div>
          <div className="list" role="table" aria-label={textUI("m_1de3df70ddf4")} tabIndex={0}>
            <div className="list__head step-row" role="row"><span role="columnheader">{textUI("m_78e75a25d809")}</span><span role="columnheader">{textUI("m_8c2e4a035f5f")}</span><span role="columnheader">{textUI("m_9805e88d93aa")}</span><span role="columnheader">{textUI("m_6b76958c1ab8")}</span><span role="columnheader">{textUI("m_a628d0e473bf")}</span></div>
            {formatUI(s.steps.map((st) => (<div key={st.key} className="asset-row step-row" role="row">
                <span role="cell" className="asset-row__text"><strong>{formatUI(st.title)}</strong><span className="muted small">{formatUI(st.contract_name)}</span></span>
                <span role="cell"><StepBadge status={st.status}/></span>
                <span role="cell" className="small">{formatUI(st.trigger_mode === 'on_finding' ? textUI("m_7c7f18e81f37", [st.trigger?.type?.replace('_', ' '), st.trigger?.values?.length ? ` ${st.trigger.values.join(', ')}` : '']) : textUI("m_a15646e5902c"))}{formatUI(st.delay_seconds ? textUI("m_84c22471d8b0", [st.delay_seconds]) : '')}</span>
                <span role="cell" className="small">{formatUI(st.executions.total ? textUI("m_78b5d2778db1", [st.executions.success ?? 0, st.executions.total]) : '—')}</span>
                <span role="cell">{formatUI(st.finding_count)}</span>
              </div>)))}
          </div>
        </>))}
      {formatUI(tab === 'findings' && <FindingsTab id={id}/>)}
      {formatUI(tab === 'path' && advancedEnabled(me) && (<div className="panel">
          <h2 className="panel__heading">{textUI("m_9dcc7a59b4b9")}</h2>
          <p className="muted small">{textUI("m_73ad6e6671f8")}</p>
          {formatUI(path.isPending ? <span className="skeleton"/> : path.isError ? <p role="alert">{formatUI(describeError(path.error).message)}</p> : <AttackPathView path={path.data}/>)}
          {formatUI(path.data?.truncated && <p className="muted small">{textUI("m_0cfefb16aa22")}</p>)}
        </div>))}

      {formatUI(advancedEnabled(me) && can(me, 'finding.read') && can(me, 'asset.read') && <section className="panel"><h2 className="panel__heading">{textUI("m_e41b4ce9c067")}</h2><p className="muted small">{textUI("m_f45e3f43d6d0")}</p><button className="btn btn--secondary btn--auto" disabled={dryRun.isPending} onClick={() => dryRun.mutate()}>{textUI("m_292478ce20c2")}</button>{formatUI(dryRun.error && <p role="alert">{formatUI(describeError(dryRun.error).message)}</p>)}{formatUI(dryRun.data?.items.map((row, i) => <article className="org-card" key={i}><h3>{formatUI(row.step)} · {formatUI(row.status)}</h3><p>{formatUI(row.recorded_reason || row.reason || textUI("m_134b3af50073"))}</p><p className="muted small">{richUI("m_814cf889d372", [formatUI(row.asset_id)])}</p>{formatUI(row.mapping_error ? <p>{formatUI(row.mapping_error)}</p> : <pre className="mono">{JSON.stringify(row.inputs, null, 2)}</pre>)}</article>))}</section>)}
      {formatUI(cancelling && (<ConfirmDialog title={textUI("m_6138e9c04b00")} body="Queued and running actions are stopped. Findings already recorded are kept. This can’t be undone." confirmLabel="Stop simulation" busy={act.isPending} error={act.isError ? describeError(act.error).message : undefined} onConfirm={() => act.mutate({ action: 'cancel', reason: 'Stopped by user' }, { onSuccess: () => setCancelling(false) })} onClose={() => setCancelling(false)}/>))}
    </section>);
}

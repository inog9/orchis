import { t as textUI, tr as richUI, useLocale, fv as formatUI } from "../i18n/runtime";
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
    if (key.startsWith('agent.'))
        return textUI("m_93bdf12a2678");
    if (key !== 'nmap.tcp_connect' && !(p as NucleiParams).severity)
        return textUI("m_0f1fe6ecad80");
    if (key === 'nmap.tcp_connect') {
        const n = p as NmapParams;
        const ports = n.profile === 'custom' ? textUI("m_40adb5d58880", [n.ports]) : n.profile === 'web' ? textUI("m_2610a6ae1bde") : textUI("m_c59afc84897e", [n.profile.replace('top', '')]);
        return `${ports}${n.service_detection ? ', service detection' : ''}`;
    }
    const n = p as NucleiParams;
    return textUI("m_0cd63db91be5", [n.severity.join(', '), n.tags?.length ? textUI("m_326599724708", [n.tags.join(', ')]) : '']);
}
export function AtomicTestDetailPage() {
    useLocale();
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
    useEffect(() => {
        if (latest)
            setSelected(latest);
    }, [latest]);
    const exec = useExecution(selected);
    if (q.isPending)
        return <section className="page"><span className="skeleton"/></section>;
    if (q.isError || !q.data) {
        return (<section className="page"><div className="state state--error" role="alert">
        <h2>{textUI("m_bf014fee7b0a")}</h2><p>{formatUI(describeError(q.error).message)}</p>
        <Link className="btn btn--secondary btn--auto" to="/atomic">{textUI("m_d29548026a0c")}</Link>
      </div></section>);
    }
    const t = q.data;
    const active = t.last_execution && t.last_execution.state !== 'ended';
    const e = exec.data;
    return (<section className="page" aria-labelledby="atomic-detail-title">
      <div className="page__head">
        <div>
          <nav className="crumbs" aria-label={textUI("m_2bd873d6c734")}><Link to="/atomic">{textUI("m_ad1451c7094d")}</Link><ChevronRight size={12} aria-hidden="true"/><span className="crumbs__current">{formatUI(t.name)}</span></nav>
          <div className="page__title-row">
            <h1 id="atomic-detail-title" className="page__title">{formatUI(t.name)}</h1>
            <RunBadge run={t.last_execution}/>
          </div>
          <p className="page__sub">{richUI("m_fc6de8f37c09", [formatUI(t.contract_name), <strong>{formatUI(t.target_asset_name ?? textUI("m_8581bd739761"))}</strong>, formatUI(describeParams(t.contract_key, t.params))])}</p>
        </div>
        <div className="page__actions">
          {formatUI(can(me, 'atomic.run') && !active && (<button type="button" className="btn btn--primary btn--auto" onClick={() => run.mutate()} disabled={run.isPending}><Play size={15} aria-hidden="true"/> {formatUI(run.isPending ? textUI("m_bbe5fc3b9ef3") : textUI("m_00d60e31a4e6"))}</button>))}
          {formatUI(can(me, 'atomic.run') && active && t.last_execution && (<button type="button" className="btn btn--secondary btn--auto" onClick={() => cancel.mutate(t.last_execution!.id)} disabled={cancel.isPending}>{richUI("m_e0aa95bd3a1c", [<Square size={14} aria-hidden="true"/>])}</button>))}
          {formatUI(can(me, 'atomic.delete') && (<button type="button" className="btn btn--danger btn--auto" onClick={() => setConfirmDelete(true)}>{richUI("m_03384eb1560f", [<Trash2 size={15} aria-hidden="true"/>])}</button>))}
        </div>
      </div>
      {formatUI(e?.is_demo && <p className="notice" role="status">{richUI("m_2c8c514058b0", [<strong>{textUI("m_625ef09cc624")}</strong>])}</p>)}
      {formatUI(run.isError && <div className="alert" role="alert"><strong>{textUI("m_e246c5427ba8")}</strong><span>{formatUI(describeError(run.error).message)}</span></div>)}
      {formatUI(cancel.isError && <div className="alert" role="alert"><strong>{textUI("m_5f4e98fd3129")}</strong><span>{formatUI(describeError(cancel.error).message)}</span></div>)}

      <TestPrerequisites targetID={t.target_asset_id} needsAgent={t.contract_key.startsWith('agent.')}/>
      <div className="split">
        <div className="panel">
          <h2 className="panel__title">{textUI("m_848f54e89660")}</h2>
          {formatUI((t.executions ?? []).length === 0 ? <p className="muted pad">{textUI("m_bf16e2f67e14")}</p> : (<ul className="runs">
              {formatUI(t.executions!.map((r) => (<li key={r.id}>
                  <button type="button" className={`runs__item ${selected === r.id ? 'is-on' : ''}`} onClick={() => setSelected(r.id)} aria-pressed={selected === r.id}>
                    <span>{formatUI(when(r.created_at))}</span>
                    <RunBadge run={r}/>
                    <span className="muted small">{formatUI(r.finding_count)} {formatUI(r.finding_count === 1 ? 'finding' : 'findings')}</span>
                  </button>
                </li>)))}
            </ul>))}
        </div>

        <div className="panel" aria-live="polite">
          {formatUI(!e ? <p className="muted pad">{formatUI(selected ? textUI("m_ba3bbbe10d8b") : textUI("m_c1b68508e19e"))}</p> : (<>
              <h2 className="panel__title">{richUI("m_ae4429c818a0", [<RunBadge run={e}/>])}</h2>
              <dl className="kv">
                <div><dt>{textUI("m_978354db0c00")}</dt><dd className="mono">{formatUI(e.target_address)}</dd></div>
                <div><dt>{textUI("m_ecbc89cd37a0")}</dt><dd>{formatUI(when(e.started_at ?? e.created_at))}</dd></div>
                <div><dt>{textUI("m_7804f7a79a9e")}</dt><dd>{formatUI(when(e.ended_at))}</dd></div>
              </dl>
              {formatUI(e.error && <div className="alert" role="alert"><strong>{textUI("m_531d6f75750c")}</strong><span>{formatUI(e.error)}</span></div>)}
              <h3 className="panel__sub">{richUI("m_bbd683ae7fb9", [formatUI(e.findings.length)])}</h3>
              {formatUI(e.state !== 'ended' ? <p className="muted pad">{textUI("m_6cb003bced1e")}</p> : e.findings.length === 0 ? (<p className="muted pad">{formatUI(e.outcome === 'success' ? textUI("m_a138a77f64b2") : textUI("m_1d10914064be"))}</p>) : (<ul className="finds">
                  {formatUI(e.findings.map((f) => (<li key={f.id}><SeverityBadge severity={f.severity}/><span className="finds__title">{formatUI(f.title)}</span><code className="mono small">{f.value}</code></li>)))}
                </ul>))}
              <h3 className="panel__sub">{textUI("m_21e49eb2b299")}</h3>
              <ol className="log">
                {formatUI(e.logs.map((l, i) => <li key={i} className={`log--${l.level}`}><time>{formatUI(when(l.at))}</time> {formatUI(l.message)}</li>))}
              </ol>
            </>))}
        </div>
      </div>
      {formatUI(confirmDelete && (<ConfirmDialog title={textUI("m_8d0ac15b4717")} body="The test and its run history are removed. Findings it discovered stay in Findings." confirmLabel="Delete test" busy={del.isPending} error={del.isError ? describeError(del.error).message : undefined} onConfirm={() => del.mutate(undefined, { onSuccess: () => navigate('/atomic') })} onClose={() => setConfirmDelete(false)}/>))}
    </section>);
}

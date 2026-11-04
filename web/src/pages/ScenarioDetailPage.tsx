import { t as textUI, tr as richUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { ChevronRight, Network, Play, Trash2, X } from 'lucide-react';
import { useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { useAllAssets, useDeleteScenario, useGroups, useLaunch, useMe, useScenario, useSetTargets } from '../api/hooks';
import { can, type ScenarioDetail } from '../api/types';
import { describeError } from '../components/ErrorNote';
import { Modal } from '../components/Modal';
import { ScenarioSeverity } from '../components/StatusPills';
import { ConfirmDialog } from './ConfirmDialog';
function TargetsDialog({ sc, onClose }: {
    sc: ScenarioDetail;
    onClose: () => void;
}) {
    useLocale();
    const assets = useAllAssets();
    const groups = useGroups();
    const save = useSetTargets(sc.id);
    const [a, setA] = useState(new Set(sc.targets.filter((t) => t.kind === 'asset').map((t) => t.id)));
    const [g, setG] = useState(new Set(sc.targets.filter((t) => t.kind === 'group').map((t) => t.id)));
    const toggle = (set: Set<string>, id: string, put: (s: Set<string>) => void) => { const n = new Set(set); if (n.has(id))
        n.delete(id);
    else
        n.add(id); put(n); };
    return (<Modal labelledBy="targets-title" onClose={onClose} width={560}>
      <>
        <header className="dialog__head"><h2 id="targets-title">{textUI("m_5ff5c9fe1bac")}</h2><button type="button" className="icon-btn" aria-label={textUI("m_7d9eb7acb13e")} onClick={onClose}><X size={18}/></button></header>
        <p className="subtitle">{textUI("m_202e479c86d8")}</p>
        <fieldset className="fieldset"><legend>{textUI("m_bd12731d7bc9")}</legend>
          <div className="checklist">
            {formatUI((assets.data ?? []).map((x) => (<label key={x.id} className="check"><input type="checkbox" checked={a.has(x.id)} onChange={() => toggle(a, x.id, setA)}/><span className="check__box" aria-hidden="true"/>
                <span>{formatUI(x.name)}{formatUI(!x.testing_authorized && <small className="check__sub">{textUI("m_ea75138c6bee")}</small>)}</span></label>)))}
            {formatUI(assets.data?.length === 0 && <p className="muted small">{textUI("m_2ab50eac38cb")}</p>)}
          </div>
        </fieldset>
        <fieldset className="fieldset"><legend>{textUI("m_39dd12de3b02")}</legend>
          <div className="checklist">
            {formatUI((groups.data ?? []).map((x) => (<label key={x.id} className="check"><input type="checkbox" checked={g.has(x.id)} onChange={() => toggle(g, x.id, setG)}/><span className="check__box" aria-hidden="true"/>
                <span>{formatUI(x.name)}<small className="check__sub">{formatUI(x.member_count)} {formatUI(x.member_count === 1 ? 'asset' : 'assets')}</small></span></label>)))}
          </div>
        </fieldset>
        {formatUI(save.isError && <div className="alert" role="alert"><strong>{textUI("m_f2708df3bab9")}</strong><span>{formatUI(describeError(save.error).message)}</span></div>)}
        <footer className="dialog__foot">
          <button type="button" className="btn btn--secondary btn--auto" onClick={onClose}>{textUI("m_19766ed6ccb2")}</button>
          <button type="button" className="btn btn--primary btn--auto" disabled={save.isPending} onClick={() => save.mutate({ version: sc.version, asset_ids: [...a], group_ids: [...g] }, { onSuccess: onClose })}>{formatUI(save.isPending ? textUI("m_23e39291d613") : textUI("m_61c2c81d1bbb"))}</button>
        </footer>
      </>
    </Modal>);
}
function LaunchDialog({ sc, onClose, onLaunched }: {
    sc: ScenarioDetail;
    onClose: () => void;
    onLaunched: (id: string) => void;
}) {
    useLocale();
    const launch = useLaunch(sc.id);
    const [name, setName] = useState('');
    return (<Modal labelledBy="launch-title" onClose={onClose} width={520}>
      <>
        <header className="dialog__head"><h2 id="launch-title">{richUI("m_5290d984198b", [formatUI(sc.name)])}</h2><button type="button" className="icon-btn" aria-label={textUI("m_7d9eb7acb13e")} onClick={onClose}><X size={18}/></button></header>
        <p className="subtitle">{richUI("m_7af5abdf9c9b", [formatUI(sc.steps.filter((s) => s.is_enabled).length), formatUI(sc.targets.length)])}</p>
        <div className="field">
          <div className="field__label-row"><label htmlFor="sim-name" className="field__label">{textUI("m_20bf3ee6dfac")}</label></div>
          <div className="field__input"><input id="sim-name" value={name} onChange={(e) => setName(e.target.value)} placeholder={sc.name}/></div>
        </div>
        {formatUI(launch.isError && <div className="alert" role="alert"><strong>{textUI("m_1baad0fcfa1c")}</strong><span>{formatUI(describeError(launch.error).message)}</span></div>)}
        <footer className="dialog__foot">
          <button type="button" className="btn btn--secondary btn--auto" onClick={onClose}>{textUI("m_19766ed6ccb2")}</button>
          <button type="button" className="btn btn--primary btn--auto" disabled={launch.isPending} onClick={() => launch.mutate(name.trim(), { onSuccess: (s) => onLaunched(s.id) })}>{formatUI(launch.isPending ? textUI("m_a6f90e03c4f2") : textUI("m_b3b76a9af806"))}</button>
        </footer>
      </>
    </Modal>);
}
export function ScenarioDetailPage() {
    useLocale();
    const { id = '' } = useParams();
    const { data: me } = useMe();
    const navigate = useNavigate();
    const q = useScenario(id);
    const del = useDeleteScenario(id);
    const [dialog, setDialog] = useState<'targets' | 'launch' | 'delete' | null>(null);
    if (q.isPending)
        return <section className="page"><span className="skeleton"/></section>;
    if (q.isError || !q.data) {
        return <section className="page"><div className="state state--error" role="alert"><h2>{textUI("m_ff4bc490d607")}</h2><p>{formatUI(describeError(q.error).message)}</p><Link className="btn btn--secondary btn--auto" to="/chains">{textUI("m_0b1e48affc33")}</Link></div></section>;
    }
    const sc = q.data;
    const ready = sc.problems.length === 0;
    return (<section className="page" aria-labelledby="scenario-title">
      <div className="page__head">
        <div>
          <nav className="crumbs" aria-label={textUI("m_2bd873d6c734")}><Link to="/chains">{textUI("m_72fa5509b2f9")}</Link><ChevronRight size={12} aria-hidden="true"/><span className="crumbs__current">{formatUI(sc.name)}</span></nav>
          <div className="page__title-row"><h1 id="scenario-title" className="page__title">{formatUI(sc.name)}</h1><span className={`status ${ready ? 'status--ok' : ''}`}>{formatUI(ready ? textUI("m_5fa7aac5375c") : textUI("m_ebf12ef47cf5"))}</span></div>
          <p className="page__sub">{formatUI(sc.description || textUI("m_06a694820920"))}</p>
        </div>
        <div className="page__actions">
          <Link className="btn btn--secondary btn--auto" to={`/chains/${sc.id}`}>{richUI("m_c32451fcb48d", [<Network size={15} aria-hidden="true"/>])}</Link>
          {formatUI(can(me, 'simulation.run') && <button type="button" className="btn btn--primary btn--auto" disabled={!ready} onClick={() => setDialog('launch')}>{richUI("m_22e1ee0c9c30", [<Play size={15} aria-hidden="true"/>])}</button>)}
          {formatUI(can(me, 'scenario.delete') && <button type="button" className="btn btn--danger btn--auto" onClick={() => setDialog('delete')}>{richUI("m_03384eb1560f", [<Trash2 size={15} aria-hidden="true"/>])}</button>)}
        </div>
      </div>

      {formatUI(!ready && (<div className="notice" role="status"><strong>{textUI("m_37e2c5a1a94e")}</strong>
          <ul className="problems">{formatUI(sc.problems.map((p) => <li key={p}>{formatUI(p)}</li>))}</ul></div>))}

      <div className="split split--even">
        <div className="panel">
          <h2 className="panel__title">{textUI("m_8e76a94ac832")}</h2>
          <dl className="kv kv--2">
            <div><dt>{textUI("m_5e9f98120dbe")}</dt><dd><ScenarioSeverity severity={sc.severity}/></dd></div>
            <div><dt>{textUI("m_292c06f0045a")}</dt><dd>{formatUI(sc.category ?? '—')}</dd></div>
            <div><dt>{textUI("m_ff8059dc6752")}</dt><dd>{formatUI(sc.step_count)}</dd></div>
            <div><dt>{textUI("m_2e41b118eb20")}</dt><dd>{formatUI(sc.edge_count)}</dd></div>
            <div><dt>{textUI("m_13c5a6c1c5e9")}</dt><dd><Link to="/simulations">{formatUI(sc.simulations)}</Link></dd></div>
          </dl>
        </div>
        <div className="panel">
          <div className="posture__head"><h2 className="panel__title">{textUI("m_27445f6ab61d")}</h2>
            {formatUI(can(me, 'scenario.write') && <button type="button" className="btn btn--secondary btn--sm" onClick={() => setDialog('targets')}>{textUI("m_e3b185e8906f")}</button>)}</div>
          {formatUI(sc.targets.length === 0 ? <p className="muted pad">{textUI("m_4f060f10be90")}</p> : (<ul className="mini-list pad">{formatUI(sc.targets.map((t) => (<li key={t.kind + t.id}><div className="mini-list__main"><strong>{formatUI(t.name)}</strong><span className="muted small">{formatUI(t.kind === 'group' ? textUI("m_0cdcb5be16fd") : textUI("m_80d298c9f240"))}</span></div>
                {formatUI(t.kind === 'asset' && <span className={`status ${t.authorized ? 'status--ok' : 'status--bad'}`}>{formatUI(t.authorized ? textUI("m_67f542ba6d8f") : textUI("m_52c79c7b5ba0"))}</span>)}</li>)))}</ul>))}
        </div>
      </div>

      <div className="panel">
        <div className="posture__head"><h2 className="panel__title">{textUI("m_ff8059dc6752")}</h2><Link to={`/chains/${sc.id}`} className="link-quiet">{textUI("m_67643371965d")}</Link></div>
        {formatUI(sc.steps.length === 0 ? <p className="muted pad">{textUI("m_1b21fffe972d")}</p> : (<ol className="steps-flow pad">{formatUI(sc.steps.map((s) => (<li key={s.key} className={s.is_enabled ? '' : 'is-off'}><strong>{formatUI(s.title)}</strong><span className="muted small">{formatUI(s.contract_key)} · {formatUI(s.trigger_mode === 'on_finding' ? textUI("m_28ec3dea2d85", [s.trigger?.type?.replace('_', ' ')]) : textUI("m_499c1aa507a9"))}</span></li>)))}</ol>))}
      </div>

      {formatUI(dialog === 'targets' && <TargetsDialog sc={sc} onClose={() => setDialog(null)}/>)}
      {formatUI(dialog === 'launch' && <LaunchDialog sc={sc} onClose={() => setDialog(null)} onLaunched={(sid) => navigate(`/simulations/legacy/${sid}`)}/>)}
      {formatUI(dialog === 'delete' && (<ConfirmDialog title={textUI("m_110f955fc270")} body="Simulations already run from it stay in the history." confirmLabel="Delete scenario" busy={del.isPending} error={del.isError ? describeError(del.error).message : undefined} onConfirm={() => del.mutate(undefined, { onSuccess: () => navigate('/chains') })} onClose={() => setDialog(null)}/>))}
    </section>);
}

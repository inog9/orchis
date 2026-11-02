import { ChevronRight, Network, Play, Trash2, X } from 'lucide-react';
import { useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { useAllAssets, useDeleteScenario, useGroups, useLaunch, useMe, useScenario, useSetTargets } from '../api/hooks';
import { can, type ScenarioDetail } from '../api/types';
import { describeError } from '../components/ErrorNote';
import { Modal } from '../components/Modal';
import { ScenarioSeverity } from '../components/StatusPills';
import { ConfirmDialog } from './ConfirmDialog';

function TargetsDialog({ sc, onClose }: { sc: ScenarioDetail; onClose: () => void }) {
  const assets = useAllAssets();
  const groups = useGroups();
  const save = useSetTargets(sc.id);
  const [a, setA] = useState(new Set(sc.targets.filter((t) => t.kind === 'asset').map((t) => t.id)));
  const [g, setG] = useState(new Set(sc.targets.filter((t) => t.kind === 'group').map((t) => t.id)));
  const toggle = (set: Set<string>, id: string, put: (s: Set<string>) => void) => { const n = new Set(set); if (n.has(id)) n.delete(id); else n.add(id); put(n); };
  return (
    <Modal labelledBy="targets-title" onClose={onClose} width={560}>
      <>
        <header className="dialog__head"><h2 id="targets-title">Choose targets</h2><button type="button" className="icon-btn" aria-label="Close" onClick={onClose}><X size={18} /></button></header>
        <p className="subtitle">Every action of this chain runs against these assets. Groups are resolved when you launch.</p>
        <fieldset className="fieldset"><legend>Assets</legend>
          <div className="checklist">
            {(assets.data ?? []).map((x) => (
              <label key={x.id} className="check"><input type="checkbox" checked={a.has(x.id)} onChange={() => toggle(a, x.id, setA)} /><span className="check__box" aria-hidden="true" />
                <span>{x.name}{!x.testing_authorized && <small className="check__sub">Not authorized for testing yet</small>}</span></label>
            ))}
            {assets.data?.length === 0 && <p className="muted small">No assets yet.</p>}
          </div>
        </fieldset>
        <fieldset className="fieldset"><legend>Asset groups</legend>
          <div className="checklist">
            {(groups.data ?? []).map((x) => (
              <label key={x.id} className="check"><input type="checkbox" checked={g.has(x.id)} onChange={() => toggle(g, x.id, setG)} /><span className="check__box" aria-hidden="true" />
                <span>{x.name}<small className="check__sub">{x.member_count} {x.member_count === 1 ? 'asset' : 'assets'}</small></span></label>
            ))}
          </div>
        </fieldset>
        {save.isError && <div className="alert" role="alert"><strong>Couldn’t save targets</strong><span>{describeError(save.error).message}</span></div>}
        <footer className="dialog__foot">
          <button type="button" className="btn btn--secondary btn--auto" onClick={onClose}>Cancel</button>
          <button type="button" className="btn btn--primary btn--auto" disabled={save.isPending} onClick={() => save.mutate({ version: sc.version, asset_ids: [...a], group_ids: [...g] }, { onSuccess: onClose })}>{save.isPending ? 'Saving…' : 'Save targets'}</button>
        </footer>
      </>
    </Modal>
  );
}

function LaunchDialog({ sc, onClose, onLaunched }: { sc: ScenarioDetail; onClose: () => void; onLaunched: (id: string) => void }) {
  const launch = useLaunch(sc.id);
  const [name, setName] = useState('');
  return (
    <Modal labelledBy="launch-title" onClose={onClose} width={520}>
      <>
        <header className="dialog__head"><h2 id="launch-title">Launch “{sc.name}”?</h2><button type="button" className="icon-btn" aria-label="Close" onClick={onClose}><X size={18} /></button></header>
        <p className="subtitle">This runs {sc.steps.filter((s) => s.is_enabled).length} action(s) against {sc.targets.length} target(s) now. Only assets authorized for testing can be targeted. You can pause or stop the simulation at any time.</p>
        <div className="field">
          <div className="field__label-row"><label htmlFor="sim-name" className="field__label">Simulation name (optional)</label></div>
          <div className="field__input"><input id="sim-name" value={name} onChange={(e) => setName(e.target.value)} placeholder={sc.name} /></div>
        </div>
        {launch.isError && <div className="alert" role="alert"><strong>Couldn’t launch</strong><span>{describeError(launch.error).message}</span></div>}
        <footer className="dialog__foot">
          <button type="button" className="btn btn--secondary btn--auto" onClick={onClose}>Cancel</button>
          <button type="button" className="btn btn--primary btn--auto" disabled={launch.isPending} onClick={() => launch.mutate(name.trim(), { onSuccess: (s) => onLaunched(s.id) })}>{launch.isPending ? 'Launching…' : 'Launch simulation'}</button>
        </footer>
      </>
    </Modal>
  );
}

export function ScenarioDetailPage() {
  const { id = '' } = useParams();
  const { data: me } = useMe();
  const navigate = useNavigate();
  const q = useScenario(id);
  const del = useDeleteScenario(id);
  const [dialog, setDialog] = useState<'targets' | 'launch' | 'delete' | null>(null);

  if (q.isPending) return <section className="page"><span className="skeleton" /></section>;
  if (q.isError || !q.data) {
    return <section className="page"><div className="state state--error" role="alert"><h2>Couldn’t load this scenario</h2><p>{describeError(q.error).message}</p><Link className="btn btn--secondary btn--auto" to="/chains">Back to chains</Link></div></section>;
  }
  const sc = q.data;
  const ready = sc.problems.length === 0;
  return (
    <section className="page" aria-labelledby="scenario-title">
      <div className="page__head">
        <div>
          <nav className="crumbs" aria-label="Breadcrumb"><Link to="/chains">Scenarios</Link><ChevronRight size={12} aria-hidden="true" /><span className="crumbs__current">{sc.name}</span></nav>
          <div className="page__title-row"><h1 id="scenario-title" className="page__title">{sc.name}</h1><span className={`status ${ready ? 'status--ok' : ''}`}>{ready ? 'Ready' : 'Draft'}</span></div>
          <p className="page__sub">{sc.description || 'No description.'}</p>
        </div>
        <div className="page__actions">
          <Link className="btn btn--secondary btn--auto" to={`/chains/${sc.id}`}><Network size={15} aria-hidden="true" /> Open chain workspace</Link>
          {can(me, 'simulation.run') && <button type="button" className="btn btn--primary btn--auto" disabled={!ready} onClick={() => setDialog('launch')}><Play size={15} aria-hidden="true" /> Launch simulation</button>}
          {can(me, 'scenario.delete') && <button type="button" className="btn btn--danger btn--auto" onClick={() => setDialog('delete')}><Trash2 size={15} aria-hidden="true" /> Delete</button>}
        </div>
      </div>

      {!ready && (
        <div className="notice" role="status"><strong>Not ready to launch yet.</strong>
          <ul className="problems">{sc.problems.map((p) => <li key={p}>{p}</li>)}</ul></div>
      )}

      <div className="split split--even">
        <div className="panel">
          <h2 className="panel__title">Summary</h2>
          <dl className="kv kv--2">
            <div><dt>Severity</dt><dd><ScenarioSeverity severity={sc.severity} /></dd></div>
            <div><dt>Category</dt><dd>{sc.category ?? '—'}</dd></div>
            <div><dt>Actions</dt><dd>{sc.step_count}</dd></div>
            <div><dt>Dependencies</dt><dd>{sc.edge_count}</dd></div>
            <div><dt>Simulations run</dt><dd><Link to="/simulations">{sc.simulations}</Link></dd></div>
          </dl>
        </div>
        <div className="panel">
          <div className="posture__head"><h2 className="panel__title">Targets</h2>
            {can(me, 'scenario.write') && <button type="button" className="btn btn--secondary btn--sm" onClick={() => setDialog('targets')}>Edit targets</button>}</div>
          {sc.targets.length === 0 ? <p className="muted pad">No targets chosen yet.</p> : (
            <ul className="mini-list pad">{sc.targets.map((t) => (
              <li key={t.kind + t.id}><div className="mini-list__main"><strong>{t.name}</strong><span className="muted small">{t.kind === 'group' ? 'Asset group' : 'Asset'}</span></div>
                {t.kind === 'asset' && <span className={`status ${t.authorized ? 'status--ok' : 'status--bad'}`}>{t.authorized ? 'Authorized' : 'Not authorized'}</span>}</li>
            ))}</ul>
          )}
        </div>
      </div>

      <div className="panel">
        <div className="posture__head"><h2 className="panel__title">Actions</h2><Link to={`/chains/${sc.id}`} className="link-quiet">Edit in workspace</Link></div>
        {sc.steps.length === 0 ? <p className="muted pad">No actions yet. Open the chain workspace to add some.</p> : (
          <ol className="steps-flow pad">{sc.steps.map((s) => (
            <li key={s.key} className={s.is_enabled ? '' : 'is-off'}><strong>{s.title}</strong><span className="muted small">{s.contract_key} · {s.trigger_mode === 'on_finding' ? `when a parent finds ${s.trigger?.type?.replace('_', ' ')}` : 'after its parents'}</span></li>
          ))}</ol>
        )}
      </div>

      {dialog === 'targets' && <TargetsDialog sc={sc} onClose={() => setDialog(null)} />}
      {dialog === 'launch' && <LaunchDialog sc={sc} onClose={() => setDialog(null)} onLaunched={(sid) => navigate(`/simulations/legacy/${sid}`)} />}
      {dialog === 'delete' && (
        <ConfirmDialog title="Delete this scenario?" body="Simulations already run from it stay in the history." confirmLabel="Delete scenario" busy={del.isPending}
          error={del.isError ? describeError(del.error).message : undefined} onConfirm={() => del.mutate(undefined, { onSuccess: () => navigate('/chains') })} onClose={() => setDialog(null)} />
      )}
    </section>
  );
}

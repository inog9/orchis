import { ExternalLink, X } from 'lucide-react';
import { useState } from 'react';
import { useMitreMatrix, useMitreTechnique } from '../api/hooks';
import type { MitreCell } from '../api/types';
import { describeError } from '../components/ErrorNote';
import { when } from '../components/RunBadges';

function Detail({ id, onClose }: { id: string; onClose: () => void }) {
  const q = useMitreTechnique(id);
  return (
    <section className="panel inspector" aria-label="Technique details">
      <div className="posture__head"><h2 className="panel__heading">{q.data ? `${q.data.id} · ${q.data.name}` : id}</h2><button type="button" className="icon-btn" aria-label="Close details" onClick={onClose}><X size={16} /></button></div>
      {q.isPending ? <span className="skeleton" /> : q.isError ? <p role="alert">{describeError(q.error).message}</p> : (
        <>
          <p className="muted small">{q.data.tactics.join(' · ')}{q.data.platforms.length ? ` · ${q.data.platforms.join(', ')}` : ''}</p>
          <p className="desc">{q.data.description || 'No description.'}</p>
          <dl className="kv kv--2">
            <div><dt>Exercised</dt><dd>{q.data.runs > 0 ? `${q.data.runs} successful ${q.data.runs === 1 ? 'run' : 'runs'}` : 'Not yet'}</dd></div>
            <div><dt>Last run</dt><dd>{when(q.data.last_run_at)}</dd></div>
          </dl>
          <p className="muted small">{q.data.actions.length ? `Actions that exercise it: ${q.data.actions.join(', ')}.` : 'No platform action exercises this technique yet.'}</p>
          <a className="btn btn--secondary btn--sm" href={q.data.url} target="_blank" rel="noopener noreferrer">View on MITRE <ExternalLink size={13} aria-hidden="true" /></a>
        </>
      )}
    </section>
  );
}

function Cell({ c, selected, onPick }: { c: MitreCell; selected: boolean; onPick: (id: string) => void }) {
  const state = c.exercised ? 'exercised' : c.mapped ? 'mapped' : 'idle';
  const label = `${c.id} ${c.name}: ${c.exercised ? 'exercised' : c.mapped ? 'can be exercised, not yet run' : 'not covered'}`;
  return (
    <button type="button" className={`mcell mcell--${state} ${selected ? 'is-on' : ''}`} aria-label={label} aria-pressed={selected} onClick={() => onPick(c.id)}>
      <span className="mcell__id">{c.id}</span><span className="mcell__name">{c.name}</span>
    </button>
  );
}

export function MitrePage() {
  const q = useMitreMatrix();
  const [only, setOnly] = useState(false);
  const [picked, setPicked] = useState<string | null>(null);
  const m = q.data;
  return (
    <section className="page" aria-labelledby="mitre-title">
      <div className="page__head"><div>
        <h1 id="mitre-title" className="page__title">MITRE ATT&amp;CK</h1>
        <p className="page__sub">Which techniques your actions can exercise, and which you have actually run. Only successful runs count.</p></div></div>
      {q.isPending ? <span className="skeleton" /> : q.isError || !m ? (
        <div className="state state--error" role="alert"><h2>Couldn’t load the matrix</h2><p>{describeError(q.error).message}</p><button type="button" className="btn btn--secondary btn--auto" onClick={() => q.refetch()}>Try again</button></div>
      ) : !m.imported ? (
        <div className="state"><h2>{m.last_run?.status === 'running' ? 'Importing ATT&CK data…' : 'No ATT&CK data yet'}</h2>
          <p>{m.last_run?.status === 'running' ? 'This takes a minute.' : m.last_run?.status === 'failed' ? `The last import failed: ${m.last_run.error ?? 'unknown error'}` : 'The data downloads automatically after the platform starts. An administrator can also import it from Settings.'}</p></div>
      ) : (
        <>
          <ul className="kpis kpis--3" aria-label="Coverage">
            <li className="kpi"><div className="kpi__link"><span className="kpi__label">Techniques</span><strong className="kpi__value">{m.total}</strong><span className="kpi__note">in the ATT&amp;CK matrix</span></div></li>
            <li className="kpi"><div className="kpi__link"><span className="kpi__label">Can be exercised</span><strong className="kpi__value">{m.mapped}</strong><span className="kpi__note">covered by an action</span></div></li>
            <li className="kpi"><div className="kpi__link"><span className="kpi__label">Exercised</span><strong className="kpi__value">{m.exercised}</strong><span className="kpi__note">run successfully</span></div></li>
          </ul>
          <div className="toolbar">
            <label className="check"><input type="checkbox" checked={only} onChange={(e) => setOnly(e.target.checked)} /><span className="check__box" aria-hidden="true" />Only techniques we can exercise</label>
            <span className="legend"><i className="mcell--exercised" /> Exercised <i className="mcell--mapped" /> Can be exercised <i className="mcell--idle" /> Not covered</span>
            {m.last_run?.status === 'succeeded' && <span className="muted small">ATT&amp;CK {m.last_run.source_version ? `v${m.last_run.source_version} · ` : ''}imported {when(m.last_run.finished_at)}</span>}
          </div>
          <div className="workspace">
            <div className="canvas matrix" role="region" aria-label="ATT&CK matrix" tabIndex={0}>
              <div className="matrix__cols">
                {m.tactics.map((t) => {
                  const cells = only ? t.techniques.filter((c) => c.mapped) : t.techniques;
                  return (
                    <section key={t.id} className="mcol" aria-label={t.name}>
                      <h2 className="mcol__title">{t.name}<small>{cells.length}</small></h2>
                      {cells.length === 0 ? <p className="muted small">None</p> : cells.map((c) => <Cell key={c.id} c={c} selected={picked === c.id} onPick={setPicked} />)}
                    </section>
                  );
                })}
              </div>
            </div>
            {picked ? <Detail id={picked} onClose={() => setPicked(null)} /> : <section className="panel inspector"><p className="muted">Select a technique to see what covers it.</p></section>}
          </div>
        </>
      )}
    </section>
  );
}

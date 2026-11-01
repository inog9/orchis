import { ExternalLink, X } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { useMitreMatrix, useMitreTechnique } from '../api/hooks';
import type { MitreCell } from '../api/types';
import { describeError } from '../components/ErrorNote';
import { when } from '../components/RunBadges';

const MITIGATIONS_SHOWN = 6;

function Detail({ id, onClose, onPick }: { id: string; onClose: () => void; onPick: (id: string) => void }) {
  const panelRef = useRef<HTMLElement>(null);
  const q = useMitreTechnique(id);
  const [allMitigations, setAllMitigations] = useState(false);
  const d = q.data;
  const mappings = d?.mappings ?? [];
  const mitigations = d?.mitigations ?? [];
  const shown = allMitigations ? mitigations : mitigations.slice(0, MITIGATIONS_SHOWN);
  useEffect(() => {
    const panel = panelRef.current;
    if (!panel) return;
    const reveal = () => panel.scrollIntoView?.({ block: 'nearest' });
    panel.focus({ preventScroll: true });
    reveal();
    window.addEventListener('resize', reveal);
    return () => window.removeEventListener('resize', reveal);
  }, [id]);
  return (
    <section ref={panelRef} className="panel inspector" aria-label="Technique details" aria-describedby="mitre-detail-heading" tabIndex={-1} onKeyDown={(event) => { if (event.key === 'Escape') { event.preventDefault(); onClose(); } }}>
      <div className="posture__head"><h2 id="mitre-detail-heading" className="panel__heading">{d ? `${d.id} · ${d.name}` : id}</h2><button type="button" className="icon-btn" aria-label="Close details" onClick={onClose}><X size={16} /></button></div>
      {q.isPending ? <span className="skeleton" /> : q.isError || !d ? <p role="alert">{describeError(q.error).message}</p> : (
        <>
          <p className="muted small">{d.tactics.join(' · ')}{d.platforms.length ? ` · ${d.platforms.join(', ')}` : ''}</p>
          {d.deprecated && <p className="muted small">This technique is deprecated in ATT&amp;CK.</p>}
          {d.parent && <p className="muted small">Sub-technique of <button type="button" className="link-subtle" onClick={() => onPick(d.parent!)}>{d.parent}</button>.</p>}
          <div className="desc" role="region" aria-label="Technique description" tabIndex={0}>{d.description || 'No description.'}</div>
          <dl className="kv kv--2">
            <div><dt>Trained</dt><dd>{d.runs > 0 ? `${d.runs} successful ${d.runs === 1 ? 'run' : 'runs'}` : 'Not yet'}</dd></div>
            <div><dt>Last run</dt><dd>{when(d.last_run_at)}</dd></div>
          </dl>
          {d.simulated && <p className="muted small"><strong>Demo / synthetic evidence:</strong> {d.simulated_runs ?? 0} illustrative successful runs and {d.simulated_findings ?? 0} finding references. These never count as trained.</p>}
          <h3 className="panel__sub">How our actions cover it</h3>
          {mappings.length === 0 && d.actions.length > 0 ? <p className="muted small">Actions that exercise it: {d.actions.join(', ')}.</p> : mappings.length === 0 ? <p className="muted small">No platform action is mapped to this technique.</p> : (
            <ul className="stack-list">
              {mappings.map((mp) => (
                <li key={`${mp.action}-${mp.via ?? ''}`}><strong>{mp.action}</strong>{mp.via ? ` (via ${mp.via})` : ''} <span className="muted small">{mp.basis === 'direct' ? 'direct' : 'estimate'}</span><br /><span className="muted small">{mp.reason}</span></li>
              ))}
            </ul>
          )}
          {d.subs.length > 0 && (
            <>
              <h3 className="panel__sub">Sub-techniques ({d.subs.length})</h3>
              <ul className="stack-list">{d.subs.map((sc) => <li key={sc.id}><button type="button" className="link-subtle" onClick={() => onPick(sc.id)}>{sc.id}</button> {sc.name}</li>)}</ul>
            </>
          )}
          {mitigations.length > 0 && (
            <>
              <h3 className="panel__sub">Mitigations ({mitigations.length})</h3>
              <ul className="stack-list">{shown.map((mt) => <li key={mt.id}><strong>{mt.id}</strong> {mt.name}{mt.description ? <><br /><span className="muted small">{mt.description}</span></> : null}</li>)}</ul>
              {mitigations.length > MITIGATIONS_SHOWN && <button type="button" className="btn btn--secondary btn--sm" onClick={() => setAllMitigations(!allMitigations)}>{allMitigations ? 'Show fewer' : `Show all ${mitigations.length}`}</button>}
            </>
          )}
          <a className="btn btn--secondary btn--sm" href={d.url} target="_blank" rel="noopener noreferrer">View on MITRE <ExternalLink size={13} aria-hidden="true" /></a>
        </>
      )}
    </section>
  );
}

// Warna mengutamakan bukti nyata; badge demo tetap terlihat pada status yang beririsan.
function cellState(c: MitreCell): 'exercised' | 'mapped' | 'simulated' | 'idle' {
  return c.exercised ? 'exercised' : c.mapped ? 'mapped' : c.simulated ? 'simulated' : 'idle';
}
const STATE_TEXT = { exercised: 'trained: an action was run successfully', mapped: 'mapped: an action can exercise it, not yet run', simulated: 'simulated: synthetic demo evidence', idle: 'not covered' } as const;

function Cell({ c, selected, onPick }: { c: MitreCell; selected: boolean; onPick: (id: string, button: HTMLButtonElement) => void }) {
  const state = cellState(c);
  return (
    <button type="button" className={`mcell mcell--${state} ${selected ? 'is-on' : ''}`} aria-label={`${c.id} ${c.name}: ${STATE_TEXT[state]}${c.simulated && state !== 'simulated' ? '; simulated: demo evidence also present' : ''}`} aria-pressed={selected} onClick={(event) => onPick(c.id, event.currentTarget)}>
      <span className="mcell__meta"><span className="mcell__id">{c.id}</span>{c.simulated && <span className="mcell__demo">Demo</span>}</span><span className="mcell__name">{c.name}</span>
    </button>
  );
}

function matches(c: MitreCell, term: string, platform: string): boolean {
  const own = (x: MitreCell) => (!term || x.id.toLowerCase().includes(term) || x.name.toLowerCase().includes(term)) && (!platform || (x.platforms ?? []).includes(platform));
  return own(c) || (c.subs ?? []).some(own);
}

export function MitrePage() {
  const q = useMitreMatrix();
  const originRef = useRef<HTMLButtonElement>(null);
  const canvasRef = useRef<HTMLDivElement>(null);
  const [only, setOnly] = useState(false);
  const [platform, setPlatform] = useState('');
  const [search, setSearch] = useState('');
  const [picked, setPicked] = useState<string | null>(null);
  const term = search.trim().toLowerCase();
  const m = q.data;
  const closeDetails = () => {
    setPicked(null);
    const target = originRef.current?.isConnected ? originRef.current : canvasRef.current;
    target?.focus();
    target?.scrollIntoView?.({ block: 'nearest' });
  };
  return (
    <section className="page" aria-labelledby="mitre-title">
      <div className="page__head"><div>
        <h1 id="mitre-title" className="page__title">MITRE ATT&amp;CK</h1>
        <p className="page__sub">Which techniques your actions are mapped to, and which you have actually run. Only successful real runs count as trained; demo data never does.</p></div></div>
      {q.isPending ? <span className="skeleton" /> : q.isError || !m ? (
        <div className="state state--error" role="alert"><h2>Couldn’t load the matrix</h2><p>{describeError(q.error).message}</p><button type="button" className="btn btn--secondary btn--auto" onClick={() => q.refetch()}>Try again</button></div>
      ) : !m.imported ? (
        <div className="state"><h2>{m.last_run?.status === 'running' ? 'Importing ATT&CK data…' : 'No ATT&CK data yet'}</h2>
          <p>{m.last_run?.status === 'running' ? 'This takes a minute.' : m.last_run?.status === 'failed' ? `The last import failed: ${m.last_run.error ?? 'unknown error'}` : 'The data downloads automatically after the platform starts. An administrator can also import it from Settings.'}</p></div>
      ) : (
        <>
          <ul className="kpis kpis--4" aria-label="Coverage">
            <li className="kpi"><div className="kpi__link"><span className="kpi__label">Techniques</span><strong className="kpi__value">{m.total}</strong><span className="kpi__note">in the ATT&amp;CK matrix</span></div></li>
            <li className="kpi"><div className="kpi__link"><span className="kpi__label">Mapped</span><strong className="kpi__value">{m.mapped}</strong><span className="kpi__note">an action can exercise it</span></div></li>
            <li className="kpi"><div className="kpi__link"><span className="kpi__label">Trained</span><strong className="kpi__value">{m.exercised}</strong><span className="kpi__note">run successfully for real</span></div></li>
            <li className="kpi"><div className="kpi__link"><span className="kpi__label">Simulated</span><strong className="kpi__value">{m.simulated ?? 0}</strong><span className="kpi__note">demo runs or references</span></div></li>
          </ul>
          <div className="toolbar">
            <label className="field field--inline"><span className="field__label">Search</span><input type="search" value={search} placeholder="T1046 or discovery" onChange={(e) => setSearch(e.target.value)} /></label>
            <label className="field field--inline"><span className="field__label">Platform</span>
              <select value={platform} onChange={(e) => setPlatform(e.target.value)}><option value="">All platforms</option>{(m.platforms ?? []).map((pl) => <option key={pl} value={pl}>{pl}</option>)}</select></label>
            <label className="check"><input type="checkbox" checked={only} onChange={(e) => setOnly(e.target.checked)} /><span className="check__box" aria-hidden="true" />Only mapped techniques</label>
            {m.last_run?.status === 'succeeded' && <span className="muted small">ATT&amp;CK {m.last_run.source_version ? `v${m.last_run.source_version} · ` : ''}{m.last_run.tactics ?? m.tactics.length} tactics · imported {when(m.last_run.finished_at)}</span>}
          </div>
          <details className="panel legend-help">
            <summary>What do the colours mean?</summary>
            <p className="small"><i className="mcell--exercised" /> <strong>Trained</strong>: an action mapped to this technique ran successfully against a real target. <i className="mcell--mapped" /> <strong>Mapped</strong>: an action can perform this behaviour. <i className="mcell--simulated" /> <strong>Simulated</strong>: demo runs or finding references; synthetic evidence never counts as trained. A Demo badge remains visible when a technique is also mapped or trained. These counts can overlap. <i className="mcell--idle" /> <strong>Not covered</strong>: no action or execution evidence. A mapping means the action performs that behaviour, not that it defends against it.</p>
          </details>
          <div className="workspace matrix-workspace">
            <div ref={canvasRef} className="canvas matrix" role="region" aria-label="ATT&CK matrix" tabIndex={0}>
              <div className="matrix__cols">
                {m.tactics.map((t) => {
                  const cells = t.techniques.filter((c) => (!only || c.mapped) && matches(c, term, platform));
                  return (
                    <section key={t.id} className="mcol" aria-label={t.name}>
                      <h2 className="mcol__title">{t.name}<small>{cells.length}</small></h2>
                      {cells.length === 0 ? <p className="muted small">None</p> : cells.map((c) => <Cell key={c.id} c={c} selected={picked === c.id} onPick={(id, button) => { originRef.current = button; setPicked(id); }} />)}
                    </section>
                  );
                })}
              </div>
            </div>
            {picked ? <Detail key={picked} id={picked} onClose={closeDetails} onPick={setPicked} /> : <section className="panel inspector"><p className="muted">Select a technique to see what covers it.</p></section>}
          </div>
        </>
      )}
    </section>
  );
}

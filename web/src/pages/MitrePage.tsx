import { t as textUI, tr as richUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { ExternalLink, X } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { useMitreMatrix, useMitreTechnique } from '../api/hooks';
import type { MitreCell } from '../api/types';
import { describeError } from '../components/ErrorNote';
import { when } from '../components/RunBadges';
const MITIGATIONS_SHOWN = 6;
function Detail({ id, onClose, onPick }: {
    id: string;
    onClose: () => void;
    onPick: (id: string) => void;
}) {
    useLocale();
    const panelRef = useRef<HTMLElement>(null);
    const q = useMitreTechnique(id);
    const [allMitigations, setAllMitigations] = useState(false);
    const d = q.data;
    const mappings = d?.mappings ?? [];
    const mitigations = d?.mitigations ?? [];
    const shown = allMitigations ? mitigations : mitigations.slice(0, MITIGATIONS_SHOWN);
    useEffect(() => {
        const panel = panelRef.current;
        if (!panel)
            return;
        const reveal = () => panel.scrollIntoView?.({ block: 'nearest' });
        panel.focus({ preventScroll: true });
        reveal();
        window.addEventListener('resize', reveal);
        return () => window.removeEventListener('resize', reveal);
    }, [id]);
    return (<section ref={panelRef} className="panel inspector" aria-label={textUI("m_743a0efcd070")} aria-describedby="mitre-detail-heading" tabIndex={-1} onKeyDown={(event) => {
            if (event.key === 'Escape') {
                event.preventDefault();
                onClose();
            }
        }}>
      <div className="posture__head"><h2 id="mitre-detail-heading" className="panel__heading">{formatUI(d ? `${d.id} · ${d.name}` : id)}</h2><button type="button" className="icon-btn" aria-label={textUI("m_edc82d69e76d")} onClick={onClose}><X size={16}/></button></div>
      {formatUI(q.isPending ? <span className="skeleton"/> : q.isError || !d ? <p role="alert">{formatUI(describeError(q.error).message)}</p> : (<>
          <p className="muted small">{formatUI(d.tactics.join(' · '))}{formatUI(d.platforms.length ? ` · ${d.platforms.join(', ')}` : '')}</p>
          {formatUI(d.deprecated && <p className="muted small">{textUI("m_9140bd146239")}</p>)}
          {formatUI(d.parent && <p className="muted small">{richUI("m_79ff3272bdd1", [<button type="button" className="link-subtle" onClick={() => onPick(d.parent!)}>{formatUI(d.parent)}</button>])}</p>)}
          <div className="desc" role="region" aria-label={textUI("m_8c64fc6a5e77")} tabIndex={0}>{formatUI(d.description || textUI("m_06a694820920"))}</div>
          <dl className="kv kv--2">
            <div><dt>{textUI("m_cad9bf74e315")}</dt><dd>{formatUI(d.runs > 0 ? textUI("m_32df551181d5", [d.runs, d.runs === 1 ? textUI("m_acba25512100") : textUI("m_1f64fff08d78")]) : textUI("m_a9549d7d0636"))}</dd></div>
            <div><dt>{textUI("m_512a48218ba2")}</dt><dd>{formatUI(when(d.last_run_at))}</dd></div>
          </dl>
          {formatUI(d.simulated && <p className="muted small">{richUI("m_4d974e675958", [<strong>{textUI("m_31f9a349c636")}</strong>, formatUI(d.simulated_runs ?? 0), formatUI(d.simulated_findings ?? 0)])}</p>)}
          <h3 className="panel__sub">{textUI("m_0d73a8de7893")}</h3>
          {formatUI(mappings.length === 0 && d.actions.length > 0 ? <p className="muted small">{richUI("m_9a23155e6576", [formatUI(d.actions.join(', '))])}</p> : mappings.length === 0 ? <p className="muted small">{textUI("m_2ecf9c965c55")}</p> : (<ul className="stack-list">
              {formatUI(mappings.map((mp) => (<li key={`${mp.action}-${mp.via ?? ''}`}><strong>{formatUI(mp.action)}</strong>{formatUI(mp.via ? textUI("m_1528ae061aaf", [mp.via]) : '')} <span className="muted small">{formatUI(mp.basis === 'direct' ? 'direct' : 'estimate')}</span><br /><span className="muted small">{formatUI(mp.reason)}</span></li>)))}
            </ul>))}
          {formatUI(d.subs.length > 0 && (<>
              <h3 className="panel__sub">{richUI("m_66f2129f3eb8", [formatUI(d.subs.length)])}</h3>
              <ul className="stack-list">{formatUI(d.subs.map((sc) => <li key={sc.id}><button type="button" className="link-subtle" onClick={() => onPick(sc.id)}>{formatUI(sc.id)}</button> {formatUI(sc.name)}</li>))}</ul>
            </>))}
          {formatUI(mitigations.length > 0 && (<>
              <h3 className="panel__sub">{richUI("m_8d02cd8e1dc8", [formatUI(mitigations.length)])}</h3>
              <ul className="stack-list">{formatUI(shown.map((mt) => <li key={mt.id}><strong>{formatUI(mt.id)}</strong> {formatUI(mt.name)}{formatUI(mt.description ? <><br /><span className="muted small">{formatUI(mt.description)}</span></> : null)}</li>))}</ul>
              {formatUI(mitigations.length > MITIGATIONS_SHOWN && <button type="button" className="btn btn--secondary btn--sm" onClick={() => setAllMitigations(!allMitigations)}>{formatUI(allMitigations ? textUI("m_255beb6510e7") : textUI("m_6bf0e611f8d7", [mitigations.length]))}</button>)}
            </>))}
          <a className="btn btn--secondary btn--sm" href={d.url} target="_blank" rel="noopener noreferrer">{richUI("m_86c5c4295527", [<ExternalLink size={13} aria-hidden="true"/>])}</a>
        </>))}
    </section>);
}
// Warna mengutamakan bukti nyata; badge demo tetap terlihat pada status yang beririsan.
function cellState(c: MitreCell): 'exercised' | 'mapped' | 'simulated' | 'idle' {
    return c.exercised ? 'exercised' : c.mapped ? 'mapped' : c.simulated ? 'simulated' : 'idle';
}
const STATE_TEXT = { exercised: 'trained: an action was run successfully', mapped: 'mapped: an action can exercise it, not yet run', simulated: 'simulated: synthetic demo evidence', get idle() {
        return textUI("m_0635717249e5");
    } } as const;
function Cell({ c, selected, onPick }: {
    c: MitreCell;
    selected: boolean;
    onPick: (id: string, button: HTMLButtonElement) => void;
}) {
    useLocale();
    const state = cellState(c);
    return (<button type="button" className={`mcell mcell--${state} ${selected ? 'is-on' : ''}`} aria-label={`${c.id} ${c.name}: ${STATE_TEXT[state]}${c.simulated && state !== 'simulated' ? '; simulated: demo evidence also present' : ''}`} aria-pressed={selected} onClick={(event) => onPick(c.id, event.currentTarget)}>
      <span className="mcell__meta"><span className="mcell__id">{formatUI(c.id)}</span>{formatUI(c.simulated && <span className="mcell__demo">{textUI("m_8a2cc0673b1c")}</span>)}</span><span className="mcell__name">{formatUI(c.name)}</span>
    </button>);
}
function matches(c: MitreCell, term: string, platform: string): boolean {
    const own = (x: MitreCell) => (!term || x.id.toLowerCase().includes(term) || x.name.toLowerCase().includes(term)) && (!platform || (x.platforms ?? []).includes(platform));
    return own(c) || (c.subs ?? []).some(own);
}
export function MitrePage() {
    useLocale();
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
    return (<section className="page" aria-labelledby="mitre-title">
      <div className="page__head"><div>
        <h1 id="mitre-title" className="page__title">{textUI("m_0259f2cc705f")}</h1>
        <p className="page__sub">{textUI("m_8f6d14646b40")}</p></div></div>
      {formatUI(q.isPending ? <span className="skeleton"/> : q.isError || !m ? (<div className="state state--error" role="alert"><h2>{textUI("m_7bc63bc2ad7e")}</h2><p>{formatUI(describeError(q.error).message)}</p><button type="button" className="btn btn--secondary btn--auto" onClick={() => q.refetch()}>{textUI("m_d8b8392e2c54")}</button></div>) : !m.imported ? (<div className="state"><h2>{formatUI(m.last_run?.status === 'running' ? textUI("m_a54e4002d116") : textUI("m_2dc266764d6e"))}</h2>
          <p>{formatUI(m.last_run?.status === 'running' ? textUI("m_63c0f3f49cbd") : m.last_run?.status === 'failed' ? textUI("m_605fc4d30c12", [m.last_run.error ?? textUI("m_3e4443e52275")]) : textUI("m_16dd37bc3844"))}</p></div>) : (<>
          <ul className="kpis kpis--4" aria-label={textUI("m_523487a5de21")}>
            <li className="kpi"><div className="kpi__link"><span className="kpi__label">{textUI("m_52551d32e69a")}</span><strong className="kpi__value">{formatUI(m.total)}</strong><span className="kpi__note">{textUI("m_b98ed300aaea")}</span></div></li>
            <li className="kpi"><div className="kpi__link"><span className="kpi__label">{textUI("m_879a696a1944")}</span><strong className="kpi__value">{formatUI(m.mapped)}</strong><span className="kpi__note">{textUI("m_727f504074ba")}</span></div></li>
            <li className="kpi"><div className="kpi__link"><span className="kpi__label">{textUI("m_cad9bf74e315")}</span><strong className="kpi__value">{formatUI(m.exercised)}</strong><span className="kpi__note">{textUI("m_8be03f909536")}</span></div></li>
            <li className="kpi"><div className="kpi__link"><span className="kpi__label">{textUI("m_31fe293b1b90")}</span><strong className="kpi__value">{formatUI(m.simulated ?? 0)}</strong><span className="kpi__note">{textUI("m_9c307cfd7488")}</span></div></li>
          </ul>
          <div className="toolbar">
            <label className="field field--inline"><span className="field__label">{textUI("m_49c266baaaa7")}</span><input type="search" value={search} placeholder={textUI("m_2c5abecc0e64")} onChange={(e) => setSearch(e.target.value)}/></label>
            <label className="field field--inline"><span className="field__label">{textUI("m_c78ffe195710")}</span>
              <select value={platform} onChange={(e) => setPlatform(e.target.value)}><option value="">{textUI("m_6d46bb346f19")}</option>{formatUI((m.platforms ?? []).map((pl) => <option key={pl} value={pl}>{formatUI(pl)}</option>))}</select></label>
            <label className="check">{richUI("m_4fcbeb096998", [<input type="checkbox" checked={only} onChange={(e) => setOnly(e.target.checked)}/>, <span className="check__box" aria-hidden="true"/>])}</label>
            {formatUI(m.last_run?.status === 'succeeded' && <span className="muted small">{richUI("m_792da1a18dd3", [formatUI(m.last_run.source_version ? `v${m.last_run.source_version} · ` : ''), formatUI(m.last_run.tactics ?? m.tactics.length), formatUI(when(m.last_run.finished_at))])}</span>)}
          </div>
          <details className="panel legend-help">
            <summary>{textUI("m_0d4a53b2a8c8")}</summary>
            <p className="small">{richUI("m_ab5dbe73388f", [<i className="mcell--exercised"/>, <strong>{textUI("m_cad9bf74e315")}</strong>, <i className="mcell--mapped"/>, <strong>{textUI("m_879a696a1944")}</strong>, <i className="mcell--simulated"/>, <strong>{textUI("m_31fe293b1b90")}</strong>, <i className="mcell--idle"/>, <strong>{textUI("m_06baa5b3fa1b")}</strong>])}</p>
          </details>
          <div className="workspace matrix-workspace">
            <div ref={canvasRef} className="canvas matrix" role="region" aria-label={textUI("m_52b7b2901f43")} tabIndex={0}>
              <div className="matrix__cols">
                {formatUI(m.tactics.map((t) => {
                const cells = t.techniques.filter((c) => (!only || c.mapped) && matches(c, term, platform));
                return (<section key={t.id} className="mcol" aria-label={t.name}>
                      <h2 className="mcol__title">{formatUI(t.name)}<small>{formatUI(cells.length)}</small></h2>
                      {formatUI(cells.length === 0 ? <p className="muted small">{textUI("m_dc937b598926")}</p> : cells.map((c) => <Cell key={c.id} c={c} selected={picked === c.id} onPick={(id, button) => { originRef.current = button; setPicked(id); }}/>))}
                    </section>);
            }))}
              </div>
            </div>
            {formatUI(picked ? <Detail key={picked} id={picked} onClose={closeDetails} onPick={setPicked}/> : <section className="panel inspector"><p className="muted">{textUI("m_5cd80faf603c")}</p></section>)}
          </div>
        </>))}
    </section>);
}

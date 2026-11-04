import { t as textUI, tr as richUI, useLocale, locale, fv as formatUI } from "../i18n/runtime";
import { EdgeRuleEditor } from '../components/EdgeRuleEditor';
import { SchemaFields } from '../components/SchemaFields';
import { ChevronRight, Plus, Trash2 } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { ApiError } from '../api/client';
import { useContracts, useMe, useSaveGraph, useScenario } from '../api/hooks';
import { can, type ChainEdge, type ChainStep, type ScenarioDetail } from '../api/types';
import { describeError } from '../components/ErrorNote';
import { CARD_H, CARD_W, layoutChain, nextKey, wouldCycle } from '../lib/chainLayout';
const SEVERITIES = ['info', 'low', 'medium', 'high', 'critical'];
const FINDING_TYPES = () => [['open_port', textUI("m_5c9e0324afba")], ['service', textUI("m_d677190e0a99")], ['vulnerability', textUI("m_a123dd728a7a")]] as const;
function StepParams({ step, onChange, disabled }: {
    step: ChainStep;
    onChange: (p: Record<string, unknown>) => void;
    disabled: boolean;
}) {
    useLocale();
    const contracts = useContracts();
    const def = contracts.data?.find(c => c.key === step.contract_key);
    return <SchemaFields schema={def?.input_schema} value={step.params as Record<string, unknown>} onChange={onChange} disabled={disabled}/>;
}
function describeTrigger(s: ChainStep): string {
    if (s.trigger_mode !== 'on_finding' || !s.trigger)
        return textUI("m_aa657b3aeb0b");
    const vals = s.trigger.values?.length ? ` ${s.trigger.values.join(', ')}` : '';
    return textUI("m_321a77708c8e", [s.trigger.type.replace('_', ' '), vals]);
}
export function ChainWorkspacePage() {
    useLocale();
    const { id = '' } = useParams();
    const { data: me } = useMe();
    const q = useScenario(id);
    const contracts = useContracts();
    const save = useSaveGraph(id);
    const editable = can(me, 'scenario.write');
    const [steps, setSteps] = useState<ChainStep[] | null>(null);
    const [edges, setEdges] = useState<ChainEdge[]>([]);
    const [selected, setSelected] = useState<string | null>(null);
    const [base, setBase] = useState<ScenarioDetail | null>(null);
    useEffect(() => {
        if (q.data && (steps === null || (base && q.data.version !== base.version && !dirty(base, steps, edges)))) {
            setBase(q.data);
            setSteps(q.data.steps);
            setEdges(q.data.edges);
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [q.data]);
    const placed = useMemo(() => layoutChain((steps ?? []).map((s) => s.key), edges), [steps, edges, locale()]);
    const pos = new Map(placed.map((p) => [p.key, p]));
    const width = Math.max(1, ...placed.map((p) => p.x + CARD_W)) + 16;
    const height = Math.max(1, ...placed.map((p) => p.y + CARD_H)) + 16;
    if (q.isPending || steps === null)
        return <section className="page"><span className="skeleton"/></section>;
    if (q.isError || !q.data || !base) {
        return <section className="page"><div className="state state--error" role="alert"><h2>{textUI("m_21c45024120c")}</h2><p>{formatUI(describeError(q.error).message)}</p><Link className="btn btn--secondary btn--auto" to="/chains">{textUI("m_0b1e48affc33")}</Link></div></section>;
    }
    const isDirty = dirty(base, steps, edges);
    const sel = steps.find((s) => s.key === selected) ?? null;
    const patch = (key: string, change: Partial<ChainStep>) => setSteps(steps.map((s) => (s.key === key ? { ...s, ...change } : s)));
    const parentsOf = (key: string) => edges.filter((e) => e.child === key).map((e) => e.parent);
    const add = () => {
        const key = nextKey(steps.map((s) => s.key));
        setSteps([...steps, { key, title: textUI("m_79b92f59b828"), description: '', contract_key: contracts.data?.find((c) => c.key === 'nmap.tcp_connect')?.key ?? contracts.data?.find((c) => !c.needs_agent)?.key ?? 'nmap.tcp_connect', params: {}, delay_seconds: 0, trigger_mode: 'after_parents', trigger: null, is_enabled: true }]);
        setSelected(key);
    };
    const remove = (key: string) => { setSteps(steps.filter((s) => s.key !== key)); setEdges(edges.filter((e) => e.parent !== key && e.child !== key)); setSelected(null); };
    const toggleParent = (child: string, parent: string) => {
        const has = edges.some((e) => e.parent === parent && e.child === child);
        setEdges(has ? edges.filter((e) => !(e.parent === parent && e.child === child)) : [...edges, { parent, child }]);
    };
    const discard = () => { setSteps(base.steps); setEdges(base.edges); setSelected(null); save.reset(); };
    const doSave = () => save.mutate({ version: base.version, steps, edges }, { onSuccess: (d) => { setBase(d); setSteps(d.steps); setEdges(d.edges); } });
    const err = save.error;
    const problems = err instanceof ApiError && err.status === 422 ? err.fields : [];
    return (<section className="page" aria-labelledby="chain-title">
      <div className="page__head">
        <div>
          <nav className="crumbs" aria-label={textUI("m_2bd873d6c734")}><Link to="/chains">{textUI("m_ccd8711a2343")}</Link><ChevronRight size={12} aria-hidden="true"/><span className="crumbs__current">{formatUI(base.name)}</span></nav>
          <div className="page__title-row"><h1 id="chain-title" className="page__title">{formatUI(base.name)}</h1><span className="badge">{formatUI(steps.length)} {formatUI(steps.length === 1 ? 'action' : 'actions')}</span>{formatUI(isDirty && <span className="status status--live" role="status">{textUI("m_a710c2b90913")}</span>)}</div>
          <p className="page__sub">{richUI("m_87f30914bc8d", [<strong>{textUI("m_5ef5ef0364b6")}</strong>])}</p>
        </div>
        <div className="page__actions">
          <Link className="btn btn--secondary btn--auto" to={`/chains/${base.id}/setup`}>{textUI("m_2fcb5d374697")}</Link>
          {formatUI(editable && <>
            <button type="button" className="btn btn--secondary btn--auto" onClick={add}>{richUI("m_d179b0e6d989", [<Plus size={15} aria-hidden="true"/>])}</button>
            <button type="button" className="btn btn--secondary btn--auto" onClick={discard} disabled={!isDirty}>{textUI("m_eb1a70e39274")}</button>
            <button type="button" className="btn btn--primary btn--auto" onClick={doSave} disabled={!isDirty || save.isPending}>{formatUI(save.isPending ? textUI("m_23e39291d613") : textUI("m_02874919aae9"))}</button>
          </>)}
        </div>
      </div>

      {formatUI(err && problems.length === 0 && <div className="alert" role="alert"><strong>{formatUI(err instanceof ApiError && err.status === 409 ? textUI("m_bd280c9a5a85") : textUI("m_a018b7a8a188"))}</strong><span>{formatUI(describeError(err).message)}{formatUI(err instanceof ApiError && err.status === 409 ? ' Reload the page to see their version.' : '')}</span></div>)}
      {formatUI(problems.length > 0 && (<div className="alert" role="alert"><strong>{textUI("m_7e9555fee323")}</strong>
          <ul className="problems">{formatUI(problems.map((f, i) => <li key={i}>{formatUI(f.message)}</li>))}</ul></div>))}

      <div className="workspace">
        <div className="canvas" role="region" aria-label={textUI("m_816e125e3154")} tabIndex={0}>
          {formatUI(steps.length === 0 ? (<div className="state"><h2>{textUI("m_13038913bd3e")}</h2><p>{textUI("m_68a8488bcc8c")}</p>
              {formatUI(editable && <button type="button" className="btn btn--primary btn--auto" onClick={add}>{textUI("m_13643f9151cd")}</button>)}</div>) : (<div className="canvas__inner" style={{ width, height }}>
              <svg className="canvas__edges" width={width} height={height} aria-hidden="true">
                {formatUI(edges.map((e) => {
                const a = pos.get(e.parent), b = pos.get(e.child);
                if (!a || !b)
                    return null;
                const x1 = a.x + CARD_W, y1 = a.y + CARD_H / 2, x2 = b.x, y2 = b.y + CARD_H / 2, mx = (x1 + x2) / 2;
                return <path key={e.parent + e.child} d={`M${x1},${y1} C${mx},${y1} ${mx},${y2} ${x2},${y2}`} className="canvas__edge"/>;
            }))}
              </svg>
              {formatUI(steps.map((s) => {
                const p = pos.get(s.key)!;
                return (<button key={s.key} type="button" className={`stepcard ${selected === s.key ? 'is-on' : ''} ${s.is_enabled ? '' : 'is-off'}`} style={{ left: p.x, top: p.y, width: CARD_W, height: CARD_H }} aria-pressed={selected === s.key} aria-label={textUI("m_48d7db9b2b65", [s.title])} onClick={() => setSelected(s.key)}>
                    <strong>{formatUI(s.title)}</strong>
                    <span className="muted small">{formatUI(contracts.data?.find((c) => c.key === s.contract_key)?.name ?? s.contract_key)}</span>
                    <span className="stepcard__trigger">{formatUI(describeTrigger(s))}{formatUI(s.delay_seconds > 0 ? textUI("m_84c22471d8b0", [s.delay_seconds]) : '')}</span>
                  </button>);
            }))}
            </div>))}
        </div>

        <section className="panel inspector" aria-label={textUI("m_03dcaf20ac64")}>
          {formatUI(!sel ? <p className="muted">{textUI("m_61f48e5d1faf")}</p> : (<>
              <div className="posture__head"><h2 className="panel__heading">{textUI("m_03dcaf20ac64")}</h2>
                {formatUI(editable && <button type="button" className="btn btn--danger btn--sm" onClick={() => remove(sel.key)}>{richUI("m_86391c8ffc72", [<Trash2 size={14} aria-hidden="true"/>])}</button>)}</div>
              <div className="field"><div className="field__label-row"><label htmlFor="st-title" className="field__label">{textUI("m_7e8cd2056da7")}</label></div>
                <div className="field__input"><input id="st-title" disabled={!editable} value={sel.title} onChange={(e) => patch(sel.key, { title: e.target.value })}/></div></div>
              <div className="select"><label htmlFor="st-contract">{textUI("m_64cff1319d2f")}</label>
                <select id="st-contract" disabled={!editable} value={sel.contract_key} onChange={(e) => patch(sel.key, { contract_key: e.target.value, params: {} })}>
                  {formatUI((contracts.data ?? []).map((c) => <option key={c.key} value={c.key}>{formatUI(c.name)}</option>))}
                </select></div>
              <StepParams step={sel} disabled={!editable} onChange={(params) => patch(sel.key, { params })}/>

              <div className="select"><label htmlFor="st-mode">{textUI("m_96dbedeca7df")}</label>
                <select id="st-mode" disabled={!editable} value={sel.trigger_mode} onChange={(e) => patch(sel.key, e.target.value === 'on_finding'
                ? { trigger_mode: 'on_finding', trigger: sel.trigger ?? { type: 'open_port', values: [] } } : { trigger_mode: 'after_parents', trigger: null })}>
                  <option value="after_parents">{textUI("m_8308aa3898b6")}</option><option value="on_finding">{textUI("m_c536b148d329")}</option>
                </select></div>
              {formatUI(sel.trigger_mode === 'on_finding' && sel.trigger && (<>
                  <div className="select"><label htmlFor="st-ttype">{textUI("m_34221770fd07")}</label>
                    <select id="st-ttype" disabled={!editable} value={sel.trigger.type} onChange={(e) => patch(sel.key, { trigger: { ...sel.trigger!, type: e.target.value } })}>
                      {formatUI(FINDING_TYPES().map(([v, l]) => <option key={v} value={v}>{formatUI(l)}</option>))}</select></div>
                  <div className="field"><div className="field__label-row"><label htmlFor="st-tvals" className="field__label">{textUI("m_1a67f1bad663")}</label></div>
                    <div className="field__input"><input id="st-tvals" disabled={!editable} value={(sel.trigger.values ?? []).join(', ')} placeholder={textUI("m_b7935769c956")} onChange={(e) => patch(sel.key, { trigger: { ...sel.trigger!, values: e.target.value.split(',').map((v) => v.trim()).filter(Boolean) } })}/></div></div>
                  <div className="select"><label htmlFor="st-tsev">{textUI("m_4f0b011effef")}</label>
                    <select id="st-tsev" disabled={!editable} value={sel.trigger.min_severity ?? ''} onChange={(e) => patch(sel.key, { trigger: { ...sel.trigger!, min_severity: e.target.value || undefined } })}>
                      <option value="">{textUI("m_2b505597daa7")}</option>{formatUI(SEVERITIES.map((s) => <option key={s} value={s}>{formatUI(s[0]!.toUpperCase() + s.slice(1))}</option>))}</select></div>
                </>))}
              <div className="field"><div className="field__label-row"><label htmlFor="st-delay" className="field__label">{textUI("m_030777c817bc")}</label></div>
                <div className="field__input"><input id="st-delay" type="number" min={0} disabled={!editable} value={sel.delay_seconds} onChange={(e) => patch(sel.key, { delay_seconds: Math.max(0, Number(e.target.value) || 0) })}/></div></div>
              <label className="check">{richUI("m_5a87fbc817ab", [<input type="checkbox" disabled={!editable} checked={sel.is_enabled} onChange={(e) => patch(sel.key, { is_enabled: e.target.checked })}/>, <span className="check__box" aria-hidden="true"/>])}</label>

              <fieldset className="fieldset"><legend>{textUI("m_a2257a1d883d")}</legend>
                {formatUI(steps.filter((s) => s.key !== sel.key).length === 0 ? <p className="muted small">{textUI("m_b79a758b6359")}</p> : (<div className="checklist">{formatUI(steps.filter((s) => s.key !== sel.key).map((s) => {
                    const checked = parentsOf(sel.key).includes(s.key);
                    const blocked = !checked && wouldCycle(edges, s.key, sel.key);
                    return (<label key={s.key} className="check"><input type="checkbox" disabled={!editable || blocked} checked={checked} onChange={() => toggleParent(sel.key, s.key)}/><span className="check__box" aria-hidden="true"/>
                        <span>{formatUI(s.title)}{formatUI(blocked && <small className="check__sub">{textUI("m_edc363b01d92")}</small>)}</span></label>);
                }))}</div>))}
              </fieldset>
              {formatUI(edges.filter(e => e.child === sel.key).map(edge => <EdgeRuleEditor key={edge.parent + edge.child} edge={edge} disabled={!editable} onChange={v => setEdges(edges.map(e => e.parent === v.parent && e.child === v.child ? v : e))}/>))}
            </>))}
        </section>
      </div>
    </section>);
}
function dirty(base: ScenarioDetail, steps: ChainStep[], edges: ChainEdge[]): boolean {
    const norm = (s: ChainStep[], e: ChainEdge[]) => JSON.stringify({ s, e: [...e].sort((a, b) => (a.parent + a.child).localeCompare(b.parent + b.child)) });
    return norm(base.steps, base.edges) !== norm(steps, edges);
}

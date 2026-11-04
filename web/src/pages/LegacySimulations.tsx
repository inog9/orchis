import { t as textUI, tr as richUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { Link } from 'react-router-dom';
import { useState } from 'react';
import { useSimulations } from '../api/hooks';
import { describeError } from '../components/ErrorNote';
import { SimBadge, elapsed } from '../components/StatusPills';
import { when } from '../components/RunBadges';
const FILTERS = () => [['', textUI("m_a52ace420f21")], ['draft', textUI("m_ebf12ef47cf5")], ['scheduled', textUI("m_4724f344c1c0")], ['running', textUI("m_f4ccae29e1bb")], ['paused', textUI("m_e159b06187d3")], ['finished', textUI("m_7804f7a79a9e")], ['canceled', textUI("m_13ca2ee24993")], ['failed', textUI("m_031a8f0f659d")]] as const;
export function LegacySimulations() {
    useLocale();
    const [status, setStatus] = useState('');
    const q = useSimulations(status);
    const items = q.data ?? [];
    return (<section className="page" aria-labelledby="sims-title">
      <div className="page__head">
        <div>
          <div className="page__title-row"><h1 id="sims-title" className="page__title">{textUI("m_078a864ea0f1")}</h1>{formatUI(q.data && <span className="badge">{formatUI(items.length)}</span>)}</div>
          <p className="page__sub">{textUI("m_ea170ee5640c")}</p>
        </div>
      </div>
      <div className="chips" role="group" aria-label={textUI("m_94d5a7703a82")}>
        {formatUI(FILTERS().map(([v, l]) => <button key={v} type="button" className={`chip ${status === v ? 'is-on' : ''}`} aria-pressed={status === v} onClick={() => setStatus(v)}>{formatUI(l)}</button>))}
      </div>
      {formatUI(q.isPending ? <span className="skeleton"/> : q.isError ? (<div className="state state--error" role="alert"><h2>{textUI("m_ec3e81d83626")}</h2><p>{richUI("m_19636dc199da", [formatUI(describeError(q.error).message)])}</p>
          <button type="button" className="btn btn--secondary btn--auto" onClick={() => q.refetch()}>{textUI("m_d8b8392e2c54")}</button></div>) : items.length === 0 ? (<div className="state"><h2>{formatUI(status ? textUI("m_396a0d883847") : textUI("m_24c6b9ffc418"))}</h2>
          <p>{formatUI(status ? textUI("m_c9379e975e66") : textUI("m_af7137afb0e7"))}</p>
          {formatUI(!status && <Link className="btn btn--secondary btn--auto" to="/organizations">{textUI("m_d78c44ecc1be")}</Link>)}</div>) : (<div className="list" role="table" aria-label={textUI("m_078a864ea0f1")} tabIndex={0}>
          <div className="list__head sim-row" role="row"><span role="columnheader">{textUI("m_5cccd402752d")}</span><span role="columnheader">{textUI("m_8c2e4a035f5f")}</span><span role="columnheader">{textUI("m_e0d0adc74c8b")}</span><span role="columnheader">{textUI("m_a628d0e473bf")}</span><span role="columnheader">{textUI("m_4d010bf1455a")}</span></div>
          {formatUI(items.map((s) => (<div key={s.id} className="asset-row sim-row" role="row">
              <span role="cell" className="asset-row__text"><Link className="asset-row__name" to={`/simulations/legacy/${s.id}`}>{formatUI(s.name)}</Link><span className="muted small">{formatUI(s.is_demo && textUI("m_37bf396b59ee"))}{formatUI(s.code)} · {formatUI(s.scenario_name ?? textUI("m_643f23b836de"))}</span></span>
              <span role="cell"><SimBadge status={s.status}/></span>
              <span role="cell">{richUI("m_1c39c81ac615", [formatUI(s.steps_done), formatUI(s.step_count)])}</span>
              <span role="cell">{formatUI(s.finding_count)}</span>
              <span role="cell" className="muted small">{formatUI(when(s.started_at))}{formatUI(s.started_at ? ` · ${elapsed(s.started_at, s.ended_at)}` : '')}</span>
            </div>)))}
        </div>))}
    </section>);
}

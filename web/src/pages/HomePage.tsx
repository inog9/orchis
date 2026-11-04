import { t as textUI, tr as richUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { OrganisationSetup } from '../components/OrganisationSetup';
import { Link } from 'react-router-dom';
import { useHome, useMe } from '../api/hooks';
import { can, type HomeSummary } from '../api/types';
import { describeError } from '../components/ErrorNote';
import { RunBadge, when } from '../components/RunBadges';
import { SimBadge } from '../components/StatusPills';
const SEVERITIES = ['critical', 'high', 'medium', 'low', 'info'] as const;
// Tingkat paparan diturunkan dari kerentanan yang ditemukan; bukan skor resmi, hanya ringkasan cepat.
export function exposureLevel(by: Record<string, number>, total: number): {
    label: string;
    tone: string;
} {
    if (total === 0)
        return { label: textUI("m_e2e18a767a2b"), tone: 'none' };
    if ((by.critical ?? 0) > 0)
        return { label: textUI("m_5e12b9290669"), tone: 'critical' };
    if ((by.high ?? 0) > 0)
        return { label: textUI("m_149a3ee3b92f"), tone: 'high' };
    if ((by.medium ?? 0) > 0)
        return { label: textUI("m_1894e0b14dfa"), tone: 'medium' };
    return { label: textUI("m_3d749c9d4d6b"), tone: 'low' };
}
function Kpi({ label, value, note, to }: {
    label: string;
    value: number;
    note?: string;
    to?: string;
}) {
    useLocale();
    const body = (<>
      <span className="kpi__label">{formatUI(label)}</span>
      <strong className="kpi__value">{formatUI(value)}</strong>
      <span className="kpi__note">{formatUI(note ?? '\u00a0')}</span>
    </>);
    return <li className="kpi">{formatUI(to ? <Link className="kpi__link" to={to}>{formatUI(body)}</Link> : <div className="kpi__link">{formatUI(body)}</div>)}</li>;
}
export function HomePage() {
    useLocale();
    const { data: me } = useMe();
    const q = useHome();
    const h = q.data;
    return (<section className="page" aria-labelledby="home-title">
      <div className="page__head">
        <div>
          <h1 id="home-title" className="page__title">{textUI("m_3a78695388b3")}</h1>
          <p className="page__sub">{richUI("m_584ab53ab844", [formatUI(me ? `, ${me.display_name}` : '')])}</p>
        </div>
      </div>
      {formatUI(q.isPending ? (<div aria-label={textUI("m_5ef130c39669")}><ul className="kpis">{formatUI([0, 1, 2, 3].map((i) => <li key={i} className="kpi"><span className="skeleton"/></li>))}</ul></div>) : q.isError || !h ? (<div className="state state--error" role="alert">
          <h2>{textUI("m_b96e0c63b366")}</h2><p>{richUI("m_19636dc199da", [formatUI(describeError(q.error).message)])}</p>
          <button type="button" className="btn btn--secondary btn--auto" onClick={() => q.refetch()}>{textUI("m_d8b8392e2c54")}</button>
        </div>) : (<HomeBody h={h}/>))}
    </section>);
}
function HomeBody({ h }: {
    h: HomeSummary;
}) {
    useLocale();
    const { data: me } = useMe();
    const offline = h.injectors.filter((i) => i.status !== 'online');
    const lvl = h.findings ? exposureLevel(h.findings.by_severity, h.findings.vulnerabilities) : null;
    const sevMax = h.findings ? Math.max(1, ...SEVERITIES.map((s) => h.findings!.by_severity[s] ?? 0)) : 1;
    const starting = h.organizations?.length === 0 && (h.assets?.total ?? 0) === 0 && (h.findings?.total ?? 0) === 0 && (h.atomic?.tests ?? 0) === 0 && (h.chains?.scenarios ?? 0) === 0;
    if (starting)
        return <>
    {formatUI(can(me, 'organization.read') && <OrganisationSetup />)}
    <section className="panel" aria-labelledby="first-results-title"><h2 id="first-results-title" className="panel__heading">{textUI("m_d822a6b52b11")}</h2><p className="muted">{textUI("m_1de16038df84")}</p><p className="muted">{richUI("m_ff02cdc06eab", [formatUI(h.injectors.filter(i => i.status === 'online').length), formatUI(h.injectors.length)])}</p>{formatUI(offline.length > 0 && <p>{richUI("m_8a1c28fa6db9", [formatUI(offline.map(i => i.name).join(' and ')), formatUI(offline.length === 1 ? textUI("m_8a27f2fcccaa") : textUI("m_ba5bff2b4cdf"))])}</p>)}{formatUI(can(me, 'integration.read') && <Link to="/settings/integrations">{textUI("m_8ac30ac6ae4d")}</Link>)}</section>
  </>;
    return (<>
      {formatUI(can(me, 'organization.read') && <OrganisationSetup />)}
      {formatUI(h.organizations && <section aria-labelledby="organisation-overview-title" className="org-overview"><div className="page__head"><div><p className="eyebrow">{textUI("m_53000c389d4d")}</p><h2 id="organisation-overview-title">{textUI("m_0a154b9350d3")}</h2><p className="muted">{textUI("m_14f942a625e7")}</p></div><Link className="btn btn--secondary btn--auto" to="/organizations">{formatUI(can(me, 'organization.write') ? textUI("m_07c7940a29a3") : textUI("m_17e36c43582d"))}</Link></div>{formatUI(h.organizations.length > 0 ? <div className="org-grid">{formatUI(h.organizations.map(o => <article className="org-card" key={o.id}><p className="eyebrow">{formatUI(o.latest_status.replace('_', ' '))}</p><h3><Link to={`/organizations/${o.id}`}>{formatUI(o.name)}</Link></h3><strong className="org-risk">{formatUI(o.risk)}</strong><p>{richUI("m_86370f6e7235", [formatUI(o.authorized), formatUI(o.assets)])}</p><p className="muted small">{formatUI(o.comparison)}</p><div className="row"><Link to={`/organizations/${o.id}`}>{formatUI(can(me, 'assessment.run') ? textUI("m_d5a89eaa6e23") : textUI("m_91020e1cae44"))}</Link>{formatUI(o.latest_assessment && <Link to={`/assessments/${o.latest_assessment}`}>{textUI("m_660449417ba0")}</Link>)}</div></article>))}</div> : <div className="org-card"><h3>{textUI("m_37ce5ee4967c")}</h3><p>{textUI("m_7fd913d97b30")}</p><Link to="/organizations">{formatUI(can(me, 'organization.write') ? textUI("m_63230efa4040") : textUI("m_633c362aeef1"))}</Link></div>)}</section>)}
      <ul className="kpis" aria-label={textUI("m_b6fead36007e")}>
        {formatUI(h.assets && <Kpi label={textUI("m_bd12731d7bc9")} value={h.assets.total} note={`${h.assets.authorized} authorized for testing`} to="/assets"/>)}
        {formatUI(h.chains && <Kpi label={textUI("m_72fa5509b2f9")} value={h.chains.scenarios} to="/chains"/>)}
        {formatUI(h.chains && <Kpi label={textUI("m_078a864ea0f1")} value={h.chains.simulations} note={h.chains.running ? `${h.chains.running} running or paused` : undefined} to="/simulations"/>)}
        {formatUI(h.atomic && <Kpi label={textUI("m_ad1451c7094d")} value={h.atomic.tests} note={h.atomic.active ? `${h.atomic.active} running now` : undefined} to="/atomic"/>)}
        {formatUI(h.atomic && <Kpi label={textUI("m_9da972dae885")} value={h.atomic.runs_7d} note={h.atomic.failed_7d ? `${h.atomic.failed_7d} did not complete` : undefined} to="/atomic"/>)}
        {formatUI(h.injectors.length > 0 && <Kpi label={textUI("m_0db61c756736")} value={h.injectors.filter((i) => i.status === 'online').length} note={`of ${h.injectors.length} configured`}/>)}
        {formatUI(h.findings && <Kpi label={textUI("m_64f74286b22d")} value={h.findings.total} note={h.findings.new_7d ? `+${h.findings.new_7d} this week` : undefined} to="/findings"/>)}
        {formatUI(h.findings && <Kpi label={textUI("m_10153f84a1c8")} value={h.findings.vulnerabilities} to="/findings"/>)}
        {formatUI(h.findings && <Kpi label={textUI("m_3d249dbb9128")} value={h.findings.vulnerable_assets} to="/findings"/>)}
        {formatUI(h.findings && <Kpi label={textUI("m_ea2d982b1997")} value={h.findings.open_ports} to="/findings"/>)}
      </ul>

      {formatUI(h.findings && lvl && (<section className="panel" aria-labelledby="posture-title">
          <div className="posture__head">
            <div>
              <h2 id="posture-title" className="panel__heading">{textUI("m_82e4552a530d")}</h2>
              <p className="muted small">{textUI("m_2bf1c9acabb0")}</p>
            </div>
            <span className={`level level--${lvl.tone}`}>{formatUI(lvl.label)}</span>
          </div>
          <ul className="bars" aria-label={textUI("m_22fe13ca6a2a")}>
            {formatUI(SEVERITIES.map((s) => {
                const n = h.findings!.by_severity[s] ?? 0;
                return (<li key={s}>
                  <span className="bars__label">{formatUI(s[0]!.toUpperCase() + s.slice(1))}</span>
                  <span className="bars__track"><span className={`bars__fill bars__fill--${s}`} style={{ width: `${(n / sevMax) * 100}%` }}/></span>
                  <span className="bars__n">{formatUI(n)}</span>
                </li>);
            }))}
          </ul>
        </section>))}

      {formatUI(h.chains && h.latest_simulations.length > 0 && (<section className="panel" aria-labelledby="sims-latest-title">
          <div className="posture__head"><h2 id="sims-latest-title" className="panel__heading">{textUI("m_c90295345ee7")}</h2><Link to="/simulations" className="link-quiet">{textUI("m_30a64216eaea")}</Link></div>
          <ul className="mini-list">
            {formatUI(h.latest_simulations.map((s) => (<li key={s.id}>
                <div className="mini-list__main"><Link to={`/simulations/legacy/${s.id}`}><strong>{formatUI(s.name)}</strong></Link><span className="muted small">{formatUI(s.code)} · {formatUI(when(s.started_at ?? s.created_at))}</span></div>
                <SimBadge status={s.status}/>
              </li>)))}
          </ul>
        </section>))}

      <div className="home-cols">
        {formatUI(h.atomic && (<section className="panel" aria-labelledby="latest-title">
            <div className="posture__head">
              <h2 id="latest-title" className="panel__heading">{textUI("m_2484b07e83c8")}</h2>
              <Link to="/atomic" className="link-quiet">{textUI("m_30a64216eaea")}</Link>
            </div>
            {formatUI(h.latest_runs.length === 0 ? <p className="muted">{textUI("m_b46f20e3b7c9")}</p> : (<ul className="mini-list">
                {formatUI(h.latest_runs.map((r) => (<li key={r.execution_id}>
                    <div className="mini-list__main">
                      <Link to={`/atomic/${r.test_id}`}><strong>{formatUI(r.test_name)}</strong></Link>
                      <span className="muted small">{formatUI(r.contract_name)} · {formatUI(r.target ?? textUI("m_8581bd739761"))} · {formatUI(when(r.created_at))}</span>
                    </div>
                    <RunBadge run={{ id: r.execution_id, state: r.state, outcome: r.outcome, error: null, created_at: r.created_at, started_at: null, ended_at: null, finding_count: r.finding_count }}/>
                  </li>)))}
              </ul>))}
          </section>))}
        {formatUI(h.findings && (<section className="panel" aria-labelledby="exposed-title">
            <h2 id="exposed-title" className="panel__heading">{textUI("m_78463a7fd155")}</h2>
            {formatUI(h.most_exposed.length === 0 ? <p className="muted">{textUI("m_5af7f5275308")}</p> : (<ul className="mini-list">
                {formatUI(h.most_exposed.map((a) => (<li key={a.asset_id}>
                    <div className="mini-list__main">
                      <Link to={`/assets/${a.asset_id}`}><strong>{formatUI(a.name)}</strong></Link>
                      <span className="muted small">{richUI("m_47e1fe5e70b1", [formatUI(a.open_ports), formatUI(a.open_ports === 1 ? 'port' : 'ports')])}</span>
                    </div>
                    <span className="mini-list__sev">
                      {formatUI(a.critical > 0 && <span className="sev sev--critical">{richUI("m_1e3da69aa261", [formatUI(a.critical)])}</span>)}
                      {formatUI(a.high > 0 && <span className="sev sev--high">{richUI("m_5b8ee36411fb", [formatUI(a.high)])}</span>)}
                      {formatUI(a.medium > 0 && <span className="sev sev--medium">{richUI("m_92f124e956e4", [formatUI(a.medium)])}</span>)}
                      {formatUI(a.critical + a.high + a.medium === 0 && <span className="muted small">{formatUI(a.low > 0 ? textUI("m_20edd6ad4f02", [a.low]) : textUI("m_ca952f95561c"))}</span>)}
                    </span>
                  </li>)))}
              </ul>))}
          </section>))}
      </div>

      {formatUI(offline.length > 0 && (<div className="notice" role="status">
          <strong>{richUI("m_4845a5715da9", [formatUI(offline.map((i) => i.name).join(' and ')), formatUI(offline.length === 1 ? textUI("m_8a27f2fcccaa") : textUI("m_ba5bff2b4cdf"))])}</strong>
          <span>{richUI("m_3f8de742f797", [formatUI(offline.length === 1 ? 'it' : 'them')])}</span>{formatUI(can(me, 'integration.read') ? <Link to="/settings/integrations">{textUI("m_8ac30ac6ae4d")}</Link> : <p>{textUI("m_545a0ca386d8")}</p>)}
        </div>))}
    </>);
}
export function NotFoundPage() {
    useLocale();
    return (<div className="fullpage">
      <p className="eyebrow">404</p>
      <h1 className="title">{textUI("m_4427d9bbc643")}</h1>
      <p className="subtitle">{textUI("m_8052556251cd")}</p>
      <a className="btn btn--secondary btn--auto" href="/">{textUI("m_a0aac9145249")}</a>
    </div>);
}

import { OrganisationSetup } from '../components/OrganisationSetup';
import { Link } from 'react-router-dom';
import { useHome, useMe } from '../api/hooks';
import { can, type HomeSummary } from '../api/types';
import { describeError } from '../components/ErrorNote';
import { RunBadge, when } from '../components/RunBadges';
import { SimBadge } from '../components/StatusPills';

const SEVERITIES = ['critical', 'high', 'medium', 'low', 'info'] as const;

// Tingkat paparan diturunkan dari kerentanan yang ditemukan; bukan skor resmi, hanya ringkasan cepat.
export function exposureLevel(by: Record<string, number>, total: number): { label: string; tone: string } {
  if (total === 0) return { label: 'No data yet', tone: 'none' };
  if ((by.critical ?? 0) > 0) return { label: 'High exposure', tone: 'critical' };
  if ((by.high ?? 0) > 0) return { label: 'Elevated exposure', tone: 'high' };
  if ((by.medium ?? 0) > 0) return { label: 'Moderate exposure', tone: 'medium' };
  return { label: 'Low exposure', tone: 'low' };
}

function Kpi({ label, value, note, to }: { label: string; value: number; note?: string; to?: string }) {
  const body = (
    <>
      <span className="kpi__label">{label}</span>
      <strong className="kpi__value">{value}</strong>
      <span className="kpi__note">{note ?? '\u00a0'}</span>
    </>
  );
  return <li className="kpi">{to ? <Link className="kpi__link" to={to}>{body}</Link> : <div className="kpi__link">{body}</div>}</li>;
}

export function HomePage() {
  const { data: me } = useMe();
  const q = useHome();
  const h = q.data;

  return (
    <section className="page" aria-labelledby="home-title">
      <div className="page__head">
        <div>
          <h1 id="home-title" className="page__title">Home</h1>
          <p className="page__sub">Find security exposure across your organisations, run authorized checks and turn results into reports{me ? `, ${me.display_name}` : ''}.</p>
        </div>
      </div>
      {q.isPending ? (
        <div aria-label="Loading overview"><ul className="kpis">{[0, 1, 2, 3].map((i) => <li key={i} className="kpi"><span className="skeleton" /></li>)}</ul></div>
      ) : q.isError || !h ? (
        <div className="state state--error" role="alert">
          <h2>Couldn’t load the overview</h2><p>{describeError(q.error).message} Your data is safe.</p>
          <button type="button" className="btn btn--secondary btn--auto" onClick={() => q.refetch()}>Try again</button>
        </div>
      ) : (
        <HomeBody h={h} />
      )}
    </section>
  );
}

function HomeBody({ h }: { h: HomeSummary }) {
  const {data:me}=useMe();
  const offline = h.injectors.filter((i) => i.status !== 'online');
  const lvl = h.findings ? exposureLevel(h.findings.by_severity, h.findings.vulnerabilities) : null;
  const sevMax = h.findings ? Math.max(1, ...SEVERITIES.map((s) => h.findings!.by_severity[s] ?? 0)) : 1;

  const starting = h.organizations?.length === 0 && (h.assets?.total ?? 0) === 0 && (h.findings?.total ?? 0) === 0 && (h.atomic?.tests ?? 0) === 0 && (h.chains?.scenarios ?? 0) === 0;
  if (starting) return <>
    {can(me,'organization.read') && <OrganisationSetup />}
    <section className="panel" aria-labelledby="first-results-title"><h2 id="first-results-title" className="panel__heading">No assessment results yet</h2><p className="muted">No data yet. Follow the steps above to create your first organisation assessment; its findings and report will appear here.</p><p className="muted">Scan workers connected: {h.injectors.filter(i=>i.status==='online').length} / {h.injectors.length}. Workers run checks from containers; agents register endpoints.</p>{offline.length>0&&<p>{offline.map(i=>i.name).join(' and ')} {offline.length===1?'injector is':'injectors are'} not connected. Queued scans wait until the worker reconnects.</p>}{can(me,'integration.read')&&<Link to="/settings/integrations">Check scan workers and connection status →</Link>}</section>
  </>;

  return (
    <>
      {can(me,'organization.read') && <OrganisationSetup />}
      {h.organizations&&<section aria-labelledby="organisation-overview-title" className="org-overview"><div className="page__head"><div><p className="eyebrow">YOUR SECURITY POSTURE</p><h2 id="organisation-overview-title">Organisation overview</h2><p className="muted">Coverage and exposure from your latest assessments.</p></div><Link className="btn btn--secondary btn--auto" to="/organizations">{can(me,'organization.write')?'Manage organisations':'View organisations'}</Link></div>{h.organizations.length>0?<div className="org-grid">{h.organizations.map(o=><article className="org-card" key={o.id}><p className="eyebrow">{o.latest_status.replace('_',' ')}</p><h3><Link to={`/organizations/${o.id}`}>{o.name}</Link></h3><strong className="org-risk">{o.risk}</strong><p>{o.authorized} authorized / {o.assets} assets</p><p className="muted small">{o.comparison}</p><div className="row"><Link to={`/organizations/${o.id}`}>{can(me,'assessment.run')?'Assess organisation →':'View organisation →'}</Link>{o.latest_assessment&&<Link to={`/assessments/${o.latest_assessment}`}>Latest assessment</Link>}</div></article>)}</div>:<div className="org-card"><h3>Bring your assets together</h3><p>Create an organisation to assess its authorized assets and generate one consolidated report.</p><Link to="/organizations">{can(me,'organization.write')?'Create your first organisation →':'View organisations →'}</Link></div>}</section>}
      <ul className="kpis" aria-label="Key numbers">
        {h.assets && <Kpi label="Assets" value={h.assets.total} note={`${h.assets.authorized} authorized for testing`} to="/assets" />}
        {h.chains && <Kpi label="Scenarios" value={h.chains.scenarios} to="/chains" />}
        {h.chains && <Kpi label="Simulations" value={h.chains.simulations} note={h.chains.running ? `${h.chains.running} running or paused` : undefined} to="/simulations" />}
        {h.atomic && <Kpi label="Single tests" value={h.atomic.tests} note={h.atomic.active ? `${h.atomic.active} running now` : undefined} to="/atomic" />}
        {h.atomic && <Kpi label="Single-test runs, last 7 days" value={h.atomic.runs_7d} note={h.atomic.failed_7d ? `${h.atomic.failed_7d} did not complete` : undefined} to="/atomic" />}
        {h.injectors.length > 0 && <Kpi label="Injectors online" value={h.injectors.filter((i) => i.status === 'online').length} note={`of ${h.injectors.length} configured`} />}
        {h.findings && <Kpi label="Total findings" value={h.findings.total} note={h.findings.new_7d ? `+${h.findings.new_7d} this week` : undefined} to="/findings" />}
        {h.findings && <Kpi label="Vulnerabilities" value={h.findings.vulnerabilities} to="/findings" />}
        {h.findings && <Kpi label="Vulnerable endpoints" value={h.findings.vulnerable_assets} to="/findings" />}
        {h.findings && <Kpi label="Open ports" value={h.findings.open_ports} to="/findings" />}
      </ul>

      {h.findings && lvl && (
        <section className="panel" aria-labelledby="posture-title">
          <div className="posture__head">
            <div>
              <h2 id="posture-title" className="panel__heading">Exposure posture</h2>
              <p className="muted small">Vulnerabilities found by your scans. Prevention, detection and human response appear once collectors are connected.</p>
            </div>
            <span className={`level level--${lvl.tone}`}>{lvl.label}</span>
          </div>
          <ul className="bars" aria-label="Vulnerabilities by severity">
            {SEVERITIES.map((s) => {
              const n = h.findings!.by_severity[s] ?? 0;
              return (
                <li key={s}>
                  <span className="bars__label">{s[0]!.toUpperCase() + s.slice(1)}</span>
                  <span className="bars__track"><span className={`bars__fill bars__fill--${s}`} style={{ width: `${(n / sevMax) * 100}%` }} /></span>
                  <span className="bars__n">{n}</span>
                </li>
              );
            })}
          </ul>
        </section>
      )}

      {h.chains && h.latest_simulations.length > 0 && (
        <section className="panel" aria-labelledby="sims-latest-title">
          <div className="posture__head"><h2 id="sims-latest-title" className="panel__heading">Latest simulations</h2><Link to="/simulations" className="link-quiet">View all</Link></div>
          <ul className="mini-list">
            {h.latest_simulations.map((s) => (
              <li key={s.id}>
                <div className="mini-list__main"><Link to={`/simulations/legacy/${s.id}`}><strong>{s.name}</strong></Link><span className="muted small">{s.code} · {when(s.started_at ?? s.created_at)}</span></div>
                <SimBadge status={s.status} />
              </li>
            ))}
          </ul>
        </section>
      )}

      <div className="home-cols">
        {h.atomic && (
          <section className="panel" aria-labelledby="latest-title">
            <div className="posture__head">
              <h2 id="latest-title" className="panel__heading">Latest single tests</h2>
              <Link to="/atomic" className="link-quiet">View all</Link>
            </div>
            {h.latest_runs.length === 0 ? <p className="muted">Nothing has run yet.</p> : (
              <ul className="mini-list">
                {h.latest_runs.map((r) => (
                  <li key={r.execution_id}>
                    <div className="mini-list__main">
                      <Link to={`/atomic/${r.test_id}`}><strong>{r.test_name}</strong></Link>
                      <span className="muted small">{r.contract_name} · {r.target ?? 'no target'} · {when(r.created_at)}</span>
                    </div>
                    <RunBadge run={{ id: r.execution_id, state: r.state, outcome: r.outcome, error: null, created_at: r.created_at, started_at: null, ended_at: null, finding_count: r.finding_count }} />
                  </li>
                ))}
              </ul>
            )}
          </section>
        )}
        {h.findings && (
          <section className="panel" aria-labelledby="exposed-title">
            <h2 id="exposed-title" className="panel__heading">Most exposed assets</h2>
            {h.most_exposed.length === 0 ? <p className="muted">No findings on your assets yet.</p> : (
              <ul className="mini-list">
                {h.most_exposed.map((a) => (
                  <li key={a.asset_id}>
                    <div className="mini-list__main">
                      <Link to={`/assets/${a.asset_id}`}><strong>{a.name}</strong></Link>
                      <span className="muted small">{a.open_ports} open {a.open_ports === 1 ? 'port' : 'ports'}</span>
                    </div>
                    <span className="mini-list__sev">
                      {a.critical > 0 && <span className="sev sev--critical">{a.critical} Critical</span>}
                      {a.high > 0 && <span className="sev sev--high">{a.high} High</span>}
                      {a.medium > 0 && <span className="sev sev--medium">{a.medium} Medium</span>}
                      {a.critical + a.high + a.medium === 0 && <span className="muted small">{a.low > 0 ? `${a.low} low` : 'No vulnerabilities'}</span>}
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </section>
        )}
      </div>

      {offline.length > 0 && (
        <div className="notice" role="status">
          <strong>{offline.map((i) => i.name).join(' and ')} {offline.length === 1 ? 'injector is' : 'injectors are'} not connected.</strong>
          <span> Scans for {offline.length === 1 ? 'it' : 'them'} wait in the queue until it starts. A scan worker runs Nmap/Nuclei from its container; an endpoint agent cannot replace it.</span>{can(me,'integration.read') ? <Link to="/settings/integrations">Check scan workers and connection status →</Link> : <p>Ask an administrator to reconnect the scan workers.</p>}
        </div>
      )}
    </>
  );
}

export function NotFoundPage() {
  return (
    <div className="fullpage">
      <p className="eyebrow">404</p>
      <h1 className="title">We can’t find that page</h1>
      <p className="subtitle">The link may be old, or the item was deleted. Check the address or jump somewhere you know.</p>
      <a className="btn btn--secondary btn--auto" href="/">
        Go home
      </a>
    </div>
  );
}

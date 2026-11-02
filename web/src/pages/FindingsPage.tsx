import { OrganizationFilter } from '../components/OrganizationFilter';
import { can } from '../api/types';
import { Search } from 'lucide-react';
import { useEffect, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { useFindings, useMe } from '../api/hooks';
import { describeError } from '../components/ErrorNote';
import { SeverityBadge, when } from '../components/RunBadges';

function useDebounced<T>(value: T, ms: number) {
  const [v, setV] = useState(value);
  useEffect(() => { const t = setTimeout(() => setV(value), ms); return () => clearTimeout(t); }, [value, ms]);
  return v;
}

export function FindingsPage({organizationID,organizationName}:{organizationID?:string;organizationName?:string} = {}) {
  const [params,setParams]=useSearchParams();const assetID=params.get('asset_id')??'';const {data:me}=useMe();
  const organization=organizationID ?? params.get('org') ?? params.get('organization_id') ?? '';
  const Heading=organizationID?'h2':'h1';
  const [text, setText] = useState('');
  const [type, setType] = useState('');
  const [severity, setSeverity] = useState('');
  const q = useDebounced(text.trim(), 250);
  const res = useFindings(q, type, severity, assetID,organization);
  const items = res.data?.pages.flatMap((p) => p.items) ?? [];
  const filtered = !!(q || type || severity || assetID || organization);

  return (
    <section className="page" aria-labelledby="findings-title">
      <div className="page__head">
        <div>
          <div className="page__title-row"><Heading id="findings-title" className="page__title">Findings</Heading></div>
          <p className="page__sub">Everything the scans discovered. The same finding seen again is counted once, with its number of sightings.</p>
        </div>
      </div>
      {assetID&&<div className="prerequisites" aria-label="Asset findings filter"><p>Showing findings for {items[0]?.asset_name || 'the selected asset'}. Search and severity filters stay within this asset.</p><div className="report-relations">{can(me,'asset.read')&&<Link to={`/assets/${encodeURIComponent(assetID)}`}>Review selected asset →</Link>}<button className="btn btn--secondary btn--auto" onClick={()=>{const next=new URLSearchParams(params);next.delete('asset_id');setParams(next);}}>Show findings for all assets</button></div></div>}
      <div className="toolbar" role="search"><OrganizationFilter value={organization} locked={!!organizationID} name={organizationName} onChange={value=>{const next=new URLSearchParams(params);next.delete('organization_id');if(value)next.set('org',value);else next.delete('org');setParams(next);}} />
        <label className="search-input">
          <Search size={15} aria-hidden="true" />
          <input type="search" placeholder="Search findings" aria-label="Search findings" value={text} onChange={(e) => setText(e.target.value)} />
        </label>
        <div className="filter">
          <select aria-label="Filter by type" value={type} onChange={(e) => setType(e.target.value)}>
            <option value="">All types</option><option value="open_port">Open port</option><option value="service">Service</option><option value="vulnerability">Vulnerability</option>
          </select>
        </div>
        <div className="filter">
          <select aria-label="Filter by severity" value={severity} onChange={(e) => setSeverity(e.target.value)}>
            <option value="">All severities</option><option value="critical">Critical</option><option value="high">High</option><option value="medium">Medium</option><option value="low">Low</option><option value="info">Info</option>
          </select>
        </div>
      </div>
      {res.isPending ? (
        <ul className="cards" aria-label="Loading findings">{[0, 1, 2].map((i) => <li key={i} className="card"><span className="skeleton" /></li>)}</ul>
      ) : res.isError ? (
        <div className="state state--error" role="alert">
          <h2>Couldn’t load findings</h2><p>{describeError(res.error).message} Your data is safe.</p>
          <button type="button" className="btn btn--secondary btn--auto" onClick={() => res.refetch()}>Try again</button>
        </div>
      ) : items.length === 0 ? (
        <div className="state">
          <h2>{filtered ? 'No findings match' : 'No findings yet'}</h2>
          <p>{filtered ? 'Try a different search or clear the filters.' : 'Choose an organisation and run a scan on authorized assets to discover findings.'}</p>
          {!filtered && <Link className="btn btn--secondary btn--auto" to="/organizations">Open organisations</Link>}
        </div>
      ) : (
        <>
          <ul className="list finding-list" aria-label="Findings" tabIndex={0}>
            {items.map((f) => (
              <li key={f.id} className="finding-row">
                <SeverityBadge severity={f.severity} />
                <div className="finding-row__main">
                  <strong>{f.title}</strong>
                  <span className="muted small">{f.asset_id ? <Link to={`/assets/${f.asset_id}`}>{f.asset_name}</Link> : 'No asset'} · <code className="mono">{f.value}</code></span>
                </div>
                <span className="muted small">Seen {f.occurrence_count}× · last {when(f.last_seen_at)}</span>
              </li>
            ))}
          </ul>
          {res.hasNextPage && <div className="more"><button type="button" className="btn btn--secondary btn--auto" onClick={() => res.fetchNextPage()} disabled={res.isFetchingNextPage}>{res.isFetchingNextPage ? 'Loading…' : 'Load more'}</button></div>}
        </>
      )}
    </section>
  );
}

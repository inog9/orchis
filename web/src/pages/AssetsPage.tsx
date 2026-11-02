import { ChevronRight, FileUp, Plus, Search } from 'lucide-react';
import { useEffect, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { useAssets, useMe } from '../api/hooks';
import { can, type Asset } from '../api/types';
import { OrganizationFilter } from '../components/OrganizationFilter';
import { AssetTabs } from '../components/AssetTabs';
import { describeError } from '../components/ErrorNote';
import { TagChips } from '../components/TagPicker';
import { ImportDialog } from './ImportDialog';
import { AddAssetDialog } from './AddAssetDialog';
import { CATEGORIES, categoryOf, PLATFORM_LABEL } from './categories';

function useDebounced<T>(value: T, ms: number) {
  const [v, setV] = useState(value);
  useEffect(() => {
    const t = setTimeout(() => setV(value), ms);
    return () => clearTimeout(t);
  }, [value, ms]);
  return v;
}

function Row({ a }: { a: Asset }) {
  const cat = categoryOf(a.category);
  const Icon = cat.icon;
  const primary = a.addresses.find((x) => x.is_primary) ?? a.addresses[0];
  return (
    <li className="asset-row">
      <div className="asset-row__main">
        <span className="asset-row__icon">
          <Icon size={17} />
        </span>
        <div className="asset-row__text">
          <Link className="asset-row__name" to={`/assets/${a.id}`}>{a.name}</Link>
          <span className="asset-row__meta">
            {cat.label}
            <i aria-hidden="true" />
            Added {new Date(a.created_at).toLocaleDateString('en-US', { month: 'short', day: 'numeric' })}
            {a.agent_state && a.agent_state !== 'none' && (
              <>
                <i aria-hidden="true" />
                Agent {a.agent_state}
              </>
            )}
            {a.is_internet_facing && (
              <>
                <i aria-hidden="true" />
                Internet-facing
              </>
            )}
          </span>
          <TagChips tags={a.tags} />
        </div>
      </div>
      <div className="asset-row__cell asset-row__cell--platform">
        <span className="strong">{a.platform ? (PLATFORM_LABEL[a.platform] ?? a.platform) : '—'}</span>
        {a.arch && <span className="muted small">{a.arch}</span>}
      </div>
      <div className="asset-row__cell asset-row__cell--address">
        {primary ? <span className="mono">{primary.value}{primary.scan_approved===false?' · Admin confirmation required':''}</span> : <span className="muted">No address</span>}
        {a.addresses.length > 1 && <span className="muted small">+{a.addresses.length - 1} more</span>}
      </div>
      <div className="asset-row__cell asset-row__cell--crit">
        <span className={`crit crit--${a.criticality}`}>{a.criticality[0]!.toUpperCase() + a.criticality.slice(1)}</span>
      </div>
      <div className="asset-row__cell asset-row__cell--testing">
        {a.testing_authorized ? <span className="status status--ok">Authorized</span> : <span className="status">Not authorized</span>}
      </div>
    </li>
  );
}

export function AssetsPage({organizationID,organizationName}:{organizationID?:string;organizationName?:string} = {}) {
  const { data: me } = useMe();
  const [params, setParams] = useSearchParams();
  const [q, setQ] = useState('');
  const [category, setCategory] = useState('');
  const dq = useDebounced(q.trim(), 250);
  const organization=organizationID ?? params.get('org') ?? params.get('organization_id') ?? '';
  const Heading=organizationID?'h2':'h1';
  const list = useAssets(dq, category, organization);
  const [adding, setAdding] = useState(params.get('new') === '1');
  const [importing, setImporting] = useState(false);
  const canWrite = can(me, 'asset.write') && (!organization || organization==='unassigned' || can(me,'organization.write'));

  const items = list.data?.pages.flatMap((p) => p.items) ?? [];
  const filtered = !!dq || !!category || !!organization;

  const closeDialog = () => {
    setAdding(false);
    if (params.has('new')) {
      params.delete('new');
      setParams(params, { replace: true });
    }
  };

  return (
    <section aria-labelledby="assets-title" className="page">
      <div className="page__head">
        <div>
          <nav className="crumbs" aria-label="Breadcrumb">
            <span>Assets</span>
            <ChevronRight size={12} aria-hidden="true" />
            <span className="crumbs__current">Endpoints</span>
          </nav>
          <div className="page__title-row">
            <Heading id="assets-title" className="page__title">
              Assets
            </Heading>
            {!list.isPending && !list.isError && <span className="badge">{items.length}{list.hasNextPage ? '+' : ''}</span>}
          </div>
          <p className="page__sub">Machines and services your simulations can target.</p>
        </div>
        {canWrite && (
          <div className="page__actions">
            {!organizationID && <button type="button" className="btn btn--secondary btn--auto" onClick={() => setImporting(true)}>
              <FileUp size={16} aria-hidden="true" /> Import CSV
            </button>}
            <button type="button" className="btn btn--primary btn--auto" onClick={() => setAdding(true)}>
              <Plus size={16} aria-hidden="true" /> Add asset
            </button>
          </div>
        )}
      </div>
      {!organizationID && <AssetTabs />}

      <div className="toolbar"><OrganizationFilter value={organization} locked={!!organizationID} name={organizationName} allowUnassigned onChange={value=>{const next=new URLSearchParams(params);next.delete('organization_id');if(value)next.set('org',value);else next.delete('org');setParams(next);}} />
        <label className="search-input">
          <Search size={15} aria-hidden="true" />
          <input type="search" placeholder="Search by name…" aria-label="Search assets" value={q} onChange={(e) => setQ(e.target.value)} />
        </label>
        <label className="filter">
          <span className="sr-only">Category</span>
          <select value={category} onChange={(e) => setCategory(e.target.value)} aria-label="Category">
            <option value="">All categories</option>
            {CATEGORIES.map((c) => (
              <option key={c.key} value={c.key}>
                {c.label}
              </option>
            ))}
          </select>
        </label>
      </div>

      <div className="list" aria-busy={list.isPending} tabIndex={0}>
        <div className="list__head" aria-hidden="true">
          <span>ASSET</span>
          <span>PLATFORM</span>
          <span>ADDRESS</span>
          <span>CRITICALITY</span>
          <span>TESTING</span>
        </div>
        {list.isPending ? (
          <ul className="list__body" aria-label="Loading assets">
            {[0, 1, 2, 3].map((i) => (
              <li key={i} className="asset-row asset-row--skeleton">
                <span className="skeleton" />
              </li>
            ))}
          </ul>
        ) : list.isError ? (
          <div className="state state--error" role="alert">
            <h2>Couldn’t load assets</h2>
            <p>{describeError(list.error).message} Your data is safe.</p>
            {describeError(list.error).errorId && <code className="mono">{describeError(list.error).errorId}</code>}
            <button type="button" className="btn btn--secondary btn--auto" onClick={() => list.refetch()}>
              Try again
            </button>
          </div>
        ) : items.length === 0 ? (
          <div className="state">
            <h2>{filtered ? 'No assets match your filters' : 'No assets yet'}</h2>
            <p>
              {filtered
                ? 'Try a different search or clear the category filter.'
                : 'Assets are the machines and services your simulations can target. Add one to get started.'}
            </p>
            {!filtered && canWrite && (
              <button type="button" className="btn btn--primary btn--auto" onClick={() => setAdding(true)}>
                <Plus size={16} aria-hidden="true" /> Add asset
              </button>
            )}
          </div>
        ) : (
          <ul className="list__body">
            {items.map((a) => (
              <Row key={a.id} a={a} />
            ))}
          </ul>
        )}
      </div>

      {list.hasNextPage && (
        <div className="more">
          <button type="button" className="btn btn--secondary btn--auto" onClick={() => list.fetchNextPage()} disabled={list.isFetchingNextPage}>
            {list.isFetchingNextPage ? 'Loading…' : 'Load more'}
          </button>
        </div>
      )}
      {adding && canWrite && <AddAssetDialog onClose={closeDialog} organizationID={organization && organization!=='unassigned'?organization:undefined} />}
      {importing && <ImportDialog onClose={() => setImporting(false)} />}
    </section>
  );
}

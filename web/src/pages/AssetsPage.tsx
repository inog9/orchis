import { t as textUI, tr as richUI, useLocale, locale, fv as formatUI } from "../i18n/runtime";
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
import { OrganizationGroup } from '../components/OrganizationGroup';
import { groupByOrganization } from '../lib/groupByOrganization';
import { CATEGORIES, categoryOf, PLATFORM_LABEL } from './categories';
function useDebounced<T>(value: T, ms: number) {
    const [v, setV] = useState(value);
    useEffect(() => {
        const t = setTimeout(() => setV(value), ms);
        return () => clearTimeout(t);
    }, [value, ms]);
    return v;
}
function Row({ a }: {
    a: Asset;
}) {
    useLocale();
    const cat = categoryOf(a.category);
    const Icon = cat.icon;
    const primary = a.addresses.find((x) => x.is_primary) ?? a.addresses[0];
    return (<li className="asset-row">
      <div className="asset-row__main">
        <span className="asset-row__icon">
          <Icon size={17}/>
        </span>
        <div className="asset-row__text">
          <Link className="asset-row__name" to={`/assets/${a.id}`}>{formatUI(a.name)}</Link>
          <span className="asset-row__meta">{richUI("m_afaa3999bffe", [formatUI(cat.label), <i aria-hidden="true"/>, formatUI(new Date(a.created_at).toLocaleDateString(locale(), { month: 'short', day: 'numeric' })), formatUI(a.agent_state && a.agent_state !== 'none' && (<>
                <i aria-hidden="true"/>
                Agent {formatUI(a.agent_state)}
              </>)), formatUI(a.is_internet_facing && (<>
                <i aria-hidden="true"/>
                Internet-facing
              </>))])}</span>
          <TagChips tags={a.tags}/>
        </div>
      </div>
      <div className="asset-row__cell asset-row__cell--platform">
        <span className="strong">{formatUI(a.platform ? (PLATFORM_LABEL[a.platform] ?? a.platform) : '—')}</span>
        {formatUI(a.arch && <span className="muted small">{formatUI(a.arch)}</span>)}
      </div>
      <div className="asset-row__cell asset-row__cell--address">
        {formatUI(primary ? <span className="mono">{formatUI(primary.value)}{formatUI(primary.scan_approved === false ? ' · Admin confirmation required' : '')}</span> : <span className="muted">{textUI("m_841a3bc2f23b")}</span>)}
        {formatUI(a.addresses.length > 1 && <span className="muted small">{richUI("m_3f1d2fb45e57", [formatUI(a.addresses.length - 1)])}</span>)}
      </div>
      <div className="asset-row__cell asset-row__cell--crit">
        <span className={`crit crit--${a.criticality}`}>{formatUI(a.criticality[0]!.toUpperCase() + a.criticality.slice(1))}</span>
      </div>
      <div className="asset-row__cell asset-row__cell--testing">
        {formatUI(a.testing_authorized ? <span className="status status--ok">{textUI("m_67f542ba6d8f")}</span> : <span className="status">{textUI("m_52c79c7b5ba0")}</span>)}
      </div>
    </li>);
}
export function AssetsPage({ organizationID, organizationName }: {
    organizationID?: string;
    organizationName?: string;
} = {}) {
    useLocale();
    const { data: me } = useMe();
    const [params, setParams] = useSearchParams();
    const [q, setQ] = useState('');
    const [category, setCategory] = useState('');
    const dq = useDebounced(q.trim(), 250);
    const organization = organizationID ?? params.get('org') ?? params.get('organization_id') ?? '';
    const Heading = organizationID ? 'h2' : 'h1';
    const list = useAssets(dq, category, organization);
    const [adding, setAdding] = useState(params.get('new') === '1');
    const [importing, setImporting] = useState(false);
    const canWrite = can(me, 'asset.write') && (!organization || organization === 'unassigned' || can(me, 'organization.write'));
    const items = list.data?.pages.flatMap((p) => p.items) ?? [];
    const filtered = !!dq || !!category || !!organization;
    const closeDialog = () => {
        setAdding(false);
        if (params.has('new')) {
            params.delete('new');
            setParams(params, { replace: true });
        }
    };
    return (<section aria-labelledby="assets-title" className="page">
      <div className="page__head">
        <div>
          <nav className="crumbs" aria-label={textUI("m_2bd873d6c734")}>
            <span>{textUI("m_bd12731d7bc9")}</span>
            <ChevronRight size={12} aria-hidden="true"/>
            <span className="crumbs__current">{textUI("m_fce7fec2a58c")}</span>
          </nav>
          <div className="page__title-row">
            <Heading id="assets-title" className="page__title">{textUI("m_bd12731d7bc9")}</Heading>
            {formatUI(!list.isPending && !list.isError && <span className="badge">{formatUI(items.length)}{formatUI(list.hasNextPage ? '+' : '')}</span>)}
          </div>
          <p className="page__sub">{textUI("m_31c3b3913c54")}</p>
        </div>
        {formatUI(canWrite && (<div className="page__actions">
            {formatUI(!organizationID && <button type="button" className="btn btn--secondary btn--auto" onClick={() => setImporting(true)}>{richUI("m_24996c82cca2", [<FileUp size={16} aria-hidden="true"/>])}</button>)}
            <button type="button" className="btn btn--primary btn--auto" onClick={() => setAdding(true)}>{richUI("m_8a7bbf830b09", [<Plus size={16} aria-hidden="true"/>])}</button>
          </div>))}
      </div>
      {formatUI(!organizationID && <AssetTabs />)}

      <div className="toolbar"><OrganizationFilter value={organization} locked={!!organizationID} name={organizationName} allowUnassigned onChange={value => { const next = new URLSearchParams(params); next.delete('organization_id'); if (value)
        next.set('org', value);
    else
        next.delete('org'); setParams(next); }}/>
        <label className="search-input">
          <Search size={15} aria-hidden="true"/>
          <input type="search" placeholder={textUI("m_8cf423956b2a")} aria-label={textUI("m_f354c51e0b53")} value={q} onChange={(e) => setQ(e.target.value)}/>
        </label>
        <label className="filter">
          <span className="sr-only">{textUI("m_292c06f0045a")}</span>
          <select value={category} onChange={(e) => setCategory(e.target.value)} aria-label={textUI("m_292c06f0045a")}>
            <option value="">{textUI("m_9d5097a83710")}</option>
            {formatUI(CATEGORIES.map((c) => (<option key={c.key} value={c.key}>
                {formatUI(c.label)}
              </option>)))}
          </select>
        </label>
      </div>

      <div className="list" aria-busy={list.isPending} tabIndex={0}>
        <div className="list__head" aria-hidden="true">
          <span>{textUI("m_68eef920516b")}</span>
          <span>{textUI("m_6f38f66d9a24")}</span>
          <span>{textUI("m_19dbf7b7fc66")}</span>
          <span>{textUI("m_2b62f7e5ac90")}</span>
          <span>{textUI("m_5de71b2143c8")}</span>
        </div>
        {formatUI(list.isPending ? (<ul className="list__body" aria-label={textUI("m_d480f4dfb935")}>
            {formatUI([0, 1, 2, 3].map((i) => (<li key={i} className="asset-row asset-row--skeleton">
                <span className="skeleton"/>
              </li>)))}
          </ul>) : list.isError ? (<div className="state state--error" role="alert">
            <h2>{textUI("m_fa0ce6c7aacb")}</h2>
            <p>{richUI("m_19636dc199da", [formatUI(describeError(list.error).message)])}</p>
            {formatUI(describeError(list.error).errorId && <code className="mono">{describeError(list.error).errorId}</code>)}
            <button type="button" className="btn btn--secondary btn--auto" onClick={() => list.refetch()}>{textUI("m_d8b8392e2c54")}</button>
          </div>) : items.length === 0 ? (<div className="state">
            <h2>{formatUI(filtered ? textUI("m_a1f8766c1dcf") : textUI("m_cb5efbf3ab92"))}</h2>
            <p>
              {formatUI(filtered
                ? textUI("m_f69729a1d2dc") : textUI("m_33178926dc2b"))}
            </p>
            {formatUI(!filtered && canWrite && (<button type="button" className="btn btn--primary btn--auto" onClick={() => setAdding(true)}>{richUI("m_8a7bbf830b09", [<Plus size={16} aria-hidden="true"/>])}</button>))}
          </div>) : organization ? (<ul className="list__body">
            {formatUI(items.map((a) => (<Row key={a.id} a={a}/>)))}
          </ul>) : (<div className="org-groups">
            {formatUI(groupByOrganization(items).map((g) => (<OrganizationGroup key={g.key} id={g.id} name={g.name} linkable={can(me, 'organization.read')} summary={<span>{richUI("m_grp_n_assets", [formatUI(g.items.length)])}</span>}>
                <ul className="list__body">
                  {formatUI(g.items.map((a) => (<Row key={a.id} a={a}/>)))}
                </ul>
              </OrganizationGroup>)))}
          </div>))}
      </div>

      {formatUI(list.hasNextPage && (<div className="more">
          <button type="button" className="btn btn--secondary btn--auto" onClick={() => list.fetchNextPage()} disabled={list.isFetchingNextPage}>
            {formatUI(list.isFetchingNextPage ? textUI("m_ba3bbbe10d8b") : textUI("m_ac8991ef0101"))}
          </button>
        </div>))}
      {formatUI(adding && canWrite && <AddAssetDialog onClose={closeDialog} organizationID={organization && organization !== 'unassigned' ? organization : undefined}/>)}
      {formatUI(importing && <ImportDialog onClose={() => setImporting(false)}/>)}
    </section>);
}

import { t as textUI, tr as richUI, useLocale, locale, fv as formatUI } from "../i18n/runtime";
import { AssetFindings } from '../components/AssetFindings';
import { EndpointDataPanel } from '../components/EndpointDataPanel';
import { ChevronRight, Pencil, Trash2 } from 'lucide-react';
import { useState, Fragment } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { ApiError } from '../api/client';
import { useAsset, useDeleteAsset, useMe } from '../api/hooks';
import { can } from '../api/types';
import { describeError } from '../components/ErrorNote';
import { TagChips } from '../components/TagPicker';
import { ConfirmDialog } from './ConfirmDialog';
import { EditAssetDialog } from './EditAssetDialog';
import { categoryOf, PLATFORM_LABEL } from './categories';
import { NOTES_LABEL, specOf } from './categorySchema';
const fmt = (iso: string) => new Date(iso).toLocaleString(locale(), { month: 'short', day: 'numeric', year: 'numeric', hour: '2-digit', minute: '2-digit' });
export function AssetDetailPage() {
    useLocale();
    const { id = '' } = useParams();
    const navigate = useNavigate();
    const { data: me } = useMe();
    const q = useAsset(id);
    const del = useDeleteAsset(id);
    const [editing, setEditing] = useState(false);
    const [confirming, setConfirming] = useState(false);
    if (q.isPending)
        return <div className="page" aria-busy="true"><span className="skeleton" style={{ height: 120 }}/></div>;
    if (q.isError) {
        const notFound = q.error instanceof ApiError && q.error.status === 404;
        const d = describeError(q.error);
        return (<div className="state state--error" role="alert">
        <h2>{formatUI(notFound ? textUI("m_3965d1967e57") : textUI("m_cede667c6f14"))}</h2>
        <p>{formatUI(notFound ? textUI("m_50c2d179722c") : textUI("m_19636dc199da", [d.message]))}</p>
        {formatUI(d.errorId && !notFound && <code className="mono">{d.errorId}</code>)}
        <Link className="btn btn--secondary btn--auto" to="/assets">{textUI("m_089fb64fac74")}</Link>
      </div>);
    }
    const a = q.data;
    const cat = categoryOf(a.category);
    const Icon = cat.icon;
    return (<section className="page" aria-labelledby="asset-title">
      <div className="page__head">
        <div>
          <nav className="crumbs" aria-label={textUI("m_2bd873d6c734")}>
            <Link to="/assets">{textUI("m_bd12731d7bc9")}</Link><ChevronRight size={12} aria-hidden="true"/><span className="crumbs__current">{formatUI(a.name)}</span>
          </nav>
          <div className="page__title-row">
            <span className="asset-row__icon"><Icon size={17}/></span>
            <h1 id="asset-title" className="page__title mono-title">{formatUI(a.name)}</h1>
          </div>
          <p className="page__sub">
            {formatUI(cat.label)}{formatUI(a.platform ? ` · ${PLATFORM_LABEL[a.platform] ?? a.platform}` : '')}{formatUI(a.arch ? ` · ${a.arch}` : '')}
          </p>
          <TagChips tags={a.tags}/>
        </div>
        {formatUI(can(me, 'asset.write') && (<button type="button" className="btn btn--secondary btn--auto" onClick={() => setEditing(true)}>{richUI("m_12a64771a7ec", [<Pencil size={15} aria-hidden="true"/>])}</button>))}
      </div>

      <section className="prerequisites" aria-label={textUI("m_98fbfab94ab1")}>
        {formatUI(a.organization_id ? <p>{richUI("m_1d8905abad98", [<Link to={`/organizations/${a.organization_id}`}>{textUI("m_ba96d8c0fc0d")}</Link>])}</p> : <p>{richUI("m_9c2ee8a06957", [<Link to="/organizations">{textUI("m_a9e7421536b1")}</Link>])}</p>)}
        {formatUI(!a.testing_authorized && <p>{formatUI(can(me, 'asset.authorize') ? textUI("m_305ea5d5b2c5") : textUI("m_755b4f54f75a"))}</p>)}
        {formatUI(a.addresses.length === 0 && <p>{richUI("m_d11593d96866", [formatUI(can(me, 'asset.write') ? textUI("m_865fca90544a") : textUI("m_9f6dd4c28d7c"))])}</p>)}
        {formatUI(a.agent_state !== 'online' && <p>{richUI("m_13d267b8d0a8", [<Link to="/assets/agents">{textUI("m_62bcf6663fd7")}</Link>])}</p>)}
      </section>
      <div className="grid-2">
        <section className="panel" aria-labelledby="details-h">
          <h2 id="details-h" className="panel__title">{textUI("m_45989de49fb7")}</h2>
          <dl className="dl">
            <dt>{textUI("m_afdc3ea3ef86")}</dt><dd><span className={`crit crit--${a.criticality}`}>{formatUI(a.criticality[0]!.toUpperCase() + a.criticality.slice(1))}</span></dd>
            <dt>{textUI("m_8cc5257478f0")}</dt><dd>{formatUI(a.is_internet_facing ? textUI("m_85a39ab345d6") : textUI("m_1ea442a134b2"))}</dd>
            <dt>{textUI("m_bb186757b4bf")}</dt><dd>{formatUI(a.agent_state === 'online' ? <span className="status status--ok">{textUI("m_2d59e6737ad8")}</span> : a.agent_state === 'offline' ? <span className="status status--bad">{textUI("m_8853f5f45fe6")}</span> : <span className="status">{textUI("m_1f0d3f74b799")}</span>)}</dd>
            <dt>{textUI("m_e806a291cfc3")}</dt><dd>{formatUI(a.testing_authorized ? <span className="status status--ok">{textUI("m_67f542ba6d8f")}</span> : <span className="status">{textUI("m_52c79c7b5ba0")}</span>)}</dd>
            <dt>{textUI("m_0e570ca6fabe")}</dt><dd>{formatUI(a.source)}</dd>
            <dt>{textUI("m_6b02e0d363a4")}</dt><dd>{formatUI(fmt(a.created_at))}</dd>
            <dt>{textUI("m_3a5ecca188c0")}</dt><dd>{formatUI(fmt(a.updated_at))}</dd>
          </dl>
        </section>
        {formatUI(specOf(a.category).fields.length > 0 && (<section className="panel" aria-labelledby="cat-h">
          <h2 id="cat-h" className="panel__title">{formatUI(cat.label)}</h2>
          <dl className="dl">
            {formatUI(specOf(a.category).fields.map((f) => {
                const raw = a.attributes?.[f.key];
                const value = typeof raw === 'string' && raw ? (f.options.find((o) => o.value === raw)?.label ?? raw) : '—';
                return <Fragment key={f.key}><dt>{formatUI(f.label)}</dt><dd>{formatUI(value)}</dd></Fragment>;
            }))}
            {formatUI(a.description && <><dt>{formatUI(NOTES_LABEL())}</dt><dd>{formatUI(a.description)}</dd></>)}
          </dl>
        </section>))}
        <section className="panel" aria-labelledby="addr-h">
          <h2 id="addr-h" className="panel__title">{textUI("m_76179aafc43c")}</h2>
          {formatUI(a.addresses.length === 0 ? <p className="muted">{textUI("m_79da1e76bc61")}</p> : (<ul className="plain-list">
              {formatUI(a.addresses.map((ad) => (<li key={ad.kind + ad.value}><span className="kind">{formatUI(ad.kind)}</span><span className="mono">{formatUI(ad.value)}</span>{formatUI(ad.is_primary && <span className="muted small">{textUI("m_986a1b7135f4")}</span>)}</li>)))}
            </ul>))}
        </section>
        <section className="panel" aria-labelledby="groups-h">
          <h2 id="groups-h" className="panel__title">{textUI("m_39bbb719fa2b")}</h2>
          {formatUI((a.groups?.length ?? 0) === 0 ? <p className="muted">{textUI("m_7057c8895d13")}</p> : (<ul className="plain-list">
              {formatUI(a.groups!.map((g) => (<li key={g.id}><Link to={`/assets/groups/${g.id}`}>{formatUI(g.name)}</Link>{formatUI(g.is_system && <span className="muted small">{textUI("m_5c73a5c73dc1")}</span>)}</li>)))}
            </ul>))}
          <p className="muted small">{textUI("m_850f42574055")}</p>
        </section>
        <AssetFindings assetID={a.id} allowed={can(me, 'finding.read')}/>
        <EndpointDataPanel assetID={a.id} allowed={can(me, 'asset.read')} canRun={can(me, 'atomic.run')} canDelete={can(me, 'asset.write') && can(me, 'settings.manage')}/>
      </div>

      {formatUI(can(me, 'asset.delete') && (<section className="danger" aria-labelledby="danger-h">
          <div><h2 id="danger-h" className="panel__title">{textUI("m_fd8b8dae4421")}</h2><p className="muted">{textUI("m_128ede4e9e04")}</p></div>
          <button type="button" className="btn btn--danger btn--auto" onClick={() => setConfirming(true)}>{richUI("m_bb0aee048315", [<Trash2 size={15} aria-hidden="true"/>])}</button>
        </section>))}

      {formatUI(editing && <EditAssetDialog asset={a} onClose={() => setEditing(false)} onStale={() => { setEditing(false); q.refetch(); }}/>)}
      {formatUI(confirming && (<ConfirmDialog title={textUI("m_9d64ce84e2c3", [a.name])} body="It will disappear from lists and groups. This can’t be undone from the console." confirmLabel="Remove asset" busy={del.isPending} error={del.isError ? describeError(del.error).message : undefined} onClose={() => setConfirming(false)} onConfirm={() => del.mutate(undefined, { onSuccess: () => navigate('/assets', { replace: true }) })}/>))}
    </section>);
}

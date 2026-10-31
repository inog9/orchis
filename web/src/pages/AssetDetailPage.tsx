import { AssetFindings } from '../components/AssetFindings';
import { ChevronRight, Pencil, Trash2 } from 'lucide-react';
import { useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { ApiError } from '../api/client';
import { useAsset, useDeleteAsset, useMe } from '../api/hooks';
import { can } from '../api/types';
import { describeError } from '../components/ErrorNote';
import { TagChips } from '../components/TagPicker';
import { ConfirmDialog } from './ConfirmDialog';
import { EditAssetDialog } from './EditAssetDialog';
import { categoryOf, PLATFORM_LABEL } from './categories';

const fmt = (iso: string) => new Date(iso).toLocaleString('en-US', { month: 'short', day: 'numeric', year: 'numeric', hour: '2-digit', minute: '2-digit' });

export function AssetDetailPage() {
  const { id = '' } = useParams();
  const navigate = useNavigate();
  const { data: me } = useMe();
  const q = useAsset(id);
  const del = useDeleteAsset(id);
  const [editing, setEditing] = useState(false);
  const [confirming, setConfirming] = useState(false);

  if (q.isPending) return <div className="page" aria-busy="true"><span className="skeleton" style={{ height: 120 }} /></div>;
  if (q.isError) {
    const notFound = q.error instanceof ApiError && q.error.status === 404;
    const d = describeError(q.error);
    return (
      <div className="state state--error" role="alert">
        <h2>{notFound ? 'Asset not found' : 'Couldn’t load this asset'}</h2>
        <p>{notFound ? 'It may have been removed.' : `${d.message} Your data is safe.`}</p>
        {d.errorId && !notFound && <code className="mono">{d.errorId}</code>}
        <Link className="btn btn--secondary btn--auto" to="/assets">Back to assets</Link>
      </div>
    );
  }
  const a = q.data;
  const cat = categoryOf(a.category);
  const Icon = cat.icon;

  return (
    <section className="page" aria-labelledby="asset-title">
      <div className="page__head">
        <div>
          <nav className="crumbs" aria-label="Breadcrumb">
            <Link to="/assets">Assets</Link><ChevronRight size={12} aria-hidden="true" /><span className="crumbs__current">{a.name}</span>
          </nav>
          <div className="page__title-row">
            <span className="asset-row__icon"><Icon size={17} /></span>
            <h1 id="asset-title" className="page__title mono-title">{a.name}</h1>
          </div>
          <p className="page__sub">
            {cat.label}{a.platform ? ` · ${PLATFORM_LABEL[a.platform] ?? a.platform}` : ''}{a.arch ? ` · ${a.arch}` : ''}
          </p>
          <TagChips tags={a.tags} />
        </div>
        {can(me, 'asset.write') && (
          <button type="button" className="btn btn--secondary btn--auto" onClick={() => setEditing(true)}><Pencil size={15} aria-hidden="true" /> Edit</button>
        )}
      </div>

      <section className="prerequisites" aria-label="Asset testing readiness">
        {a.organization_id ? <p>Organisation: <Link to={`/organizations/${a.organization_id}`}>Open organisation and its reports →</Link></p> : <p>Unassigned asset. <Link to="/organizations">Assign it from an organisation →</Link></p>}
        {!a.testing_authorized && <p>{can(me,'asset.authorize')?'Testing is not authorized. Use Edit to approve testing only if you have permission to test this target.':'Testing is not authorized. Ask an administrator to approve it.'}</p>}
        {a.addresses.length===0&&<p>No target address is configured. {can(me,'asset.write')?'Use Edit to add an approved IP address and web URL.':'Ask an administrator to add approved target addresses.'} Agents do not set trusted scan addresses automatically.</p>}
        {a.agent_state!=='online'&&<p>Local endpoint checks need an active agent. Nmap/Nuclei use a separate scan worker and can scan reachable targets without an agent. <Link to="/assets/agents">Review agents →</Link></p>}
      </section>
      <div className="grid-2">
        <section className="panel" aria-labelledby="details-h">
          <h2 id="details-h" className="panel__title">Details</h2>
          <dl className="dl">
            <dt>Criticality</dt><dd><span className={`crit crit--${a.criticality}`}>{a.criticality[0]!.toUpperCase() + a.criticality.slice(1)}</span></dd>
            <dt>Internet-facing</dt><dd>{a.is_internet_facing ? 'Yes' : 'No'}</dd>
            <dt>Executor</dt><dd>{a.agent_state === 'online' ? <span className="status status--ok">Agent online</span> : a.agent_state === 'offline' ? <span className="status status--bad">Agent offline</span> : <span className="status">Agentless</span>}</dd>
            <dt>Testing</dt><dd>{a.testing_authorized ? <span className="status status--ok">Authorized</span> : <span className="status">Not authorized</span>}</dd>
            <dt>Source</dt><dd>{a.source}</dd>
            <dt>Added</dt><dd>{fmt(a.created_at)}</dd>
            <dt>Updated</dt><dd>{fmt(a.updated_at)}</dd>
          </dl>
        </section>
        <section className="panel" aria-labelledby="addr-h">
          <h2 id="addr-h" className="panel__title">Addresses</h2>
          {a.addresses.length === 0 ? <p className="muted">No addresses yet.</p> : (
            <ul className="plain-list">
              {a.addresses.map((ad) => (
                <li key={ad.kind + ad.value}><span className="kind">{ad.kind}</span><span className="mono">{ad.value}</span>{ad.is_primary && <span className="muted small">primary</span>}</li>
              ))}
            </ul>
          )}
        </section>
        <section className="panel" aria-labelledby="groups-h">
          <h2 id="groups-h" className="panel__title">Groups</h2>
          {(a.groups?.length ?? 0) === 0 ? <p className="muted">Not in any group.</p> : (
            <ul className="plain-list">
              {a.groups!.map((g) => (
                <li key={g.id}><Link to={`/assets/groups/${g.id}`}>{g.name}</Link>{g.is_system && <span className="muted small">built-in</span>}</li>
              ))}
            </ul>
          )}
          <p className="muted small">Members join automatically when they match a group’s rules.</p>
        </section>
        <AssetFindings assetID={a.id} allowed={can(me,'finding.read')}/>
      </div>

      {can(me, 'asset.delete') && (
        <section className="danger" aria-labelledby="danger-h">
          <div><h2 id="danger-h" className="panel__title">Danger zone</h2><p className="muted">Removing an asset hides it everywhere. Past runs keep their history.</p></div>
          <button type="button" className="btn btn--danger btn--auto" onClick={() => setConfirming(true)}><Trash2 size={15} aria-hidden="true" /> Remove asset</button>
        </section>
      )}

      {editing && <EditAssetDialog asset={a} onClose={() => setEditing(false)} onStale={() => { setEditing(false); q.refetch(); }} />}
      {confirming && (
        <ConfirmDialog title={`Remove ${a.name}?`} body="It will disappear from lists and groups. This can’t be undone from the console." confirmLabel="Remove asset"
          busy={del.isPending} error={del.isError ? describeError(del.error).message : undefined}
          onClose={() => setConfirming(false)} onConfirm={() => del.mutate(undefined, { onSuccess: () => navigate('/assets', { replace: true }) })} />
      )}
    </section>
  );
}

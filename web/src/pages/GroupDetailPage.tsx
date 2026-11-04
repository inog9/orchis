import { t as textUI, tr as richUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { ChevronRight, Pencil, Trash2 } from 'lucide-react';
import { useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { ApiError } from '../api/client';
import { useDeleteGroup, useGroup, useMe } from '../api/hooks';
import { can } from '../api/types';
import { describeError } from '../components/ErrorNote';
import { describeFilter } from '../lib/rule';
import { ConfirmDialog } from './ConfirmDialog';
import { GroupDialog } from './GroupDialog';
import { categoryOf, PLATFORM_LABEL } from './categories';
export function GroupDetailPage() {
    useLocale();
    const { id = '' } = useParams();
    const navigate = useNavigate();
    const { data: me } = useMe();
    const q = useGroup(id);
    const del = useDeleteGroup(id);
    const [editing, setEditing] = useState(false);
    const [confirming, setConfirming] = useState(false);
    if (q.isPending)
        return <div className="page" aria-busy="true"><span className="skeleton" style={{ height: 120 }}/></div>;
    if (q.isError) {
        const notFound = q.error instanceof ApiError && q.error.status === 404;
        return (<div className="state state--error" role="alert">
        <h2>{formatUI(notFound ? textUI("m_c306f1b599ea") : textUI("m_7b30271f5c70"))}</h2>
        <p>{formatUI(notFound ? textUI("m_992276f72e41") : textUI("m_19636dc199da", [describeError(q.error).message]))}</p>
        <Link className="btn btn--secondary btn--auto" to="/assets/groups">{textUI("m_033bcce2e9e6")}</Link>
      </div>);
    }
    const g = q.data;
    return (<section className="page" aria-labelledby="group-name">
      <div className="page__head">
        <div>
          <nav className="crumbs" aria-label={textUI("m_2bd873d6c734")}>
            <Link to="/assets/groups">{textUI("m_39dd12de3b02")}</Link><ChevronRight size={12} aria-hidden="true"/><span className="crumbs__current">{formatUI(g.name)}</span>
          </nav>
          <div className="page__title-row">
            <h1 id="group-name" className="page__title">{formatUI(g.name)}</h1>
            <span className="badge">{richUI("m_9fea8201753a", [formatUI(g.member_count), formatUI(g.total_assets)])}</span>
            {formatUI(g.is_system && <span className="status">{textUI("m_1f43948106d1")}</span>)}
          </div>
          {formatUI(g.description && <p className="page__sub">{formatUI(g.description)}</p>)}
        </div>
        {formatUI(!g.is_system && can(me, 'asset.write') && (<button type="button" className="btn btn--secondary btn--auto" onClick={() => setEditing(true)}>{richUI("m_12a64771a7ec", [<Pencil size={15} aria-hidden="true"/>])}</button>))}
      </div>

      <section className="panel" aria-labelledby="rules-h">
        <h2 id="rules-h" className="panel__title">{textUI("m_4228aeb07c41")}</h2>
        {formatUI(g.rule.filters.length === 0 ? <p className="muted">{textUI("m_c0c4c1e6ea43")}</p> : (<>
            <p className="muted small">{richUI("m_f9fdab22817a", [formatUI(g.rule.mode === 'or' ? 'any' : 'all')])}</p>
            <ul className="plain-list">{formatUI(g.rule.filters.map((f, i) => <li key={i}>{formatUI(describeFilter(f))}</li>))}</ul>
          </>))}
      </section>

      <section className="list" aria-labelledby="members-h" tabIndex={0}>
        <h2 id="members-h" className="panel__title list__title">{textUI("m_1044a4c056d0")}</h2>
        {formatUI(g.members.length === 0 ? (<div className="state"><h2>{textUI("m_afb39be14afc")}</h2><p>{textUI("m_c08c17e0083c")}</p></div>) : (<ul className="list__body">
            {formatUI(g.members.map((m) => {
                const Icon = categoryOf(m.category).icon;
                return (<li key={m.id} className="member">
                  <span className="asset-row__icon"><Icon size={16}/></span>
                  <Link className="member__name mono" to={`/assets/${m.id}`}>{formatUI(m.name)}</Link>
                  <span className="muted small">{formatUI(m.platform ? (PLATFORM_LABEL[m.platform] ?? m.platform) : '—')}{formatUI(m.arch ? ` · ${m.arch}` : '')}</span>
                  <span className={`crit crit--${m.criticality}`}>{formatUI(m.criticality[0]!.toUpperCase() + m.criticality.slice(1))}</span>
                </li>);
            }))}
          </ul>))}
        {formatUI(g.members_capped && <p className="muted small list__note">{richUI("m_131fb27eb3a0", [formatUI(g.member_count)])}</p>)}
      </section>

      {formatUI(!g.is_system && can(me, 'asset.delete') && (<section className="danger" aria-labelledby="gdanger-h">
          <div><h2 id="gdanger-h" className="panel__title">{textUI("m_fd8b8dae4421")}</h2><p className="muted">{textUI("m_de85dcb16628")}</p></div>
          <button type="button" className="btn btn--danger btn--auto" onClick={() => setConfirming(true)}>{richUI("m_bf34e4a9fe3c", [<Trash2 size={15} aria-hidden="true"/>])}</button>
        </section>))}
      {formatUI(editing && <GroupDialog group={g} onClose={() => setEditing(false)} onSaved={() => setEditing(false)} onStale={() => { setEditing(false); q.refetch(); }}/>)}
      {formatUI(confirming && (<ConfirmDialog title={textUI("m_dc6c5ae4c245", [g.name])} body="The group will be removed. Its assets stay untouched." confirmLabel="Delete group" busy={del.isPending} error={del.isError ? describeError(del.error).message : undefined} onClose={() => setConfirming(false)} onConfirm={() => del.mutate(undefined, { onSuccess: () => navigate('/assets/groups', { replace: true }) })}/>))}
    </section>);
}

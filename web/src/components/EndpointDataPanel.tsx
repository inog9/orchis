import { t as textUI, tr as richUI, useLocale, locale, fv as formatUI } from "../i18n/runtime";
import { useState } from 'react';
import { Modal } from './Modal';
import { Link, useNavigate } from 'react-router-dom';
import { useAssetEndpoint, useDeleteEndpointData, useMe } from '../api/hooks';
import { can } from '../api/types';
import { defenseJob, useLaunchDefense, useLatestDefenseScan } from '../api/defense';
import { describeError } from './ErrorNote';
import { StatusBadge } from './sims/StatusBadge';
import { DefenseResult } from './DefenseResult';
import './defense.css';

const when = (iso: string | null | undefined) => { if (!iso) return '—'; const d = new Date(iso); return Number.isNaN(d.getTime()) ? '—' : d.toLocaleString(locale(), { dateStyle: 'medium', timeStyle: 'short' }); };

// DefenseBot summary of one enrolled asset. The raw inventory is never shown: the result is what was found, not what is installed.
export function EndpointDataPanel({ assetID, canRun, allowed, canDelete = false, authorized = true }: {
    assetID: string;
    canRun: boolean;
    allowed: boolean;
    canDelete?: boolean;
    authorized?: boolean;
    // Kept for the organisation page that embeds this panel; it no longer changes what is shown.
    inOrganization?: boolean;
}) {
    useLocale();
    const navigate = useNavigate();
    const { data: me } = useMe();
    const q = useAssetEndpoint(assetID, allowed);
    const remove = useDeleteEndpointData(assetID);
    const orgID = q.data?.organization_id;
    const history = useLatestDefenseScan(assetID, orgID, allowed && can(me, 'simulation.read') && can(me, 'assessment.read'));
    const launch = useLaunchDefense(orgID, assetID);
    const [confirmDelete, setConfirmDelete] = useState(false);
    const data = q.data;
    const job = defenseJob(history.scan);
    const summary = job?.defense;
    const running = history.latest?.status === 'running';
    const collected = (data?.items ?? []).map((d) => d.collected_at).filter((v): v is string => !!v).sort().at(-1);
    const unsupported = data?.supported === false && data.has_agent;
    // Why the button is off is always said in words next to it.
    const blocked = !data ? '' : !data.has_agent ? textUI("m_def_no_agent") : unsupported ? textUI("m_def_unsupported") : !orgID ? textUI("m_def_need_org") : !authorized ? textUI("m_def_need_auth") : '';
    const start = () => launch.mutate(undefined, { onSuccess: (s) => navigate(`/simulations/${s.id}`) });
    return (<section className="panel ed-panel" aria-labelledby="ed-asset-h">
      <div className="def-panel__head">
        <div><h2 id="ed-asset-h" className="panel__title">{textUI("m_def_title")}</h2><p className="muted small">{textUI("m_def_sub")}</p></div>
        {formatUI(allowed && data && canRun && (<div className="def-panel__actions">
          <button type="button" className="btn btn--primary btn--auto" disabled={!!blocked || launch.isPending || running} onClick={start}>{formatUI(launch.isPending ? textUI("m_def_starting") : textUI("m_def_run"))}</button>
        </div>))}
      </div>
      {formatUI(!allowed ? <p className="muted">{textUI("m_d0cecb60890e")}</p> : q.isPending ? <span className="skeleton"/> : q.isError || !data ? <p role="alert">{formatUI(describeError(q.error).message)}</p> : (<>
        {formatUI(blocked && <p className="muted" role="status">{formatUI(blocked)}{formatUI(!data.has_agent && <> <Link to="/assets/agents">{textUI("m_703e4ff0e367")}</Link></>)}</p>)}
        {formatUI(launch.isError && <div className="alert" role="alert"><span>{formatUI(describeError(launch.error).message)}</span></div>)}
        {formatUI(history.detail.isError && <p role="alert">{formatUI(describeError(history.detail.error).message)}</p>)}
        {formatUI(!history.latest && !history.list.isPending && <p className="muted">{textUI("m_def_never")}</p>)}
        {formatUI(history.latest && (<div aria-live="polite">
          <p className="def-meta"><strong>{textUI("m_def_last")}</strong> <StatusBadge status={history.latest.status}/> <span className="muted small">{when(history.latest.created_at)}</span>
            <Link to={`/simulations/${history.latest.id}`}>{textUI("m_def_open_scan")}</Link>
            {formatUI(history.scan?.report_id && can(me, 'report.read') && <Link to={`/reports?assessment_id=${history.latest.id}`}>{textUI("m_def_open_report")}</Link>)}</p>
          {formatUI(summary && <DefenseResult summary={summary}/>)}
        </div>))}
        {formatUI(collected && <p className="muted small">{textUI("m_def_collected", [when(collected)])}</p>)}
        {formatUI(canDelete && !data.read_only && data.has_agent && <p><button className="btn btn--secondary btn--auto" type="button" onClick={() => setConfirmDelete(true)}>{textUI("m_e39b83b0b4b3")}</button></p>)}
        {formatUI(remove.isError && <p role="alert">{formatUI(describeError(remove.error).message)}</p>)}
        {formatUI(confirmDelete && <Modal labelledBy="delete-endpoint-data-title" onClose={() => setConfirmDelete(false)} width={480}><h2 id="delete-endpoint-data-title" className="dialog__title">{textUI("m_c3623ff5d9f4")}</h2><p>{textUI("m_6562710a3dbf")}</p><footer className="dialog__foot"><button className="btn btn--secondary btn--auto" type="button" onClick={() => setConfirmDelete(false)}>{textUI("m_19766ed6ccb2")}</button><button className="btn btn--primary btn--auto" type="button" disabled={remove.isPending} onClick={() => remove.mutate(data.version, { onSuccess: () => setConfirmDelete(false) })}>{formatUI(remove.isPending ? textUI("m_43b5894c3e4e") : textUI("m_3566e099aa40"))}</button></footer></Modal>)}
      </>))}
    </section>);
}

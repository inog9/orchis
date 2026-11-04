import { t as textUI, tr as richUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { ChevronRight, Plus, X } from 'lucide-react';
import { useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { ApiError } from '../api/client';
import { useAgents, useCreateEnrollmentToken, useEnrollmentStatus, useMe, useRevokeAgent } from '../api/hooks';
import { can, type AgentRow, type EnrollmentToken } from '../api/types';
import { AgentControls, AgentRollout } from '../components/AgentControls';
import { AssetTabs } from '../components/AssetTabs';
import { CopyButton } from '../components/CopyButton';
import { describeError } from '../components/ErrorNote';
import { Modal } from '../components/Modal';
import { when } from '../components/RunBadges';
import { installCommand, OS_LABEL } from '../lib/agentInstall';
import { ConfirmDialog } from './ConfirmDialog';
function validBase(v: string): boolean {
    try {
        const u = new URL(v);
        return (u.protocol === 'https:' || u.protocol === 'http:') && !u.username && !u.password;
    }
    catch {
        return false;
    }
}
export function InstallDialog({ onClose, organizationID }: {
    onClose: () => void;
    organizationID?: string;
}) {
    useLocale();
    const create = useCreateEnrollmentToken();
    const [os, setOs] = useState<'linux' | 'macos' | 'windows'>('linux');
    const [arch, setArch] = useState<'x86_64' | 'arm64'>('x86_64');
    const [mode, setMode] = useState<'service' | 'user'>('service');
    const [instanceName, setInstanceName] = useState('');
    const [base, setBase] = useState(window.location.origin);
    const [token, setToken] = useState<EnrollmentToken | null>(null);
    const status = useEnrollmentStatus(token?.id);
    const err = create.error;
    const fieldErr = (n: string) => (err instanceof ApiError ? err.fields.find((f) => f.name === n)?.message : undefined);
    const validName = !instanceName || /^[a-z0-9][a-z0-9_-]{0,39}$/.test(instanceName);
    const cmd = token ? installCommand(base, token, instanceName) : '';
    const connected = status.data?.used && status.data.agent;
    return (<Modal labelledBy="install-title" onClose={onClose} width={720}>
      <>
        <header className="dialog__head"><h2 id="install-title">{textUI("m_703e4ff0e367")}</h2><button type="button" className="icon-btn" aria-label={textUI("m_7d9eb7acb13e")} onClick={onClose}><X size={18}/></button></header>
        {formatUI(!token ? (<form className="dialog__form" noValidate onSubmit={(e) => { e.preventDefault(); if (validBase(base) && validName)
            create.mutate({ os, arch, mode, organization_id: organizationID }, { onSuccess: setToken }); }}>
            <p className="notice">{richUI("m_cdda26534247", [formatUI(organizationID ? textUI("m_d002540f1716") : textUI("m_f9087c87dec5"))])}</p><p className="subtitle">{textUI("m_0fc265db6ba6")}</p>
            <div className="field"><label htmlFor="agent-instance">{textUI("m_45d7b207f58c")}</label><div className="field__input"><input id="agent-instance" value={instanceName} pattern="[a-z0-9][a-z0-9_-]{0,39}" maxLength={40} onChange={e => setInstanceName(e.target.value)}/></div><p className="field__hint">{textUI("m_55edd0e3dc10")}</p>{formatUI(!validName && <p role="alert" className="field__error">{textUI("m_cad35085b2d1")}</p>)}</div>
            <div className="row">
              <div className="select"><label htmlFor="ag-os">{textUI("m_0fcabfe6e097")}</label>
                <select id="ag-os" value={os} onChange={(e) => setOs(e.target.value as typeof os)}><option value="linux">{textUI("m_4828e60247c1")}</option><option value="macos">{textUI("m_aed6b7aa2a05")}</option><option value="windows">{textUI("m_d598026a9cbc")}</option></select>
                {formatUI(fieldErr('os') && <small className="field__error" role="alert">{formatUI(fieldErr('os'))}</small>)}</div>
              <div className="select"><label htmlFor="ag-arch">{textUI("m_cd74053c5481")}</label>
                <select id="ag-arch" value={arch} onChange={(e) => setArch(e.target.value as typeof arch)}><option value="x86_64">{textUI("m_7520b5a1b312")}</option><option value="arm64">{textUI("m_f69162950f23")}</option></select></div>
              <div className="select"><label htmlFor="ag-mode">{textUI("m_3dbf75bd3204")}</label>
                <select id="ag-mode" value={mode} onChange={(e) => setMode(e.target.value as typeof mode)}><option value="service">{textUI("m_d053262b548e")}</option><option value="user">{textUI("m_73cb20ee0cd9")}</option></select></div>
            </div>
            <div className="field"><div className="field__label-row"><label htmlFor="ag-base" className="field__label">{textUI("m_2f4c57b2b6ff")}</label></div>
              <div className={`field__input ${base && !validBase(base) ? 'is-error' : ''}`}><input id="ag-base" value={base} onChange={(e) => setBase(e.target.value)} aria-invalid={!validBase(base)}/></div>
              {formatUI(!validBase(base) ? <p className="field__error" role="alert">{textUI("m_c7d45a4ef752")}</p> : <p className="field__hint">{textUI("m_9b67c49b1738")}</p>)}</div>
            {formatUI(err && !(err instanceof ApiError && err.status === 422) && <div className="alert" role="alert"><strong>{textUI("m_6383b4474571")}</strong><span>{formatUI(describeError(err).message)}</span></div>)}
            <footer className="dialog__foot"><button type="button" className="btn btn--secondary btn--auto" onClick={onClose}>{textUI("m_19766ed6ccb2")}</button>
              <button type="submit" className="btn btn--primary btn--auto" disabled={create.isPending || !validBase(base) || !validName}>{formatUI(create.isPending ? textUI("m_c79ed9492e3c") : textUI("m_a111cde11b72"))}</button></footer>
          </form>) : (<>
            <p className="subtitle">{richUI("m_40ac8452138e", [formatUI(OS_LABEL[token.os]), formatUI(token.arch), formatUI(token.os === 'windows' ? ' (PowerShell)' : ''), formatUI(when(token.expires_at))])}</p>
            <div className="secret secret--block"><pre className="mono cmd" tabIndex={0} data-testid="install-command">{cmd}</pre><CopyButton value={cmd} label={textUI("m_9a01feecae67")}/></div>
            <p className="muted small">{richUI("m_00d33ddb0723", [formatUI(token.version), formatUI(token.sha256.slice(0, 12))])}</p>
            <div className={`notice ${connected ? 'notice--ok' : ''}`} role="status" aria-live="polite">
              {formatUI(connected ? <><strong>{textUI("m_c64cafff1fc2")}</strong> <span>{richUI("m_9cee97951126", [formatUI(status.data!.agent!.hostname ?? status.data!.agent!.asset_name)])}</span></>
                : status.data?.expired ? <strong>{textUI("m_cbe4390f87a3")}</strong>
                    : <><strong>{textUI("m_974cb00c2f75")}</strong> <span>{textUI("m_7bd2d3bfa002")}</span></>)}
            </div>
            <footer className="dialog__foot">
              {formatUI(connected && <Link className="btn btn--secondary btn--auto" to={`/assets/${status.data!.agent!.asset_id}`} onClick={onClose}>{textUI("m_8ded2e1b284e")}</Link>)}
              <button type="button" className="btn btn--primary btn--auto" onClick={onClose}>{formatUI(connected ? textUI("m_11a6767d5674") : textUI("m_7d9eb7acb13e"))}</button>
            </footer>
          </>))}
      </>
    </Modal>);
}
const STATUS_TONE = { online: 'status--ok', offline: 'status--bad', revoked: 'status--bad' } as const;
const STATUS_LABEL = { get online() {
        return textUI("m_0d21bd52022c");
    }, get offline() {
        return textUI("m_a1794783aab7");
    }, get revoked() {
        return textUI("m_f6f738d04392");
    } } as const;
export function AgentsPage() {
    useLocale();
    const { data: me } = useMe();
    const [params, setParams] = useSearchParams();
    const focused = params.get('agent');
    const q = useAgents();
    const revoke = useRevokeAgent();
    const [installing, setInstalling] = useState(false);
    const [managed, setManaged] = useState<string | null>(focused);
    const [revoking, setRevoking] = useState<AgentRow | null>(null);
    const manage = can(me, 'agent.manage');
    const controlManage = manage && can(me, 'settings.manage');
    const items = (q.data ?? []).filter(a => !focused || a.id === focused);
    return (<section className="page" aria-labelledby="agents-title">
      {formatUI(focused && <p className="notice">{richUI("m_28f79590df6b", [<button className="link-quiet" onClick={() => setParams({})}>{textUI("m_9afcda319cd0")}</button>])}</p>)}
      <div className="page__head">
        <div>
          <nav className="crumbs" aria-label={textUI("m_2bd873d6c734")}><span>{textUI("m_bd12731d7bc9")}</span><ChevronRight size={12} aria-hidden="true"/><span className="crumbs__current">{textUI("m_279b44d2ab4b")}</span></nav>
          <div className="page__title-row"><h1 id="agents-title" className="page__title">{textUI("m_279b44d2ab4b")}</h1>{formatUI(q.data && <span className="badge">{formatUI(items.filter((a) => a.status !== 'revoked').length)}</span>)}</div>
          <p className="page__sub">{textUI("m_8877ffa70604")}</p>
        </div>
        {formatUI(manage && <button type="button" className="btn btn--primary btn--auto" onClick={() => setInstalling(true)}>{richUI("m_65f5583497cb", [<Plus size={16} aria-hidden="true"/>])}</button>)}
      </div>
      <AssetTabs />
      {formatUI(controlManage && <AgentRollout />)}
      {formatUI(q.isPending ? <span className="skeleton"/> : q.isError ? (<div className="state state--error" role="alert"><h2>{textUI("m_df27d178c541")}</h2><p>{richUI("m_19636dc199da", [formatUI(describeError(q.error).message)])}</p><button type="button" className="btn btn--secondary btn--auto" onClick={() => q.refetch()}>{textUI("m_d8b8392e2c54")}</button></div>) : items.length === 0 ? (<div className="state"><h2>{textUI("m_971b61f8cb4d")}</h2><p>{textUI("m_9b4ca8e1191c")}</p>
          {formatUI(manage && <button type="button" className="btn btn--primary btn--auto" onClick={() => setInstalling(true)}>{textUI("m_4998a219ec87")}</button>)}</div>) : (<div className="list agent-list" role="table" aria-label={textUI("m_279b44d2ab4b")} tabIndex={0}>
          <div className="list__head agent-row" role="row"><span role="columnheader">{textUI("m_1f4bc0693926")}</span><span role="columnheader">{textUI("m_6f38f66d9a24")}</span><span role="columnheader">{textUI("m_7b60b8e351cb")}</span><span role="columnheader">{textUI("m_8c2e4a035f5f")}</span><span role="columnheader">{textUI("m_1413c453e48b")}</span><span role="columnheader"><span className="sr-only">{textUI("m_ff8059dc6752")}</span></span></div>
          {formatUI(items.map((a) => (<div key={a.id} role="rowgroup" data-agent-id={a.id}><div className="asset-row agent-row" role="row">
              <span role="cell" className="asset-row__text"><Link className="asset-row__name" to={`/assets/${a.asset_id}`}>{formatUI(a.asset_name)}</Link><span className="muted small">{formatUI(a.mode === 'service' ? textUI("m_d677190e0a99") : textUI("m_1572da5fec5e"))}</span></span>
              <span role="cell">{formatUI(OS_LABEL[a.os])} · {formatUI(a.arch)}</span>
              <span role="cell"><span className="mono">{formatUI(a.version)}</span>{formatUI(a.update_available && <span className="muted small">{textUI("m_9d96fbdc7ff2")}</span>)}</span>
              <span role="cell"><span className={`status ${STATUS_TONE[a.status]}`}>{formatUI(STATUS_LABEL[a.status])}</span>{formatUI(a.paused && a.status !== 'revoked' && <span className="muted small">{textUI("m_961dace9826f")}</span>)}</span>
              <span role="cell" className="muted small">{formatUI(a.status === 'revoked' ? '—' : when(a.last_heartbeat_at))}</span>
              <span role="cell" className="row-actions" style={{ flexWrap: 'wrap' }}>{formatUI(controlManage && <button type="button" className="btn btn--secondary btn--sm" aria-expanded={managed === a.id} aria-controls={`agent-controls-${a.id}`} onClick={(e) => { setManaged(managed === a.id ? null : a.id); e.currentTarget.closest('.list')?.scrollTo({ left: 0 }); }}>{textUI("m_5a23444828db")}</button>)}{formatUI(manage && a.status !== 'revoked' && <button type="button" className="btn btn--danger btn--sm" aria-label={textUI("m_2edbc27e7ec3", [a.asset_name])} onClick={() => setRevoking(a)}>{textUI("m_87e6d00bbf53")}</button>)}</span>
            </div>{formatUI(controlManage && managed === a.id && <div role="row" id={`agent-controls-${a.id}`}><div role="cell" aria-colspan={6} style={{ padding: 20, borderBottom: '1px solid var(--border)' }}><h2 className="small" style={{ overflowWrap: 'anywhere' }}>{formatUI(a.instance_name || a.asset_name)}</h2><AgentControls agent={a}/></div></div>)}</div>)))}
        </div>))}
      {formatUI(installing && <InstallDialog onClose={() => setInstalling(false)}/>)}
      {formatUI(revoking && (<ConfirmDialog title={textUI("m_61932431e816", [revoking.asset_name])} confirmLabel="Revoke agent" busy={revoke.isPending} body="It stops working right away, anything it was running is canceled, and this can’t be undone. To use that endpoint again, install a new agent." error={revoke.isError ? describeError(revoke.error).message : undefined} onConfirm={() => revoke.mutate(revoking.id, { onSuccess: () => setRevoking(null) })} onClose={() => { revoke.reset(); setRevoking(null); }}/>))}
    </section>);
}

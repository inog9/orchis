import { ChevronRight, Plus, X } from 'lucide-react';
import { useState } from 'react';
import { Link } from 'react-router-dom';
import { ApiError } from '../api/client';
import { useAgents, useCreateEnrollmentToken, useEnrollmentStatus, useMe, useRevokeAgent } from '../api/hooks';
import { can, type AgentRow, type EnrollmentToken } from '../api/types';
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
  } catch {
    return false;
  }
}

export function InstallDialog({ onClose, organizationID }: { onClose: () => void; organizationID?: string }) {
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

  return (
    <Modal labelledBy="install-title" onClose={onClose} width={720}>
      <>
        <header className="dialog__head"><h2 id="install-title">Install an agent</h2><button type="button" className="icon-btn" aria-label="Close" onClick={onClose}><X size={18} /></button></header>
        {!token ? (
          <form className="dialog__form" noValidate onSubmit={(e) => { e.preventDefault(); if (validBase(base) && validName) create.mutate({ os, arch, mode, organization_id: organizationID }, { onSuccess: setToken }); }}>
            <p className="notice">{organizationID?'This agent and its asset will belong to this organisation.':'This agent creates an unassigned asset. To include it in a company report, install it from that organisation or assign its asset afterward.'} Installation does not authorize scans or set trusted target addresses.</p><p className="subtitle">The agent runs checks on the endpoint itself and only ever connects out to this platform. Pick the endpoint’s platform to get a one-time install command.</p>
            <div className="field"><label htmlFor="agent-instance">Agent instance name (optional)</label><div className="field__input"><input id="agent-instance" value={instanceName} pattern="[a-z0-9][a-z0-9_-]{0,39}" maxLength={40} onChange={e => setInstanceName(e.target.value)} /></div><p className="field__hint">Use a different name and installation directory for each instance on the same machine. Use lowercase letters, digits, underscores or hyphens; begin with a letter or digit (40 characters maximum).</p>{!validName&&<p role="alert" className="field__error">Choose a valid instance name before creating the command.</p>}</div>
            <div className="row">
              <div className="select"><label htmlFor="ag-os">Operating system</label>
                <select id="ag-os" value={os} onChange={(e) => setOs(e.target.value as typeof os)}><option value="linux">Linux</option><option value="macos">macOS</option><option value="windows">Windows</option></select>
                {fieldErr('os') && <small className="field__error" role="alert">{fieldErr('os')}</small>}</div>
              <div className="select"><label htmlFor="ag-arch">Architecture</label>
                <select id="ag-arch" value={arch} onChange={(e) => setArch(e.target.value as typeof arch)}><option value="x86_64">x86_64</option><option value="arm64">arm64</option></select></div>
              <div className="select"><label htmlFor="ag-mode">Run as</label>
                <select id="ag-mode" value={mode} onChange={(e) => setMode(e.target.value as typeof mode)}><option value="service">Service (starts at boot)</option><option value="user">Current user (foreground)</option></select></div>
            </div>
            <div className="field"><div className="field__label-row"><label htmlFor="ag-base" className="field__label">Platform address the endpoint can reach</label></div>
              <div className={`field__input ${base && !validBase(base) ? 'is-error' : ''}`}><input id="ag-base" value={base} onChange={(e) => setBase(e.target.value)} aria-invalid={!validBase(base)} /></div>
              {!validBase(base) ? <p className="field__error" role="alert">Enter a full address such as https://orchis.example.com.</p> : <p className="field__hint">Use a public or internal name the endpoint can resolve, not “localhost”, unless you’re testing on this machine.</p>}</div>
            {err && !(err instanceof ApiError && err.status === 422) && <div className="alert" role="alert"><strong>Couldn’t create the command</strong><span>{describeError(err).message}</span></div>}
            <footer className="dialog__foot"><button type="button" className="btn btn--secondary btn--auto" onClick={onClose}>Cancel</button>
              <button type="submit" className="btn btn--primary btn--auto" disabled={create.isPending || !validBase(base) || !validName}>{create.isPending ? 'Creating…' : 'Create install command'}</button></footer>
          </form>
        ) : (
          <>
            <p className="subtitle">Run this on the {OS_LABEL[token.os]} {token.arch} endpoint{token.os === 'windows' ? ' (PowerShell)' : ''}. The token works once and expires {when(token.expires_at)}. It won’t be shown again.</p>
            <div className="secret secret--block"><pre className="mono cmd" data-testid="install-command">{cmd}</pre><CopyButton value={cmd} label="Copy command" /></div>
            <p className="muted small">Agent version {token.version}. The command checks the file’s SHA-256 ({token.sha256.slice(0, 12)}…) before running it.</p>
            <div className={`notice ${connected ? 'notice--ok' : ''}`} role="status" aria-live="polite">
              {connected ? <><strong>Agent connected.</strong> <span>{status.data!.agent!.hostname ?? status.data!.agent!.asset_name} is now an asset. Authorize it for testing before running checks on it.</span></>
                : status.data?.expired ? <strong>This token expired. Create a new command.</strong>
                : <><strong>Waiting for the agent…</strong> <span>This turns green as soon as it connects.</span></>}
            </div>
            <footer className="dialog__foot">
              {connected && <Link className="btn btn--secondary btn--auto" to={`/assets/${status.data!.agent!.asset_id}`} onClick={onClose}>Open the asset</Link>}
              <button type="button" className="btn btn--primary btn--auto" onClick={onClose}>{connected ? 'Done' : 'Close'}</button>
            </footer>
          </>
        )}
      </>
    </Modal>
  );
}

const STATUS_TONE = { online: 'status--ok', offline: 'status--bad', revoked: 'status--bad' } as const;
const STATUS_LABEL = { online: 'Online', offline: 'Offline', revoked: 'Revoked' } as const;

export function AgentsPage() {
  const { data: me } = useMe();
  const q = useAgents();
  const revoke = useRevokeAgent();
  const [installing, setInstalling] = useState(false);
  const [revoking, setRevoking] = useState<AgentRow | null>(null);
  const manage = can(me, 'agent.manage');
  const items = q.data ?? [];
  return (
    <section className="page" aria-labelledby="agents-title">
      <div className="page__head">
        <div>
          <nav className="crumbs" aria-label="Breadcrumb"><span>Assets</span><ChevronRight size={12} aria-hidden="true" /><span className="crumbs__current">Agents</span></nav>
          <div className="page__title-row"><h1 id="agents-title" className="page__title">Agents</h1>{q.data && <span className="badge">{items.filter((a) => a.status !== 'revoked').length}</span>}</div>
          <p className="page__sub">Agents run read-only checks on the endpoint itself. They only connect out to the platform; nothing connects in.</p>
        </div>
        {manage && <button type="button" className="btn btn--primary btn--auto" onClick={() => setInstalling(true)}><Plus size={16} aria-hidden="true" /> Install agent</button>}
      </div>
      <AssetTabs />
      {q.isPending ? <span className="skeleton" /> : q.isError ? (
        <div className="state state--error" role="alert"><h2>Couldn’t load agents</h2><p>{describeError(q.error).message} Your data is safe.</p><button type="button" className="btn btn--secondary btn--auto" onClick={() => q.refetch()}>Try again</button></div>
      ) : items.length === 0 ? (
        <div className="state"><h2>No agents yet</h2><p>Install an agent on an endpoint to run checks there, such as listing its listening ports.</p>
          {manage && <button type="button" className="btn btn--primary btn--auto" onClick={() => setInstalling(true)}>Install the first agent</button>}</div>
      ) : (
        <div className="list" role="table" aria-label="Agents" tabIndex={0}>
          <div className="list__head agent-row" role="row"><span role="columnheader">ENDPOINT</span><span role="columnheader">PLATFORM</span><span role="columnheader">VERSION</span><span role="columnheader">STATUS</span><span role="columnheader">LAST SEEN</span><span role="columnheader"><span className="sr-only">Actions</span></span></div>
          {items.map((a) => (
            <div key={a.id} className="asset-row agent-row" role="row">
              <span role="cell" className="asset-row__text"><Link className="asset-row__name" to={`/assets/${a.asset_id}`}>{a.asset_name}</Link><span className="muted small">{a.mode === 'service' ? 'Service' : 'Current user'}</span></span>
              <span role="cell">{OS_LABEL[a.os]} · {a.arch}</span>
              <span role="cell"><span className="mono">{a.version}</span>{a.update_available && <span className="muted small"> · update pending</span>}</span>
              <span role="cell"><span className={`status ${STATUS_TONE[a.status]}`}>{STATUS_LABEL[a.status]}</span></span>
              <span role="cell" className="muted small">{a.status === 'revoked' ? '—' : when(a.last_heartbeat_at)}</span>
              <span role="cell" className="row-actions">{manage && a.status !== 'revoked' && <button type="button" className="btn btn--danger btn--sm" aria-label={`Revoke agent on ${a.asset_name}`} onClick={() => setRevoking(a)}>Revoke</button>}</span>
            </div>
          ))}
        </div>
      )}
      {installing && <InstallDialog onClose={() => setInstalling(false)} />}
      {revoking && (
        <ConfirmDialog title={`Revoke the agent on ${revoking.asset_name}?`} confirmLabel="Revoke agent" busy={revoke.isPending}
          body="It stops working right away, anything it was running is canceled, and this can’t be undone. To use that endpoint again, install a new agent."
          error={revoke.isError ? describeError(revoke.error).message : undefined}
          onConfirm={() => revoke.mutate(revoking.id, { onSuccess: () => setRevoking(null) })} onClose={() => { revoke.reset(); setRevoking(null); }} />
      )}
    </section>
  );
}

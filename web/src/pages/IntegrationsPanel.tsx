import { KeyRound, X } from 'lucide-react';
import { useState } from 'react';
import { useIntegrations, useMe, useRotateIntegrationToken, useSetIntegrationEnabled } from '../api/hooks';
import { can, type IntegrationRow } from '../api/types';
import { CopyButton } from '../components/CopyButton';
import { describeError } from '../components/ErrorNote';
import { Modal } from '../components/Modal';
import { when } from '../components/RunBadges';
import { ConfirmDialog } from './ConfirmDialog';

const KIND = { injector: 'Injector', collector: 'Collector', executor: 'Executor' } as const;

function TokenDialog({ secret, name, onClose }: { secret: string; name: string; onClose: () => void }) {
  return (
    <Modal labelledBy="tok-title" onClose={onClose} width={560}>
      <>
        <header className="dialog__head"><h2 id="tok-title">New token for {name}</h2><button type="button" className="icon-btn" aria-label="Close" onClick={onClose}><X size={18} /></button></header>
        <p className="subtitle">Give this to the {name} injector as <code className="mono">ORCHIS_INJECTOR_TOKEN</code>. The previous token stopped working. It won’t be shown again.</p>
        <div className="secret"><code className="mono" data-testid="injector-token">{secret}</code><CopyButton value={secret} /></div>
        <footer className="dialog__foot"><button type="button" className="btn btn--primary btn--auto" onClick={onClose}>Done</button></footer>
      </>
    </Modal>
  );
}

export function IntegrationsPanel() {
  const { data: me } = useMe();
  const q = useIntegrations();
  const toggle = useSetIntegrationEnabled();
  const rotate = useRotateIntegrationToken();
  const [secret, setSecret] = useState<{ name: string; token: string } | null>(null);
  const [confirm, setConfirm] = useState<{ kind: 'rotate' | 'disable'; i: IntegrationRow } | null>(null);
  const manage = can(me, 'integration.manage');

  if (q.isPending) return <span className="skeleton" />;
  if (q.isError) return <div className="state state--error" role="alert"><h2>Couldn’t load integrations</h2><p>{describeError(q.error).message}</p></div>;
  return (
    <div className="stack">
      <p className="muted">Injectors run scans on behalf of the platform. They check in regularly; one that hasn’t been heard from for a minute shows as offline.</p>
      {(toggle.isError || rotate.isError) && <div className="alert" role="alert"><strong>That didn’t work</strong><span>{describeError(toggle.error ?? rotate.error).message}</span></div>}
      <ul className="cards">
        {q.data!.map((i) => (
          <li key={i.id} className="card">
            <div className="card__top"><strong className="card__name">{i.name}</strong>
              <span className={`status ${i.status === 'online' ? 'status--ok' : i.status === 'disabled' ? 'status--bad' : ''}`}>{i.is_demo ? 'Synthetic · ' : ''}{i.status === 'online' ? 'Running' : i.status === 'disabled' ? 'Disabled' : 'Offline'}</span></div>
            <p className="card__rule">{KIND[i.kind]}{i.software_version ? ` · v${i.software_version}` : ''} · {i.is_demo ? 'illustrative state; no worker connected' : i.last_heartbeat_at ? `last seen ${when(i.last_heartbeat_at)}` : 'never connected'}</p>
            {i.description && <p className="muted small">{i.description}</p>}
            <ul className="loglist" aria-label={`${i.name} recent log`}>{i.logs.length === 0 ? <li className="muted small">No log lines yet.</li> : i.logs.map((l, k) => <li key={k} className={`log--${l.level}`}><time>{when(l.at)}</time> {l.message}</li>)}</ul>
            {manage && !i.is_demo && (
              <div className="row-actions">
                {i.kind === 'injector' && <button type="button" className="btn btn--secondary btn--sm" onClick={() => setConfirm({ kind: 'rotate', i })}><KeyRound size={14} aria-hidden="true" /> {i.has_token ? 'Rotate token' : 'Create token'}</button>}
                {i.enabled ? <button type="button" className="btn btn--secondary btn--sm" onClick={() => setConfirm({ kind: 'disable', i })}>Disable</button>
                  : <button type="button" className="btn btn--primary btn--sm" disabled={toggle.isPending} onClick={() => toggle.mutate({ id: i.id, enabled: true })}>Enable</button>}
              </div>
            )}
          </li>
        ))}
      </ul>
      {confirm?.kind === 'rotate' && (
        <ConfirmDialog title={`${confirm.i.has_token ? 'Rotate' : 'Create'} the ${confirm.i.name} token?`} confirmLabel={confirm.i.has_token ? 'Rotate token' : 'Create token'} busy={rotate.isPending}
          body={confirm.i.has_token ? 'The running injector stops working until you give it the new token.' : 'You get a token to give to the injector.'}
          onConfirm={() => rotate.mutate(confirm.i.id, { onSuccess: (r) => { setConfirm(null); setSecret({ name: confirm.i.name, token: r.token }); } })} onClose={() => setConfirm(null)} />
      )}
      {confirm?.kind === 'disable' && (
        <ConfirmDialog title={`Disable ${confirm.i.name}?`} confirmLabel="Disable" busy={toggle.isPending}
          body="Scans that need it can’t start until you enable it again. Work already queued waits."
          onConfirm={() => toggle.mutate({ id: confirm.i.id, enabled: false }, { onSuccess: () => setConfirm(null) })} onClose={() => setConfirm(null)} />
      )}
      {secret && <TokenDialog name={secret.name} secret={secret.token} onClose={() => setSecret(null)} />}
    </div>
  );
}

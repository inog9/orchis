import { agentDisplayName } from '../lib/engines';
import { t as textUI, tr as richUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { KeyRound, X } from 'lucide-react';
import { useState } from 'react';
import { useIntegrations, useMe, useRotateIntegrationToken, useSetIntegrationEnabled } from '../api/hooks';
import { can, type IntegrationRow } from '../api/types';
import { CopyButton } from '../components/CopyButton';
import { describeError } from '../components/ErrorNote';
import { Modal } from '../components/Modal';
import { when } from '../components/RunBadges';
import { ConfirmDialog } from './ConfirmDialog';
import './settings.css';
const KIND = { get injector() {
        return textUI("m_08da4e9c9cb3");
    }, get collector() {
        return textUI("m_9d0081a4f3d6");
    }, get executor() {
        return textUI("m_bb186757b4bf");
    } } as const;
function TokenDialog({ secret, name, onClose }: {
    secret: string;
    name: string;
    onClose: () => void;
}) {
    useLocale();
    return (<Modal labelledBy="tok-title" onClose={onClose} width={560}>
      <>
        <header className="dialog__head"><h2 id="tok-title">{richUI("m_2e313445c67c", [formatUI(name)])}</h2><button type="button" className="icon-btn" aria-label={textUI("m_7d9eb7acb13e")} onClick={onClose}><X size={18}/></button></header>
        <p className="subtitle">{richUI("m_88dcd2085fac", [formatUI(name), <code className="mono">ORCHIS_INJECTOR_TOKEN</code>])}</p>
        <div className="secret"><code className="mono" data-testid="injector-token">{secret}</code><CopyButton value={secret}/></div>
        <footer className="dialog__foot"><button type="button" className="btn btn--primary btn--auto" onClick={onClose}>{textUI("m_11a6767d5674")}</button></footer>
      </>
    </Modal>);
}
export function IntegrationsPanel() {
    useLocale();
    const { data: me } = useMe();
    const q = useIntegrations();
    const toggle = useSetIntegrationEnabled();
    const rotate = useRotateIntegrationToken();
    const [secret, setSecret] = useState<{
        name: string;
        token: string;
    } | null>(null);
    const [confirm, setConfirm] = useState<{
        kind: 'rotate' | 'disable';
        i: IntegrationRow;
    } | null>(null);
    const manage = can(me, 'integration.manage');
    if (q.isPending)
        return <span className="skeleton"/>;
    if (q.isError)
        return <div className="state state--error" role="alert"><h2>{textUI("m_992d9a30276f")}</h2><p>{formatUI(describeError(q.error).message)}</p></div>;
    return (<div className="stack settings">
      <p className="muted">{textUI("m_2320612213b3")}</p>
      {formatUI((toggle.isError || rotate.isError) && <div className="alert" role="alert"><strong>{textUI("m_a12d349fae9f")}</strong><span>{formatUI(describeError(toggle.error ?? rotate.error).message)}</span></div>)}
      <div className="set-int" role="table" aria-label={textUI("m_090512d93fcc")}>
        <div className="set-int__row set-int__head" role="row"><span role="columnheader">{textUI("m_set_col_integration")}</span><span role="columnheader">{textUI("m_set_col_kind")}</span><span role="columnheader">{textUI("m_set_col_status")}</span><span role="columnheader">{textUI("m_set_col_heartbeat")}</span><span role="columnheader">{textUI("m_set_col_actions")}</span></div>
        {formatUI(q.data!.map((i) => (<div key={i.id} className="set-int__item" role="rowgroup">
            <div className="set-int__row" role="row">
              <span role="cell" className="set-int__name"><strong>{formatUI(agentDisplayName(i.name))}</strong>{formatUI(i.description && <small className="muted">{formatUI(agentDisplayName(i.description))}</small>)}</span>
              <span role="cell">{formatUI(KIND[i.kind])}{formatUI(i.software_version ? ` · v${i.software_version}` : '')}</span>
              <span role="cell"><span className={`status ${i.status === 'online' ? 'status--ok' : i.status === 'disabled' ? 'status--bad' : ''}`}>{formatUI(i.is_demo ? textUI("m_4bb44254b234") : '')}{formatUI(i.status === 'online' ? textUI("m_f4ccae29e1bb") : i.status === 'disabled' ? textUI("m_75081b593d15") : textUI("m_a1794783aab7"))}</span></span>
              <span role="cell" className="muted small">{formatUI(i.is_demo ? textUI("m_dc18eb08e77e") : i.last_heartbeat_at ? textUI("m_5786f589c75f", [when(i.last_heartbeat_at)]) : textUI("m_aed2a2a0b59f"))}</span>
              <span role="cell" className="row-actions">
                {formatUI(manage && !i.is_demo && (<>
                  {formatUI(i.kind === 'injector' && <button type="button" className="btn btn--secondary btn--sm" onClick={() => setConfirm({ kind: 'rotate', i })}><KeyRound size={14} aria-hidden="true"/> {formatUI(i.has_token ? textUI("m_4ade788283fe") : textUI("m_5d8e8e30bc4f"))}</button>)}
                  {formatUI(i.enabled ? <button type="button" className="btn btn--secondary btn--sm" onClick={() => setConfirm({ kind: 'disable', i })}>{textUI("m_b7e3e4aa4257")}</button>
                      : <button type="button" className="btn btn--primary btn--sm" disabled={toggle.isPending} onClick={() => toggle.mutate({ id: i.id, enabled: true })}>{textUI("m_5342e09f2729")}</button>)}
                </>))}
              </span>
            </div>
            <div role="row"><div role="cell" aria-colspan={5}><ul className="loglist" aria-label={textUI("m_28b0874e62ba", [agentDisplayName(i.name)])}>{formatUI(i.logs.length === 0 ? <li className="muted small">{textUI("m_691cfdb915e2")}</li> : i.logs.map((l, k) => <li key={k} className={`log--${l.level}`}><time>{formatUI(when(l.at))}</time> {formatUI(agentDisplayName(l.message))}</li>))}</ul></div></div>
          </div>)))}
      </div>
      {formatUI(confirm?.kind === 'rotate' && (<ConfirmDialog title={textUI("m_1ed751e8da1f", [confirm.i.has_token ? textUI("m_c3613b1704f5") : textUI("m_4759498ac2a7"), agentDisplayName(confirm.i.name)])} confirmLabel={confirm.i.has_token ? 'Rotate token' : 'Create token'} busy={rotate.isPending} body={confirm.i.has_token ? 'The running injector stops working until you give it the new token.' : 'You get a token to give to the injector.'} onConfirm={() => rotate.mutate(confirm.i.id, { onSuccess: (r) => { setConfirm(null); setSecret({ name: agentDisplayName(confirm.i.name), token: r.token }); } })} onClose={() => setConfirm(null)}/>))}
      {formatUI(confirm?.kind === 'disable' && (<ConfirmDialog title={textUI("m_79d2bdcc9f73", [agentDisplayName(confirm.i.name)])} confirmLabel="Disable" busy={toggle.isPending} body="Scans that need it can’t start until you enable it again. Work already queued waits." onConfirm={() => toggle.mutate({ id: confirm.i.id, enabled: false }, { onSuccess: () => setConfirm(null) })} onClose={() => setConfirm(null)}/>))}
      {formatUI(secret && <TokenDialog name={secret.name} secret={secret.token} onClose={() => setSecret(null)}/>)}
    </div>);
}

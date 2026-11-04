import { t as textUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { Modal } from '../components/Modal';
export function ConfirmDialog({ title, body, confirmLabel, busy, error, onConfirm, onClose }: {
    title: string;
    body: string;
    confirmLabel: string;
    busy?: boolean;
    error?: string;
    onConfirm: () => void;
    onClose: () => void;
}) {
    useLocale();
    return (<Modal labelledBy="confirm-title" onClose={onClose} width={440}>
      <h2 id="confirm-title" className="dialog__title">{formatUI(title)}</h2>
      <p className="subtitle">{formatUI(body)}</p>
      {formatUI(error && <div className="alert" role="alert"><strong>{textUI("m_a59364b3e9e0")}</strong><span>{formatUI(error)}</span></div>)}
      <footer className="dialog__foot">
        <button type="button" className="btn btn--secondary btn--auto" onClick={onClose}>{textUI("m_19766ed6ccb2")}</button>
        <button type="button" className="btn btn--danger btn--auto" onClick={onConfirm} disabled={busy}>{formatUI(busy ? textUI("m_5474eef8d0f1") : confirmLabel)}</button>
      </footer>
    </Modal>);
}

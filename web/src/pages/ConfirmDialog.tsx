import { Modal } from '../components/Modal';

export function ConfirmDialog({ title, body, confirmLabel, busy, error, onConfirm, onClose }: {
  title: string; body: string; confirmLabel: string; busy?: boolean; error?: string; onConfirm: () => void; onClose: () => void;
}) {
  return (
    <Modal labelledBy="confirm-title" onClose={onClose} width={440}>
      <h2 id="confirm-title" className="dialog__title">{title}</h2>
      <p className="subtitle">{body}</p>
      {error && <div className="alert" role="alert"><strong>Couldn’t finish</strong><span>{error}</span></div>}
      <footer className="dialog__foot">
        <button type="button" className="btn btn--secondary btn--auto" onClick={onClose}>Cancel</button>
        <button type="button" className="btn btn--danger btn--auto" onClick={onConfirm} disabled={busy}>{busy ? 'Working…' : confirmLabel}</button>
      </footer>
    </Modal>
  );
}

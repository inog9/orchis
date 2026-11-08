import { t as textUI, useLocale, fv as formatUI } from "../../i18n/runtime";
import { statusTone } from '../../lib/scanView';
import './badge.css';

export function scanStatusLabel(status: string): string {
    switch (status) {
        case 'running': return textUI("m_sim_s_running");
        case 'finished': case 'done': return textUI("m_sim_s_finished");
        case 'incomplete': return textUI('hackbot_incomplete');
        case 'failed': return textUI("m_sim_s_failed");
        case 'canceled': return textUI("m_sim_s_canceled");
        case 'waiting': return textUI("m_sim_s_waiting");
        case 'skipped': return textUI("m_sim_s_skipped");
        case 'ready': return textUI("m_rep_s_ready");
        case 'generating': case 'pending': return textUI("m_rep_s_generating");
        default: return status;
    }
}
// Status selalu berteks (bukan hanya warna); titik hanya hiasan. Teks penuh tersedia lewat title bila terpotong.
export function StatusBadge({ status }: { status: string }) {
    useLocale();
    const label = scanStatusLabel(status);
    return <span className={`sim-badge sim-badge--${statusTone(status)}`} title={label}><i aria-hidden="true"/><span className="sim-badge__txt">{formatUI(label)}</span></span>;
}

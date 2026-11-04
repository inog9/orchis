import {language as selectedLanguage} from './runtime';
// Event codes are stable. Runtime data and raw diagnostics are never translated as markup.
export const notificationEN = {
 scan_finished: 'Scan finished', scan_failed: 'Scan finished with failed or skipped checks', scan_stopped: 'Scan was stopped',
 report_ready: 'Report PDF is ready', pdf_failed: 'Report PDF failed; retry is available', agent_offline: 'Agent is offline',
 agent_revoked: 'Agent credentials revoked', agent_update_failed: 'Agent update failed',
 authorization_pending: 'Asset authorization needs administrator review', consent_pending: 'Endpoint consent needs administrator renewal',
};
export const notificationID: Record<keyof typeof notificationEN, string> = {
 scan_finished: 'Scan selesai', scan_failed: 'Scan selesai dengan pemeriksaan gagal atau dilewati', scan_stopped: 'Scan dihentikan',
 report_ready: 'PDF laporan siap', pdf_failed: 'PDF laporan gagal; dapat dicoba kembali', agent_offline: 'Agent luring',
 agent_revoked: 'Kredensial agent dicabut', agent_update_failed: 'Pembaruan agent gagal',
 authorization_pending: 'Otorisasi aset menunggu tinjauan admin', consent_pending: 'Persetujuan endpoint perlu diperbarui oleh admin',
};
export function notificationTitle(kind: string, fallback: string, language: 'en' | 'id' = selectedLanguage()) {
 const dict = language === 'id' ? notificationID : notificationEN;
 return dict[kind as keyof typeof notificationEN] ?? fallback;
}

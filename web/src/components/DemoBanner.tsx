import { t as textUI, tr as richUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { useState } from 'react';
import { X } from 'lucide-react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/client';
import { useMe } from '../api/hooks';
import { can } from '../api/types';
import { describeError } from './ErrorNote';
import { Modal } from './Modal';
export type DemoStatus = {
    has_demo: boolean;
    cleanup_pending?: number;
    counts: Record<string, number>;
    seed?: number;
};
// Polling cukup jarang: status demo jarang berubah di luar aksi purge yang dilakukan dari banner ini sendiri.
const POLL_MS = 45000;
function useDemoStatus(enabled: boolean) {
    return useQuery({ queryKey: ['demo', 'status'], enabled, queryFn: () => api.get<DemoStatus>('/api/v1/demo/status'), refetchInterval: (query) => query.state.data?.cleanup_pending ? 2000 : POLL_MS, staleTime: POLL_MS, retry: false });
}
function usePurgeDemo() {
    const qc = useQueryClient();
    return useMutation({
        mutationFn: () => api.post<DemoStatus>('/api/v1/demo/purge', {}),
        onSuccess: (status) => {
            qc.setQueryData(['demo', 'status'], status);
            // Purge dapat menyentuh hampir semua domain (aset, temuan, skenario, dll); segarkan semua query aktif.
            qc.invalidateQueries();
        },
    });
}
// Hanya objek yang bermakna bagi pengguna; tabel internal (audit_log, tags) tidak ditampilkan.
const COUNT_LABELS: () => [
    string,
    string
][] = () => [
    ['organizations', textUI("m_4471bc3dace3")],
    ['assets', textUI("m_8bf729ffe074")],
    ['agents', textUI("m_8c70b25cbbbe")],
    ['findings', textUI("m_8aa3c4ce3ccd")],
    ['assessments', textUI("m_f887c6af08da")],
    ['reports', textUI("m_7f26104f77a7")],
    ['scenarios', textUI("m_d1a1c362651a")],
    ['simulations', textUI("m_46a3151f7ba8")],
    ['alerts', textUI("m_deb3e366c077")],
    ['users', textUI("m_c16a3826a6b8")],
    ['teams', textUI("m_fadff6cf8575")],
    ['asset_groups', textUI("m_546363c223ee")],
    ['atomic_tests', textUI("m_fb1be9513f4d")],
    ['ai_jobs', textUI("m_93f0480558c6")]
];
const DEMO_DISMISS_KEY = 'orchis.demo.banner';
// Banner demo bisa diciutkan jadi strip tipis; pilihan diingat (AUDIT UI-023).
function demoDismissed(): boolean {
    try { return localStorage.getItem(DEMO_DISMISS_KEY) === '1'; } catch { return false; }
}
function describeCounts(counts: Record<string, number>): string {
    const parts = COUNT_LABELS().filter(([key]) => (counts[key] ?? 0) > 0).map(([key, label]) => `${counts[key]} ${label}`);
    return parts.length > 0 ? parts.join(', ') : textUI("m_a7b605aae185");
}
export function DemoBanner() {
    useLocale();
    const { data: me } = useMe();
    const status = useDemoStatus(can(me, 'organization.read'));
    const purge = usePurgeDemo();
    const [confirming, setConfirming] = useState(false);
    const [slim, setSlim] = useState(demoDismissed);
    if (!status.data?.has_demo && !status.data?.cleanup_pending)
        return null;
    const { counts, seed } = status.data!;
    const cleanupPending = !status.data!.has_demo && (status.data!.cleanup_pending ?? 0) > 0;
    const summary = describeCounts(counts);
    const canPurge = can(me, 'settings.manage');
    const closeConfirm = () => {
        if (purge.isPending)
            return;
        purge.reset();
        setConfirming(false);
    };
    const dismiss = () => { try { localStorage.setItem(DEMO_DISMISS_KEY, '1'); } catch { /* abaikan */ } setSlim(true); };
    if (slim && !cleanupPending) return (<div className="notice demo-banner demo-banner--slim" role="status" aria-label={textUI("m_ca3b29c75bb1")}>
      <strong>{textUI("m_71ddd59aa6f1")}</strong>
      <span className="muted">{textUI("m_a7b605aae185")}</span>
      {formatUI(canPurge && <button type="button" className="link-quiet demo-banner__action" onClick={() => setConfirming(true)} disabled={purge.isPending}>{textUI("m_39fe66bad2d4")}</button>)}
      <button type="button" className="link-quiet" aria-label={textUI("m_fnd_close")} onClick={dismiss}><X size={14} aria-hidden="true"/></button>
    </div>);
    return (<div className="notice demo-banner" role="status" aria-label={textUI("m_ca3b29c75bb1")}>
      <div className="demo-banner__body">
        <strong>{formatUI(cleanupPending ? textUI("m_c6dd96e18456") : textUI("m_71ddd59aa6f1"))}</strong>
        <span>
          {formatUI(' ')}
          {formatUI(cleanupPending ? textUI("m_5565bd6cfdd7") : <>{textUI("m_0f4b7d83a0dc")}{formatUI(summary)}){formatUI(seed !== undefined ? textUI("m_3ec773cbe978", [seed]) : '')}{textUI("m_d20acc76216e")}</>)}
        </span>
      </div>
      {formatUI(cleanupPending && purge.isError && <div className="alert" role="alert">{formatUI(describeError(purge.error).message)}</div>)}
      {formatUI(canPurge && (<button type="button" className="btn btn--secondary btn--sm demo-banner__action" onClick={() => cleanupPending ? purge.mutate() : setConfirming(true)} disabled={purge.isPending}>
          {formatUI(purge.isPending ? textUI("m_347a864f14b1") : cleanupPending ? textUI("m_6100a8a01208") : textUI("m_39fe66bad2d4"))}
        </button>))}
      {!cleanupPending && <button type="button" className="link-quiet" aria-label={textUI("m_fnd_close")} onClick={dismiss}><X size={14} aria-hidden="true"/></button>}
      {formatUI(confirming && (<Modal labelledBy="demo-purge-title" onClose={closeConfirm} width={440}>
          <>
            <h2 id="demo-purge-title" className="dialog__title">{textUI("m_11a4764b8580")}</h2>
            <p className="subtitle">{richUI("m_b7a98070a727", [formatUI(summary)])}</p>
            {formatUI(purge.isError && (<div className="alert" role="alert">
                <strong>{textUI("m_3ceb4f8a6218")}</strong>
                <span>{formatUI(describeError(purge.error).message)}</span>
              </div>))}
            <footer className="dialog__foot">
              <button type="button" className="btn btn--secondary btn--auto" onClick={closeConfirm} disabled={purge.isPending}>{textUI("m_19766ed6ccb2")}</button>
              <button type="button" className="btn btn--danger btn--auto" disabled={purge.isPending} onClick={() => purge.mutate(undefined, { onSuccess: () => setConfirming(false) })}>
                {formatUI(purge.isPending ? textUI("m_347a864f14b1") : cleanupPending ? textUI("m_6100a8a01208") : textUI("m_39fe66bad2d4"))}
              </button>
            </footer>
          </>
        </Modal>))}
    </div>);
}

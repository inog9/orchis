import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/client';
import { useMe } from '../api/hooks';
import { can } from '../api/types';
import { describeError } from './ErrorNote';
import { Modal } from './Modal';

export type DemoStatus = { has_demo: boolean; cleanup_pending?: number; counts: Record<string, number>; seed?: number };

// Polling cukup jarang: status demo jarang berubah di luar aksi purge yang dilakukan dari banner ini sendiri.
const POLL_MS = 45_000;

function useDemoStatus() {
  return useQuery({ queryKey: ['demo', 'status'], queryFn: () => api.get<DemoStatus>('/api/v1/demo/status'), refetchInterval: (query) => query.state.data?.cleanup_pending ? 2000 : POLL_MS, staleTime: POLL_MS, retry: false });
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
const COUNT_LABELS: [string, string][] = [
  ['organizations', 'organisations'],
  ['assets', 'assets'],
  ['agents', 'agents'],
  ['findings', 'findings'],
  ['assessments', 'assessments'],
  ['reports', 'reports'],
  ['scenarios', 'scenarios'],
 ['simulations', 'synthetic simulations'],
  ['alerts', 'alerts'],
  ['users', 'disabled demo accounts'],
  ['teams', 'teams'],
  ['asset_groups', 'asset groups'],
  ['atomic_tests', 'saved tests'],
 ['ai_jobs', 'synthetic AI requests'],
];

function describeCounts(counts: Record<string, number>): string {
  const parts = COUNT_LABELS.filter(([key]) => (counts[key] ?? 0) > 0).map(([key, label]) => `${counts[key]} ${label}`);
  return parts.length > 0 ? parts.join(', ') : 'no items';
}

export function DemoBanner() {
  const { data: me } = useMe();
  const status = useDemoStatus();
  const purge = usePurgeDemo();
  const [confirming, setConfirming] = useState(false);

  if (!status.data?.has_demo && !status.data?.cleanup_pending) return null;
  const { counts, seed } = status.data!;
  const cleanupPending = !status.data!.has_demo && (status.data!.cleanup_pending ?? 0) > 0;
  const summary = describeCounts(counts);
  const canPurge = can(me, 'settings.manage');

  const closeConfirm = () => {
    if (purge.isPending) return;
    purge.reset();
    setConfirming(false);
  };

  return (
    <div className="notice demo-banner" role="status" aria-label="Demo data notice">
      <div className="demo-banner__body">
        <strong>{cleanupPending ? 'Demo cleanup pending' : 'Demo data'}</strong>
        <span>
          {' '}
          {cleanupPending ? 'Demo records have been removed. Storage or search cleanup is still pending; check those services and retry cleanup.' : <>This workspace contains synthetic demo data ({summary}){seed !== undefined ? ` (seed ${seed})` : ''}. It is for exploring the product and is not evidence of any real security issue.</>}
        </span>
      </div>
      {cleanupPending && purge.isError && <div className="alert" role="alert">{describeError(purge.error).message}</div>}
      {canPurge && (
        <button type="button" className="btn btn--secondary btn--sm demo-banner__action" onClick={() => cleanupPending ? purge.mutate() : setConfirming(true)} disabled={purge.isPending}>
          {purge.isPending ? 'Purging…' : cleanupPending ? 'Retry demo cleanup' : 'Purge demo data'}
        </button>
      )}
      {confirming && (
        <Modal labelledBy="demo-purge-title" onClose={closeConfirm} width={440}>
          <>
            <h2 id="demo-purge-title" className="dialog__title">
              Purge demo data?
            </h2>
            <p className="subtitle">
              This permanently removes all synthetic demo data ({summary}) from this workspace. It does not affect real data you created yourself. This can’t be undone.
            </p>
            {purge.isError && (
              <div className="alert" role="alert">
                <strong>Couldn’t purge demo data</strong>
                <span>{describeError(purge.error).message}</span>
              </div>
            )}
            <footer className="dialog__foot">
              <button type="button" className="btn btn--secondary btn--auto" onClick={closeConfirm} disabled={purge.isPending}>
                Cancel
              </button>
              <button
                type="button"
                className="btn btn--danger btn--auto"
                disabled={purge.isPending}
                onClick={() => purge.mutate(undefined, { onSuccess: () => setConfirming(false) })}
              >
                {purge.isPending ? 'Purging…' : cleanupPending ? 'Retry demo cleanup' : 'Purge demo data'}
              </button>
            </footer>
          </>
        </Modal>
      )}
    </div>
  );
}

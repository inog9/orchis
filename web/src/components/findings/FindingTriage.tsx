import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { RotateCcw } from 'lucide-react';
import { t as textUI, useLocale, locale, fv as formatUI } from '../../i18n/runtime';
import { api } from '../../api/client';
import { useMe } from '../../api/hooks';
import { can, type Finding } from '../../api/types';
import { describeError } from '../ErrorNote';
import type { Scan } from '../../api/scans';

const STATUSES = ['open', 'accepted_risk', 'false_positive', 'fixed_pending'] as const;
type TriageStatus = typeof STATUSES[number];
const label = (s: TriageStatus) => ({
    open: textUI('flow_triage_open'),
    accepted_risk: textUI('flow_triage_accepted'),
    false_positive: textUI('flow_triage_false'),
    fixed_pending: textUI('flow_triage_fixed'),
})[s];

// Triage per finding dan retest: keduanya hanya untuk yang punya izin finding.write / assessment.run.
export function FindingTriage({ finding, onChanged }: { finding: Finding; onChanged: (f: Finding) => void }) {
    useLocale();
    const { data: me } = useMe();
    const navigate = useNavigate();
    const qc = useQueryClient();
    const [status, setStatus] = useState<TriageStatus>((finding.triage_status as TriageStatus) || 'open');
    const [reason, setReason] = useState(finding.triage_reason ?? '');
    const [expires, setExpires] = useState('');
    const [note, setNote] = useState('');
    const triage = useMutation({
        mutationFn: () => api.put<Finding>(`/api/v1/findings/${finding.id}/triage`, {
            status,
            reason: status === 'open' ? '' : reason,
            expires_at: status === 'accepted_risk' && expires ? new Date(expires).toISOString() : undefined,
        }),
        onSuccess: async (f) => {
            setNote('');
            onChanged(f);
            await qc.invalidateQueries({ queryKey: ['findings'] });
        },
    });
    const retest = useMutation({
        mutationFn: () => api.post<Scan>(`/api/v1/findings/${finding.id}/retest`, {}),
        onSuccess: async (s) => {
            await Promise.all(['scans', 'home'].map((k) => qc.invalidateQueries({ queryKey: [k] })));
            navigate(`/simulations/${s.id}`);
        },
    });
    if (!can(me, 'finding.write') && !can(me, 'assessment.run')) return null;
    return (<section className="fnd-triage" aria-labelledby="fnd-triage-h">
      <h3 id="fnd-triage-h">{textUI('flow_triage')}</h3>
      {formatUI(finding.triage_status && finding.triage_status !== 'open' && (<p className="def-meta">
        <span className="def-chip def-chip--muted">{formatUI(label(finding.triage_status as TriageStatus))}</span>
        {formatUI(finding.triage_expires_at && <span>{formatUI(textUI('flow_triage_expires'))}: {new Date(finding.triage_expires_at).toLocaleDateString(locale())}</span>)}
      </p>))}
      {formatUI(can(me, 'finding.write') && (<div className="fnd-triage__form">
        <label className="select"><span className="sr-only">{textUI('flow_triage_status')}</span>
          <select aria-label={textUI('flow_triage_status')} value={status} onChange={(e) => setStatus(e.target.value as TriageStatus)}>
            {STATUSES.map((s) => <option key={s} value={s}>{formatUI(label(s))}</option>)}
          </select>
        </label>
        {status !== 'open' && (<>
          <label className="fnd-triage__reason"><span className="sr-only">{textUI('flow_triage_reason')}</span>
            <input type="text" value={reason} onChange={(e) => setReason(e.target.value)} placeholder={textUI('flow_triage_reason')} maxLength={1000}/>
          </label>
          {status === 'accepted_risk' && <label className="fnd-triage__reason"><span className="sr-only">{textUI('flow_triage_expires')}</span>
            <input type="date" value={expires} min={new Date().toISOString().slice(0, 10)} onChange={(e) => setExpires(e.target.value)} aria-label={textUI('flow_triage_expires')}/>
          </label>}
        </>)}
        <button type="button" className="btn btn--secondary btn--auto" disabled={triage.isPending} onClick={() => triage.mutate()}>{textUI('flow_triage_save')}</button>
      </div>))}
      {formatUI(triage.error && <p className="alert" role="alert">{formatUI(describeError(triage.error).message)}</p>)}
      {formatUI(can(me, 'assessment.run') && finding.asset_id && (<p className="fnd-triage__retest">
        <button type="button" className="btn btn--secondary btn--auto" disabled={retest.isPending} onClick={() => retest.mutate()} title={textUI('flow_retest_hint')}>
          <RotateCcw size={14} aria-hidden="true"/> {formatUI(retest.isPending ? textUI('m_bbe5fc3b9ef3') : textUI('flow_retest'))}
        </button>
      </p>))}
      {formatUI(retest.error && <p className="alert" role="alert">{formatUI(describeError(retest.error).message)}</p>)}
      {formatUI(note && <p role="status">{note}</p>)}
    </section>);
}

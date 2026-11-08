import { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { t as textUI, useLocale, fv as formatUI } from '../i18n/runtime';
import { api } from '../api/client';
import { useMe } from '../api/hooks';
import { can, type Asset } from '../api/types';
import { useScans, type Scan, type ScanInput } from '../api/scans';
import { useReadiness, type ReadinessItem } from '../api/readiness';
import { describeError } from './ErrorNote';
import { kindLabel, toolsFor, type EngineKind } from '../lib/engines';
import { localizeReason } from '../lib/readinessReason';

// Checklist kesiapan per aset: satu tempat untuk melihat prasyarat kedua engine dan memulai run.
export function AssetReadiness({ asset }: { asset: Asset }) {
    useLocale();
    const { data: me } = useMe();
    const navigate = useNavigate();
    const qc = useQueryClient();
    const org = asset.organization_id ?? '';
    const readiness = useReadiness(org, !!org && can(me, 'asset.read'));
    const item: ReadinessItem | undefined = readiness.data?.items.find((i) => i.id === asset.id);
    const agentState = item?.agent_status ?? (asset.agent_state === 'online' ? 'online' : asset.agent_state === 'offline' ? 'offline' : 'none');
    const supported = agentState === 'online' && !(item?.defense_reason ?? '').startsWith('DefenseBot supports');
    const consented = agentState === 'online' && supported && !(item?.defense_reason ?? '').startsWith('Collection');
    const hasURL = !!(item?.url ?? asset.addresses.find((d) => d.kind === 'url' && d.scan_approved !== false && /^https?:\/\//i.test(d.value))?.value);
    const windowOpen = readiness.data?.scan_window_open !== false;
    const emergency = readiness.data?.emergency_stop === true;
    const authorized = asset.testing_authorized;
    const webOK = authorized && hasURL;
    const defOK = authorized && agentState === 'online' && supported && consented;
    const history = useScans(org, '', !!org && can(me, 'assessment.read') && can(me, 'simulation.read') && can(me, 'asset.read') && can(me, 'organization.read'));
    const mine = (history.data?.items ?? []).filter((s) => (s.jobs ?? []).some((j) => j.asset_id === asset.id));
    const [busy, setBusy] = useState<EngineKind | ''>('');
    const launch = useMutation({
        mutationFn: (kind: EngineKind) => {
            setBusy(kind);
            return api.postIdempotent<Scan>('/api/v1/scans', {
                language: undefined,
                organization_id: org,
                asset_ids: [asset.id],
                tools: toolsFor(kind),
            } as ScanInput, crypto.randomUUID());
        },
        onSuccess: async (s) => {
            setBusy('');
            await Promise.all(['scans', 'home', 'readiness'].map((k) => qc.invalidateQueries({ queryKey: [k] })));
            navigate(`/simulations/${s.id}`);
        },
        onError: () => setBusy(''),
    });
    const steps: { label: string; done: boolean; why?: string; fix?: string }[] = [
        { label: textUI('flow_chk_step_org'), done: !!org, fix: org ? undefined : '/organizations' },
        { label: textUI('flow_chk_step_auth'), done: authorized, why: authorized ? undefined : localizeReason(item?.web_reason), fix: org ? `/organizations/${org}?tab=assets` : undefined },
        { label: textUI('flow_chk_step_agent'), done: agentState === 'online', why: agentState === 'none' ? localizeReason(item?.defense_reason) : agentState === 'offline' ? localizeReason(item?.defense_reason) : undefined, fix: '/assets/agents' },
        { label: textUI('flow_chk_step_platform'), done: agentState !== 'online' || supported, why: agentState === 'online' && !supported ? localizeReason(item?.defense_reason) : undefined },
        { label: textUI('flow_chk_step_consent'), done: agentState !== 'online' || consented, why: agentState === 'online' && !consented ? localizeReason(item?.defense_reason) : undefined, fix: org ? `/organizations/${org}?tab=agents` : undefined },
        { label: textUI('flow_chk_step_url'), done: webOK, why: !webOK ? localizeReason(item?.web_reason) : undefined, fix: '/assets' },
        { label: textUI('flow_chk_step_window'), done: windowOpen, why: !windowOpen ? localizeReason(readiness.data?.control_reason) : undefined },
    ];
    const buttons: { kind: EngineKind; label: string; ok: boolean; why: string }[] = [
        { kind: 'web', label: textUI('flow_chk_run_web'), ok: webOK && windowOpen && !emergency, why: item?.web_reason ?? '' },
        { kind: 'defense', label: textUI('flow_chk_run_def'), ok: defOK && windowOpen && !emergency, why: item?.defense_reason ?? '' },
        { kind: 'full', label: textUI('flow_chk_run_full'), ok: webOK && defOK && windowOpen && !emergency, why: item?.web_reason || item?.defense_reason || '' },
    ];
    return (<section className="panel" aria-labelledby="chk-h">
      <h2 id="chk-h" className="panel__title">{textUI('flow_chk_title')}</h2>
      <ul className="chk-list">
        {steps.map((s) => (<li key={s.label} className="chk-item">
          <span className={`status${s.done ? ' status--ok' : ''}`}>{s.done ? '✓' : '○'}</span>
          <span className="chk-item__label">{formatUI(s.label)}</span>
          {formatUI(!s.done && s.why ? <span className="chk-item__why">{s.why}</span> : null)}
          {formatUI(!s.done && s.fix && <Link className="link-quiet chk-item__fix" to={s.fix}>{textUI('flow_wiz_fix')}</Link>)}
        </li>))}
      </ul>
      <div className="chk-run">
        {buttons.map((b) => (<button key={b.kind} type="button" className="btn btn--secondary btn--auto" disabled={!b.ok || busy !== '' || !can(me, 'assessment.run') || !can(me, 'simulation.run')} title={b.ok ? undefined : b.why} onClick={() => launch.mutate(b.kind)}>
          {formatUI(busy === b.kind ? textUI('flow_wiz_launching') : b.label)}
        </button>))}
      </div>
      {formatUI(launch.error && <p className="alert" role="alert">{formatUI(describeError(launch.error).message)}</p>)}
      <p className="field__hint">
        {formatUI(mine.length === 0 ? textUI('flow_chk_no_runs') : `${textUI('flow_chk_latest')}: ${kindLabel('web')} · ${mine[0]!.status}`)}
        {formatUI(mine.length > 0 && <Link to={`/simulations/${mine[0]!.id}`}>{' '}{textUI('m_sim_open_org')}</Link>)}
      </p>
      <p className="field__hint">
        {formatUI(agentState === 'none' && `${textUI('flow_hint_agent')} `)}
        {formatUI(agentState === 'online' && !consented && `${textUI('flow_hint_consent')} `)}
        {formatUI(mine.length === 0 && textUI('flow_hint_scan'))}
      </p>
    </section>);
}

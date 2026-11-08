import { useEffect, useMemo, useState } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Globe, HardDrive, ShieldCheck } from 'lucide-react';
import { language } from '../i18n/runtime';
import { t as textUI, useLocale, fv as formatUI } from '../i18n/runtime';
import { api } from '../api/client';
import { useAssets, useMe } from '../api/hooks';
import type { Asset } from '../api/types';
import { useScans, type Scan, type ScanInput } from '../api/scans';
import { useScanControl } from '../components/ScanControls';
import { OrganizationFilter } from '../components/OrganizationFilter';
import { can } from '../api/types';
import { describeError } from '../components/ErrorNote';
import { kindLabel, toolsFor, type EngineKind } from '../lib/engines';
import { localizeReason } from '../lib/readinessReason';
import { useReadiness, synthesizedReadiness, type ReadinessItem } from '../api/readiness';
import './newscan.css';

type Row = { asset: Asset; r?: ReadinessItem };

// Wizard empat langkah: Organisasi -> Yang dijalankan -> Sasaran -> Tinjauan.
// Semua aset organisasi selalu tampil; yang belum siap tetap terlihat dengan alasan dan tautan perbaikan.
export function NewScanPage() {
    useLocale();
    const { data: me } = useMe();
    const [params, setParams] = useSearchParams();
    const navigate = useNavigate();
    const qc = useQueryClient();
    const org = params.get('org') ?? '';
    const presetAsset = params.get('asset') ?? '';
    const [kind, setKind] = useState<EngineKind | ''>('');
    const [targets, setTargets] = useState<string[]>([]);
    const [step, setStep] = useState(1);
    const control = useScanControl(org || undefined, !!org);
    const readiness = useReadiness(org, !!org && can(me, 'asset.read'));
    const list = useAssets('', '', org, !!org);
    const allAssets = list.data?.pages.flatMap((p) => p.items) ?? [];
    const rows: Row[] = useMemo(() => {
        const byID = new Map((readiness.data?.items ?? []).map((r) => [r.id, r]));
        return allAssets.map((a) => ({ asset: a, r: byID.get(a.id) ?? synthesizedReadiness(a) }));
    }, [allAssets, readiness.data]);
    const webReady = (r?: ReadinessItem) => !!r && !r.web_reason;
    const defReady = (r?: ReadinessItem) => !!r && !r.defense_reason;
    const eligible = (k: EngineKind | '', r?: ReadinessItem) => k === '' ? false : k === 'web' ? webReady(r) : k === 'defense' ? defReady(r) : webReady(r) && defReady(r);
    const reasonFor = (k: EngineKind | '', r?: ReadinessItem) =>
        !r ? textUI('flow_wiz_empty') : localizeReason(k === 'web' ? r.web_reason : k === 'defense' ? r.defense_reason : r.web_reason || r.defense_reason);
    const fixFor = (k: EngineKind | '', r?: ReadinessItem) => {
        if (!r) return null;
        if (k !== 'defense' && r.agent_status === 'none') return '/assets/agents';
        if (k !== 'web' && r.defense_reason.startsWith('Collection')) return `/organizations/${org}?tab=agents`;
        return null;
    };
    const selected = rows.filter((it) => targets.includes(it.asset.id));
    const allEligible = selected.length > 0 && kind !== '' && selected.every((it) => eligible(kind, it.r));
    const consentOK = kind !== 'web' && selected.length > 0 && selected.every((it) => !it.r?.defense_reason.startsWith('Collection'));
    const signature = JSON.stringify({ org, kind, targets });
    const key = useMemo(() => crypto.randomUUID(), [signature]);
    const launch = useMutation({
        mutationFn: () => api.postIdempotent<Scan>('/api/v1/scans', {
            language: language(),
            organization_id: org,
            asset_ids: targets,
            tools: toolsFor(kind as EngineKind),
        } satisfies ScanInput, key),
        onSuccess: async (s) => {
            await Promise.all(['scans', 'home', 'assessments', 'readiness'].map((k) => qc.invalidateQueries({ queryKey: [k] })));
            navigate(`/simulations/${s.id}`);
        },
    });
    const changeOrg = (value: string, advance = false) => {
        const next = new URLSearchParams(params);
        if (value) next.set('org', value); else next.delete('org');
        next.delete('asset');
        setParams(next);
        setTargets([]);
        setKind('');
        const wasInFlow = step >= 2;
        setStep(value ? (advance ? 2 : wasInFlow ? 2 : 1) : 1);
    };
    // Pra-pilih satu aset saat datang dari halaman aset (?asset=) begitu datanya siap.
    useEffect(() => {
        if (!presetAsset || kind === '' || targets.length > 0) return;
        const row = rows.find((i) => i.asset.id === presetAsset);
        if (row && eligible(kind, row.r)) setTargets([presetAsset]);
    }, [presetAsset, kind, rows, targets.length]);
    const toggle = (id: string) => setTargets((cur) => cur.includes(id) ? cur.filter((v) => v !== id) : [...cur, id]);
    const steps = [textUI('flow_wiz_s1'), textUI('flow_wiz_s2'), textUI('flow_wiz_s3'), textUI('flow_wiz_s4')];
    const windowOpen = control.data?.allowed_now !== false;
    const canLaunch = !!org && kind !== '' && allEligible && windowOpen && can(me, 'assessment.run') && can(me, 'simulation.run') && !launch.isPending;
    const cards: { kind: EngineKind; icon: typeof Globe; desc: string; needs: string; leaves: string; time: string }[] = [
        { kind: 'web', icon: Globe, desc: textUI('flow_wiz_card_web_desc'), needs: textUI('flow_wiz_card_web_needs'), leaves: textUI('flow_wiz_card_web_leaves'), time: textUI('flow_wiz_card_web_time') },
        { kind: 'defense', icon: HardDrive, desc: textUI('flow_wiz_card_def_desc'), needs: textUI('flow_wiz_card_def_needs'), leaves: textUI('flow_wiz_card_def_leaves'), time: textUI('flow_wiz_card_def_time') },
        { kind: 'full', icon: ShieldCheck, desc: textUI('flow_wiz_card_full_desc'), needs: textUI('flow_wiz_card_full_needs'), leaves: textUI('flow_wiz_card_full_leaves'), time: textUI('flow_wiz_card_full_time') },
    ];
    return (<section className="page newscan" aria-labelledby="newscan-title">
      <Link className="sims__back" to="/simulations">{textUI('m_ee68966f9267')}</Link>
      <div className="page__head"><div>
        <p className="eyebrow">{textUI('m_sim_eyebrow')}</p>
        <h1 id="newscan-title" className="page__title">{textUI('flow_wiz_title')}</h1>
        <p className="page__sub">{textUI('flow_wiz_sub')}</p>
      </div></div>
      <ol className="wiz-steps">
        {steps.map((label, i) => (<li key={label} className={`wiz-step${step === i + 1 ? ' is-current' : step > i + 1 ? ' is-done' : ''}`} aria-current={step === i + 1 ? 'step' : undefined}>
          {/* Langkah yang sudah tercapai bisa diklik untuk kembali (AUDIT UI-007). */}
          {step > i + 1
            ? <button type="button" className="wiz-step__back link-quiet" onClick={() => setStep(i + 1)}><span className="wiz-step__n" aria-hidden="true">✓</span><span>{formatUI(label)}</span></button>
            : <><span className="wiz-step__n" aria-hidden="true">{i + 1}</span><span>{formatUI(label)}</span></>}
        </li>))}
      </ol>
      {step === 1 && (<fieldset className="fieldset wiz-pane">
        <legend className="sr-only">{textUI('flow_wiz_s1')}</legend>
        <OrganizationFilter value={org} onChange={(v) => changeOrg(v, true)} required/>
        {formatUI(org && allAssets.length === 0 && !list.isPending && (<div className="state"><h2>{textUI('flow_wiz_empty')}</h2><p role="status">{textUI('flow_wiz_none_eligible')}</p><Link className="btn btn--secondary btn--auto" to={`/organizations/${org}?tab=assets&new=1`}>{textUI('flow_wiz_add_asset')}</Link></div>))}
        {formatUI(org && allAssets.length > 0 && <button type="button" className="btn btn--primary btn--auto" onClick={() => setStep(2)}>{textUI('flow_wiz_next')}</button>)}
      </fieldset>)}
      {formatUI(step >= 2 && (<div className="wiz-orgbar">
        <OrganizationFilter value={org} onChange={(v) => changeOrg(v)} required/>
      </div>))}
      {step === 2 && (<fieldset className="fieldset wiz-pane">
        <legend className="sr-only">{textUI('flow_wiz_s2')}</legend>
        <div className="wiz-cards" role="radiogroup" aria-label={textUI('flow_wiz_s2')}>
          {cards.map((c) => (<button key={c.kind} type="button" role="radio" aria-checked={kind === c.kind} aria-label={kindLabel(c.kind)} className={`wiz-card${kind === c.kind ? ' is-on' : ''}`} onClick={() => setKind(c.kind)}>
            <span className="wiz-card__icon" aria-hidden="true"><c.icon size={18}/></span>
            <span className="wiz-card__body">
              <strong>{formatUI(kindLabel(c.kind))}</strong>
              <span>{formatUI(c.desc)}</span>
              <span className="wiz-card__meta"><b>{formatUI(textUI('flow_wiz_needs'))}:</b> {formatUI(c.needs)}</span>
              <span className="wiz-card__meta"><b>{formatUI(textUI('flow_wiz_leaves'))}:</b> {formatUI(c.leaves)}</span>
              <span className="wiz-card__meta"><b>{formatUI(textUI('flow_wiz_time'))}:</b> {formatUI(c.time)}</span>
            </span>
          </button>))}
        </div>
        <div className="row">
          <button type="button" className="btn btn--secondary btn--auto" onClick={() => setStep(1)}>{textUI('flow_wiz_back')}</button>
          <button type="button" className="btn btn--primary btn--auto" disabled={!kind} onClick={() => setStep(3)}>{textUI('flow_wiz_next')}</button>
        </div>
      </fieldset>)}
      {step === 3 && (<fieldset className="fieldset wiz-pane">
        <legend className="sr-only">{textUI('flow_wiz_s3')}</legend>
        <p className="field__hint">{textUI('flow_wiz_targets_hint')}</p>
        {formatUI(readiness.data && !windowOpen && <p role="alert" className="alert">{formatUI(textUI('flow_wiz_window_closed', [localizeReason(readiness.data.control_reason) || '—']))}</p>)}
        {formatUI(list.isPending && <p role="status">{textUI('m_ba3bbbe10d8b')}</p>)}
        {formatUI(list.error && <p role="alert">{formatUI(describeError(list.error).message)}</p>)}
        {formatUI(org && allAssets.length === 0 && !list.isPending && (<div className="state"><h2>{textUI('flow_wiz_empty')}</h2><Link className="btn btn--secondary btn--auto" to={`/organizations/${org}?tab=assets&new=1`}>{textUI('flow_wiz_add_asset')}</Link></div>))}
        <ul className="wiz-targets">
          {rows.map(({ asset: a, r }) => {
              const ok = kind !== '' && eligible(kind, r) && !a.is_demo && windowOpen;
              const why = kind === '' ? '' : reasonFor(kind, r);
              const fix = fixFor(kind, r);
              return (<li key={a.id} className={`wiz-target${ok ? '' : ' is-off'}`}>
                <label className="check">
                  <input type="checkbox" disabled={!ok} checked={targets.includes(a.id)} onChange={() => toggle(a.id)}/>
                  <span className="check__box" aria-hidden="true"/>
                  <span className="wiz-target__name">{formatUI(a.name)}</span>
                  {formatUI(r && <span className="wiz-chips">
                    <span className={`wiz-chip${webReady(r) ? ' is-on' : ''}`} title={localizeReason(r.web_reason) || undefined}>{formatUI(textUI('flow_wiz_chip_web'))}</span>
                    <span className={`wiz-chip${defReady(r) ? ' is-on' : ''}`} title={localizeReason(r.defense_reason) || undefined}>{formatUI(textUI('flow_wiz_chip_agent'))}</span>
                  </span>)}
                  {formatUI(!ok && why && <span className="wiz-target__why">{formatUI(why)}{formatUI(r?.url && <>{' '}<a className="link-quiet" href={r.url} target="_blank" rel="noreferrer">{textUI('flow_wiz_fix')}</a></>)}{formatUI(fix && <>{' '}<Link className="link-quiet" to={fix}>{textUI('flow_wiz_fix')}</Link></>)}</span>)}
                </label>
              </li>);
          })}
        </ul>
        <p className="field__hint" aria-live="polite">{formatUI(textUI('flow_wiz_selected', [targets.length, allAssets.length]))}</p>
        {formatUI(kind !== '' && rows.length > 0 && targets.length === 0 && <p role="status" className="notice">{textUI('flow_wiz_none_eligible')}</p>)}
        <div className="row">
          <button type="button" className="btn btn--secondary btn--auto" onClick={() => setStep(2)}>{textUI('flow_wiz_back')}</button>
          <button type="button" className="btn btn--primary btn--auto" disabled={targets.length === 0} onClick={() => setStep(4)}>{textUI('flow_wiz_next')}</button>
        </div>
      </fieldset>)}
      {step === 4 && (<fieldset className="fieldset wiz-pane">
        <legend className="sr-only">{textUI('flow_wiz_s4')}</legend>
        <dl className="wiz-review">
          <dt>{textUI('flow_wiz_review_what')}</dt><dd>{formatUI(kind ? kindLabel(kind) : '—')}</dd>
          <dt>{textUI('flow_wiz_review_contact')}</dt><dd>{formatUI(selected.map((it) => it.r?.url || it.asset.name).join(', ') || '—')}</dd>
          <dt>{textUI('flow_wiz_review_leaves')}</dt><dd>{formatUI(kind === 'web' ? textUI('flow_wiz_card_web_leaves') : kind === 'defense' ? textUI('flow_wiz_card_def_leaves') : textUI('flow_wiz_card_full_leaves'))}</dd>
          <dt>{textUI('flow_wiz_review_consent')}</dt><dd>{formatUI(kind === 'web' ? '—' : consentOK ? textUI('flow_wiz_consent_ok') : textUI('flow_wiz_consent_no'))}</dd>
          <dt>{textUI('flow_wiz_review_window')}</dt><dd>{formatUI(windowOpen ? readiness.data?.control_reason ? textUI('flow_wiz_window_closed', [readiness.data.control_reason]) : 'OK' : readiness.data?.control_reason || '—')}</dd>
        </dl>
        {formatUI(launch.error && <p className="alert" role="alert">{formatUI(describeError(launch.error).message)}</p>)}
        <div className="row">
          <button type="button" className="btn btn--secondary btn--auto" onClick={() => setStep(3)}>{textUI('flow_wiz_back')}</button>
          <button type="button" className="btn btn--primary btn--auto" disabled={!canLaunch} onClick={() => launch.mutate()}>{formatUI(launch.isPending ? textUI('flow_wiz_launching') : textUI('flow_wiz_launch'))}</button>
        </div>
      </fieldset>)}
    </section>);
}

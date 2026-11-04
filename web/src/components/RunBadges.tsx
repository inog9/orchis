import { t as textUI, useLocale, locale, fv as formatUI } from "../i18n/runtime";
import type { ExecutionSummary, Finding } from '../api/types';
const OUTCOME_LABEL: Record<string, string> = { get success() {
        return textUI("m_6d9a6f97a5fd");
    }, get error() {
        return textUI("m_031a8f0f659d");
    }, get canceled() {
        return textUI("m_13ca2ee24993");
    }, get timeout() {
        return textUI("m_9718cc761cfd");
    }, get skipped() {
        return textUI("m_12698ce1ea5c");
    } };
export function runLabel(e: ExecutionSummary | null | undefined): string {
    if (!e)
        return textUI("m_3d40a69d3160");
    if (e.state === 'ready')
        return textUI("m_661ff40a07e0");
    if (e.state === 'running')
        return textUI("m_f4ccae29e1bb");
    return OUTCOME_LABEL[e.outcome ?? ''] ?? textUI("m_7804f7a79a9e");
}
export function RunBadge({ run }: {
    run: ExecutionSummary | null | undefined;
}) {
    useLocale();
    const tone = !run ? '' : run.state !== 'ended' ? 'status--live' : run.outcome === 'success' ? 'status--ok' : 'status--bad';
    return <span className={`status ${tone}`}>{formatUI(runLabel(run))}</span>;
}
const SEV_LABEL: Record<Finding['severity'], string> = { get info() {
        return textUI("m_170322a32f3c");
    }, get low() {
        return textUI("m_f793de205ead");
    }, get medium() {
        return textUI("m_8e588cd18774");
    }, get high() {
        return textUI("m_c4ebc6d4a583");
    }, get critical() {
        return textUI("m_427dd2969bd1");
    } };
export function SeverityBadge({ severity }: {
    severity: Finding['severity'];
}) {
    useLocale();
    return <span className={`sev sev--${severity}`}>{formatUI(SEV_LABEL[severity])}</span>;
}
export function when(iso: string | null | undefined): string {
    if (!iso)
        return '—';
    return new Date(iso).toLocaleString(locale(), { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
}

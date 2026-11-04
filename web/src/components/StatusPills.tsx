import {capitalLabel} from '../i18n/display';
import { t as textUI, useLocale, fv as formatUI } from "../i18n/runtime";
import type { Simulation, StepStatus } from '../api/types';
const SIM_LABEL: Record<Simulation['status'], string> = {
    get draft() {
        return textUI("m_ebf12ef47cf5");
    },
    get scheduled() {
        return textUI("m_4724f344c1c0");
    },
    get running() {
        return textUI("m_f4ccae29e1bb");
    },
    get paused() {
        return textUI("m_e159b06187d3");
    },
    get finished() {
        return textUI("m_7804f7a79a9e");
    },
    get canceled() {
        return textUI("m_13ca2ee24993");
    },
    get failed() {
        return textUI("m_031a8f0f659d");
    },
};
export function SimBadge({ status }: {
    status: Simulation['status'];
}) {
    useLocale();
    const tone = status === 'running' ? 'status--live' : status === 'finished' ? 'status--ok' : status === 'failed' || status === 'canceled' ? 'status--bad' : '';
    return <span className={`status ${tone}`}>{formatUI(SIM_LABEL[status])}</span>;
}
const STEP_LABEL: Record<StepStatus, string> = {
    get waiting() {
        return textUI("m_6e293a8c009e");
    },
    get queued() {
        return textUI("m_661ff40a07e0");
    },
    get running() {
        return textUI("m_f4ccae29e1bb");
    },
    get done() {
        return textUI("m_11a6767d5674");
    },
    get failed() {
        return textUI("m_031a8f0f659d");
    },
    get skipped() {
        return textUI("m_12698ce1ea5c");
    },
    get blocked() {
        return textUI("m_18f2a0947f9d");
    },
    get canceled() {
        return textUI("m_13ca2ee24993");
    },
};
export function StepBadge({ status }: {
    status: StepStatus;
}) {
    useLocale();
    const tone = status === 'running' ? 'status--live' : status === 'done' ? 'status--ok' : status === 'failed' || status === 'blocked' || status === 'canceled' ? 'status--bad' : '';
    return <span className={`status ${tone}`}>{formatUI(STEP_LABEL[status] ?? status)}</span>;
}
export function ScenarioSeverity({ severity }: {
    severity: string;
}) {
    useLocale();
    return <span className={`crit crit--${severity}`}>{formatUI(capitalLabel(severity))}</span>;
}
export function elapsed(start: string | null, end: string | null): string {
    if (!start)
        return '—';
    const s = Math.max(0, Math.floor(((end ? new Date(end) : new Date()).getTime() - new Date(start).getTime()) / 1000));
    const h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60), r = s % 60;
    return h > 0 ? textUI("m_d426ce3b6441", [h, String(m).padStart(2, '0')]) : m > 0 ? textUI("m_661f2349cb5d", [m, String(r).padStart(2, '0')]) : textUI("m_c4c041f88832", [r]);
}

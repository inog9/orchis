import type { ScanJob } from '../api/scans';
import { t as textUI } from '../i18n/runtime';

// Kosakata engine v3: pengguna hanya melihat dua engine. Nmap dan Nuclei adalah langkah
// persiapan internal orchis-redteam-agent dan tidak pernah tampil sebagai nama di UI.
export type EngineKind = 'web' | 'defense' | 'full';

export function toolsFor(kind: EngineKind): string[] {
    if (kind === 'web') return ['nmap', 'nuclei', 'hackbot'];
    if (kind === 'defense') return ['defensebot'];
    return ['nmap', 'nuclei', 'hackbot', 'defensebot'];
}

// Nama tampilan untuk teks lama dari branch lain; tidak mengubah identifier tool
// yang tersimpan. DefenseBot -> orchis-defense-agent, Hackbot -> orchis-redteam-agent.
export function agentDisplayName(value: string): string {
    return value.replace(/\bhackbot\b/gi, 'orchis-redteam-agent').replace(/\bdefensebot\b/gi, 'orchis-defense-agent');
}

export function isFullTools(tools: string[] | undefined): boolean {
    return !!tools && tools.includes('hackbot') && tools.includes('defensebot');
}

export function engineLabel(tool: string): string {
    return tool === 'defensebot' ? 'orchis-defense-agent' : 'orchis-redteam-agent';
}

// Label engine untuk riwayat scan: "nmap + nuclei + hackbot" ditampilkan sebagai "orchis-redteam-agent".
// Nama engine yang benar menurut alat yang dipilih: hanya orchis-defense-agent -> "orchis-defense-agent", hanya jalur web -> "orchis-redteam-agent".
export function engineSetLabel(tools: string[] | undefined): string {
    const list = tools ?? ['nmap', 'nuclei', 'hackbot'];
    if (isFullTools(list)) return textUI('flow_eng_full_sub');
    if (list.includes('defensebot') && !list.includes('hackbot')) return 'orchis-defense-agent';
    return 'orchis-redteam-agent';
}

export function kindLabel(kind: EngineKind): string {
    if (kind === 'web') return textUI('flow_eng_web');
    if (kind === 'defense') return textUI('flow_eng_defense');
    return textUI('flow_eng_full');
}

// Satu baris per engine untuk detail scan: job nmap/nuclei/hackbot dilebur menjadi engine orchis-redteam-agent
// dengan tahap berbahasa manusia; defensebot menjadi engine orchis-defense-agent.
export type EngineRow = {
    engine: 'orchis-redteam-agent' | 'orchis-defense-agent';
    stage: string;
    status: string;
    jobs: ScanJob[];
    findings: number;
    error: string;
};

function hackbotStage(jobs: ScanJob[], reportReady: boolean): string {
    const anyRunning = jobs.some((j) => j.status === 'running');
    const anyFailed = jobs.some((j) => j.status === 'failed');
    const allDone = jobs.every((j) => j.status === 'finished' || j.status === 'incomplete' || j.status === 'failed' || j.status === 'canceled');
    if (anyFailed && !anyRunning) return '';
    if (allDone) return reportReady ? textUI('m_rep_s_ready') : textUI('flow_stage_writing');
    if (anyRunning) return textUI('flow_stage_testing');
    return textUI('flow_stage_preparing');
}

export function engineRows(jobs: ScanJob[], reportReady = false): EngineRow[] {
    const rows: EngineRow[] = [];
    const web = jobs.filter((j) => j.tool === 'nmap' || j.tool === 'nuclei' || j.tool === 'hackbot');
    if (web.length > 0) {
        const status = web.some((j) => j.status === 'running') ? 'running'
            : web.every((j) => j.status === 'failed') ? 'failed'
            : web.some((j) => j.status === 'waiting') ? 'waiting'
            : web.every((j) => j.status === 'finished') ? 'finished' : 'incomplete';
        rows.push({
            engine: 'orchis-redteam-agent',
            stage: status === 'waiting' ? textUI('flow_stage_preparing') : hackbotStage(web, reportReady),
            status,
            jobs: web,
            findings: web.reduce((n, j) => n + (j.findings?.length ?? 0), 0),
            error: web.map((j) => j.error).filter(Boolean).join(' · '),
        });
    }
    const defense = jobs.filter((j) => j.tool === 'defensebot');
    if (defense.length > 0) {
        const j = defense[0]!;
        const status = j.status;
        const stage = status === 'running' ? textUI('flow_stage_matching')
            : status === 'waiting' ? textUI('flow_stage_wait_scanner')
            : status === 'finished' || status === 'incomplete' ? textUI('flow_stage_collecting') : '';
        rows.push({ engine: 'orchis-defense-agent', stage, status, jobs: defense, findings: defense.reduce((n, x) => n + (x.findings?.length ?? 0), 0), error: j.error });
    }
    return rows;
}

// Nama engine yang tampil untuk daftar injector luring: pipeline orchis-redteam-agent (nmap/nuclei/hackbot)
// dilaporkan sebagai satu "orchis-redteam-agent", koleksi endpoint sebagai "orchis-defense-agent" (AUDIT UI-004d).
export function engineNames(keys: string[]): string[] {
    const out: string[] = [];
    if (keys.some(k => k === 'nmap' || k === 'nuclei' || k === 'hackbot')) out.push('orchis-redteam-agent');
    if (keys.some(k => k === 'defensebot')) out.push('orchis-defense-agent');
    return out;
}

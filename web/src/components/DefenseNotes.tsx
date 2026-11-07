import { t as textUI } from '../i18n/runtime';
import type { DefenseNote } from '../api/defense';

// Coverage notes arrive as codes plus a count or a short detail; the text is localized here and never taken from the server.
export function defenseNoteText(n: DefenseNote): string {
    const c = n.count ?? 0;
    const d = n.detail ?? '';
    switch (n.code) {
        case 'packages_truncated': return textUI("m_def_note_packages_truncated");
        case 'rpm_unsupported': return textUI("m_def_note_rpm_unsupported", [c]);
        case 'system_packages_skipped': return textUI("m_def_note_system_packages_skipped", [c]);
        case 'old_kernels_skipped': return textUI("m_def_note_old_kernels_skipped", [c]);
        case 'ecosystem_unsupported': return textUI("m_def_note_ecosystem_unsupported", [d]);
        case 'apps_unmapped': return textUI("m_def_note_apps_unmapped", [c]);
        case 'dataset_missing': return textUI("m_def_note_dataset_missing", [d]);
        case 'dataset_truncated': return textUI("m_def_note_dataset_truncated", [d]);
        case 'dataset_unavailable': return textUI("m_def_note_dataset_unavailable", [d]);
        case 'source_down': return textUI("m_def_note_source_down");
        case 'timeout': return textUI("m_def_note_timeout");
        case 'advisory_unreadable': return textUI("m_def_note_advisory_unreadable", [c]);
        case 'kev_unavailable': return textUI("m_def_note_kev_unavailable");
        case 'epss_unavailable': return textUI("m_def_note_epss_unavailable");
        case 'advisories_omitted': return textUI("m_def_note_advisories_omitted", [c]);
        case 'ports_unattributed': return textUI("m_def_note_ports_unattributed", [c]);
        case 'exposure_needs_scan': return textUI("m_def_note_exposure_needs_scan");
        default: return n.code;
    }
}

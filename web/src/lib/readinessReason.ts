import { language } from '../i18n/runtime';

// Alasan kesiapan datang dari API dalam teks Inggris tetap (internal/organizations/readiness.go).
// UI menerjemahkan reason yang dikenal; reason dinamis (mis. "Target refused: …") ditampilkan apa adanya.
const ID: Record<string, string> = {
    'Not authorized for testing': 'Belum diotorisasi untuk pengujian',
    'No agent is installed on this asset.': 'Belum ada agent terpasang pada aset ini.',
    'No approved web URL on this asset.': 'Belum ada URL web yang disetujui pada aset ini.',
    'Collection is not approved for the current consent document.': 'Koleksi belum disetujui untuk dokumen persetujuan saat ini.',
    'DefenseBot supports Linux and macOS agents only.': 'DefenseBot hanya mendukung agent Linux dan macOS.',
    'The agent is offline; bring it back online to collect software data.': 'Agent sedang luring; nyalakan kembali untuk mengumpulkan data perangkat lunak.',
    'the asset has no address this action can use (needs an IP, host name or URL)': 'aset tidak punya alamat yang bisa dipakai aksi ini (butuh alamat IP, nama host, atau URL)',
    'hostname targets require internet-facing authorization': 'target nama host butuh penandaan menghadap internet',
    'public targets require internet-facing authorization': 'target publik butuh penandaan menghadap internet',
    'scan target is not authorized': 'target scan belum diotorisasi',
};
// "Target refused: " adalah awalan tetap dari readiness.go; isi di belakangnya diterjemahkan bila dikenal.
const TARGET_REFUSED = 'Target refused: ';
export function localizeReason(reason: string | undefined | null): string {
    if (!reason) return '';
    if (language() !== 'id') return reason;
    if (reason.startsWith(TARGET_REFUSED)) {
        const inner = reason.slice(TARGET_REFUSED.length);
        return 'Target ditolak: ' + (ID[inner] ?? inner);
    }
    return ID[reason] ?? reason;
}
export function currentLanguage(): string {
    return language();
}

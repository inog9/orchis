// Platform agent yang punya artefak osquery terpatok (cermin dari osquery.SupportedPlatforms di Go; tes menjaga keduanya
// sama). Nama mengikuti API: os = linux | macos | windows, arch = x86_64 | arm64.
export const COLLECTION_PLATFORMS = ['linux/x86_64', 'macos/x86_64', 'macos/arm64'] as const;

export function collectionSupported(os: string | null | undefined, arch: string | null | undefined): boolean {
    const name = os === 'darwin' ? 'macos' : os;
    const cpu = arch === 'amd64' ? 'x86_64' : arch;
    return (COLLECTION_PLATFORMS as readonly string[]).includes(`${name}/${cpu}`);
}

// Query katalog yang dibuka untuk OS ini (agent macOS memakai nama "macos" di katalog).
export function catalogOS(os: string): string {
    return os === 'darwin' ? 'macos' : os;
}

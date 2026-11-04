import { t as textUI } from "../i18n/runtime";
import type { EnrollmentToken } from '../api/types';
const PS_QUOTE = (s: string) => `"${s.replace(/`/g, '``').replace(/"/g, '`"')}"`;
const SH_QUOTE = (s: string) => `'${s.replace(/'/g, `'\\''`)}'`;
// Perintah pemasangan untuk satu token. Biner diunduh dengan token di header (bukan di URL, supaya tidak masuk log),
// checksum SHA-256 dari konsol diperiksa sebelum dijalankan, lalu agent mendaftar sendiri.
export function installCommand(base: string, t: EnrollmentToken, name = ''): string {
    if (name && !/^[a-z0-9][a-z0-9_-]{0,39}$/.test(name))
        return textUI("m_22208ef6eebd");
    const server = base.replace(/\/+$/, '');
    if (t.os === 'windows') {
        const run = t.mode === 'service' ? '# Run as Administrator:\n.\\orchis-agent.exe install\n.\\orchis-agent.exe start' : '.\\orchis-agent.exe run';
        return [
            ...(name ? [`New-Item -ItemType Directory -Path ${PS_QUOTE('orchis-' + name)} -ErrorAction Stop | Out-Null`, `Set-Location ${PS_QUOTE('orchis-' + name)}`] : []),
            `Invoke-WebRequest -Headers @{ Authorization = ${PS_QUOTE('Bearer ' + t.token)} } -Uri ${PS_QUOTE(server + '/api/v1/agent/download')} -OutFile orchis-agent.exe`,
            `if ((Get-FileHash .\\orchis-agent.exe -Algorithm SHA256).Hash.ToLower() -ne ${PS_QUOTE(t.sha256)}) { throw "Checksum mismatch: do not run this file." }`,
            `.\\orchis-agent.exe enroll --server ${PS_QUOTE(server)} --token ${PS_QUOTE(t.token)}${name ? ` --name ${PS_QUOTE(name)}` : ''}`,
            run,
        ].join('\n');
    }
    // Server menyediakan kunci rilis: pakai /install.sh (SHA-256 + tanda tangan Ed25519 + TLS yang tidak pernah dimatikan).
    if (t.release_key)
        return installScriptCommand(server, t, name);
    const check = t.os === 'macos' ? `shasum -a 256 -c -` : `sha256sum -c -`;
    const tail = t.mode === 'service' ? ['sudo ./orchis-agent install', 'sudo ./orchis-agent start'] : ['./orchis-agent run'];
    return [
        ...(name ? [`mkdir ${SH_QUOTE('orchis-' + name)}`, `cd ${SH_QUOTE('orchis-' + name)}`] : []),
        `curl -fsSL -H ${SH_QUOTE('Authorization: Bearer ' + t.token)} -o orchis-agent ${SH_QUOTE(server + '/api/v1/agent/download')}`,
        `echo ${SH_QUOTE(t.sha256 + '  orchis-agent')} | ${check}`,
        'chmod +x orchis-agent',
        `./orchis-agent enroll --server ${SH_QUOTE(server)} --token ${SH_QUOTE(t.token)}${name ? ` --name ${SH_QUOTE(name)}` : ''}`,
        ...tail,
    ].join(' \\\n  && ');
}
// Server dengan CA internal atau sertifikat self-signed tidak dipercaya CA sistem, jadi sertifikatnya disimpan dulu
// (dicek sidik jari/pin-nya oleh pengguna) dan dipakai curl lewat --cacert; install.sh memverifikasi ulang. Tidak pernah -k.
function installScriptCommand(server: string, t: EnrollmentToken, name: string): string {
    const trust = t.ca_fingerprint ? { cert: 'ca.pem', note: textUI('install_ca_steps', [server, t.ca_fingerprint]), flag: ['--ca-fingerprint', t.ca_fingerprint] }
        : t.tls_pin ? { cert: 'server.pem', note: textUI('install_pin_steps', [server, t.tls_pin]), flag: ['--pin', t.tls_pin] }
            : null;
    const fetch = `curl -fsSL${trust ? ` --cacert ${trust.cert}` : ''} -o install.sh ${SH_QUOTE(server + '/install.sh')}`;
    const run = ['sh install.sh', `--server ${SH_QUOTE(server)}`, `--token ${SH_QUOTE(t.token)}`, `--sha256 ${SH_QUOTE(t.sha256)}`,
        `--release-key ${SH_QUOTE(t.release_key ?? '')}`, ...(trust ? [`${trust.flag[0]} ${SH_QUOTE(trust.flag[1] ?? '')}`] : []),
        ...(name ? [`--name ${SH_QUOTE(name)}`] : []), `--mode ${t.mode}`].join(' ');
    return [...(trust ? [trust.note.split('\n').map((line) => `# ${line}`).join('\n')] : []), `${fetch} \\\n  && ${run}`].join('\n');
}
export const OS_LABEL = { get windows() {
        return textUI("m_d598026a9cbc");
    }, get linux() {
        return textUI("m_4828e60247c1");
    }, macos: 'macOS' } as const;

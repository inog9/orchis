import type { EnrollmentToken } from '../api/types';

const PS_QUOTE = (s: string) => `"${s.replace(/`/g, '``').replace(/"/g, '`"')}"`;
const SH_QUOTE = (s: string) => `'${s.replace(/'/g, `'\\''`)}'`;

// Perintah pemasangan untuk satu token. Biner diunduh dengan token di header (bukan di URL, supaya tidak masuk log),
// checksum SHA-256 dari konsol diperiksa sebelum dijalankan, lalu agent mendaftar sendiri.
export function installCommand(base: string, t: EnrollmentToken, name = ''): string {
  if (name && !/^[a-z0-9][a-z0-9_-]{0,39}$/.test(name)) return 'Choose a valid agent instance name.';
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

export const OS_LABEL = { windows: 'Windows', linux: 'Linux', macos: 'macOS' } as const;

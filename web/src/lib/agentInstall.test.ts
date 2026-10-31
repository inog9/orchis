import { describe, expect, it } from 'vitest';
import type { EnrollmentToken } from '../api/types';
import { installCommand } from './agentInstall';

const tok = (extra: Partial<EnrollmentToken> = {}): EnrollmentToken => ({ id: 'i', token: 'oen_AbC-123_x', os: 'linux', arch: 'x86_64', mode: 'service', expires_at: '', version: '1.0.0', sha256: 'a'.repeat(64), file_name: 'f', ...extra });

describe('installCommand', () => {
  it('Linux service: unduh dengan token di header, cek checksum, daftar, pasang sebagai service', () => {
    const c = installCommand('https://orchis.example.com/', tok());
    expect(c).toContain(`-H 'Authorization: Bearer oen_AbC-123_x'`);
    expect(c).toContain(`'https://orchis.example.com/api/v1/agent/download'`);
    expect(c).not.toContain('?token=');
    expect(c).toContain(`echo '${'a'.repeat(64)}  orchis-agent' | sha256sum -c -`);
    expect(c.indexOf('sha256sum')).toBeLessThan(c.indexOf('chmod +x'));
    expect(c.indexOf('chmod +x')).toBeLessThan(c.indexOf('enroll'));
    expect(c).toContain('sudo ./orchis-agent install');
    expect(c).toContain('sudo ./orchis-agent start');
    expect(c).not.toContain('orchis-agent run');
  });
  it('mode user berjalan di latar depan tanpa sudo', () => {
    const c = installCommand('https://h', tok({ mode: 'user' }));
    expect(c).toContain('./orchis-agent run');
    expect(c).not.toContain('sudo');
  });
  it('macOS memakai shasum', () => {
    expect(installCommand('https://h', tok({ os: 'macos' }))).toContain('shasum -a 256 -c -');
  });
  it('Windows memakai PowerShell dengan pemeriksaan hash', () => {
    const c = installCommand('https://h', tok({ os: 'windows', arch: 'arm64' }));
    expect(c).toContain('Invoke-WebRequest -Headers @{ Authorization = "Bearer oen_AbC-123_x" }');
    expect(c).toContain('Get-FileHash .\\orchis-agent.exe -Algorithm SHA256');
    expect(c).toContain('throw "Checksum mismatch');
    expect(c).toContain('.\\orchis-agent.exe install');
  });
  it('tanda kutip pada alamat server tidak bisa menyuntikkan perintah', () => {
    const c = installCommand(`https://h'; rm -rf ~ #`, tok());
    expect(c).toContain(`'https://h'\\''; rm -rf ~ #/api/v1/agent/download'`);
    const w = installCommand('https://h"; calc #', tok({ os: 'windows' }));
    expect(w).toContain('"https://h`"; calc #/api/v1/agent/download"');
  });
});

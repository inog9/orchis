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

describe('installCommand dengan /install.sh', () => {
  const key = 'A'.repeat(43) + '=';
  it('memakai install.sh bila server menyediakan kunci rilis, tanpa -k', () => {
    const c = installCommand('https://orchis.example.com/', tok({ release_key: key }), 'lab');
    expect(c).toContain(`curl -fsSL -o install.sh 'https://orchis.example.com/install.sh'`);
    expect(c).toContain(`sh install.sh --server 'https://orchis.example.com' --token 'oen_AbC-123_x' --sha256 '${'a'.repeat(64)}' --release-key '${key}' --name 'lab' --mode service`);
    expect(c).not.toMatch(/\s-k\b|--insecure/);
    expect(c).not.toContain('/api/v1/agent/download');
  });
  it('mode user diteruskan dan macOS juga memakai install.sh', () => {
    const c = installCommand('https://h', tok({ release_key: key, mode: 'user', os: 'macos' }));
    expect(c).toContain('--mode user');
    expect(c).toContain('install.sh');
  });
  it('CA internal: simpan ca.pem, cek sidik jari, lalu --ca-fingerprint', () => {
    const fp = 'ab'.repeat(32);
    const c = installCommand('https://h', tok({ release_key: key, ca_fingerprint: fp }));
    expect(c).toContain('# This server uses an internal CA.');
    expect(c).toContain(`# openssl x509 -in ca.pem -outform DER | openssl dgst -sha256`);
    expect(c).toContain(`curl -fsSL --cacert ca.pem -o install.sh 'https://h/install.sh'`);
    expect(c).toContain(`--ca-fingerprint '${fp}'`);
    expect(c).not.toContain('--pin');
    expect(c).not.toMatch(/\s-k\b|--insecure/);
  });
  it('self-signed: simpan server.pem lalu --pin', () => {
    const pin = 'sha256//' + 'B'.repeat(43) + '=';
    const c = installCommand('https://h', tok({ release_key: key, tls_pin: pin }));
    expect(c).toContain('--cacert server.pem');
    expect(c).toContain(`--pin '${pin}'`);
    expect(c).not.toContain('--ca-fingerprint');
  });
  it('tanpa kunci rilis tetap memakai alur unduh lama (server lama)', () => {
    expect(installCommand('https://h', tok())).toContain('/api/v1/agent/download');
  });
  it('petunjuk CA/pin diterjemahkan ke Indonesia, perintah tetap sama', async () => {
    const { setLanguage } = await import('../i18n/runtime');
    setLanguage('id');
    try {
      const c = installCommand('https://h', tok({ release_key: key, ca_fingerprint: 'ab'.repeat(32) }));
      expect(c).toContain('# Server ini memakai CA internal.');
      expect(c).toContain('--ca-fingerprint');
    } finally { setLanguage('en'); }
  });
  it('tanda kutip pada token/server tidak bisa menyuntikkan perintah', () => {
    const c = installCommand(`https://h'; rm -rf ~ #`, tok({ release_key: key }));
    expect(c).toContain(`--server 'https://h'\\''; rm -rf ~ #'`);
  });
});

 describe('user mode startup guidance',()=>{
 it('shows the named installed binary and leaves service mode alone',()=>{
 const user=installCommand('https://h',tok({release_key:'A'.repeat(43)+'=',mode:'user'}),'lab');
 expect(user).toContain('without starting it');
 expect(user).toContain('"$HOME/orchis-agent-lab/orchis-agent" run');
 expect(installCommand('https://h',tok({release_key:'A'.repeat(43)+'='}),'lab')).not.toContain('without starting it');
 });
 });

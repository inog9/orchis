// Mengisi instalasi dengan data contoh untuk ditinjau manual. Pakai: E2E_BASE=http://127.0.0.1:8090 PASS=... node e2e/seed-demo.mjs
const BASE = process.env.E2E_BASE ?? 'http://127.0.0.1:8090';
const H = { 'Content-Type': 'application/json', 'X-Requested-With': 'orchis-web' };
let cookie = '';
const call = async (method, path, body) => {
  const r = await fetch(BASE + path, { method, headers: { ...H, ...(cookie ? { Cookie: cookie } : {}) }, body: body ? JSON.stringify(body) : undefined });
  const sc = r.headers.get('set-cookie'); if (sc) cookie = sc.split(';')[0];
  const t = await r.text(); if (!r.ok) throw new Error(`${method} ${path} ${r.status} ${t}`); return t ? JSON.parse(t) : null;
};
await call('POST', '/api/v1/auth/login', { email: 'admin@orchis.local', password: process.env.PASS });
const tag = await call('POST', '/api/v1/tags', { name: 'crown-jewel', color: '#E8A557' });
const me = await call('POST', '/api/v1/assets', { category: 'host', name: 'this-server (demo target)', platform: 'linux', arch: 'x86_64', criticality: 'high', testing_authorized: true, tag_ids: [tag.id],
  addresses: [{ kind: 'ip', value: '127.0.0.1' }, { kind: 'url', value: BASE }] });
await call('POST', '/api/v1/assets', { category: 'web_application', name: 'portal.corp.example', criticality: 'critical', is_internet_facing: true, addresses: [{ kind: 'url', value: 'https://portal.corp.example' }] });
await call('POST', '/api/v1/assets', { category: 'host', name: 'WINTERFELL', platform: 'windows', arch: 'x86_64', criticality: 'medium', addresses: [{ kind: 'hostname', value: 'winterfell.corp.example' }] });
await call('POST', '/api/v1/asset-groups', { name: 'Linux servers', rule: { mode: 'and', filters: [{ field: 'platform', operator: 'in', values: ['linux'] }] } });
const ada = await call('POST', '/api/v1/people', { first_name: 'Ada', last_name: 'Lovelace', email: 'ada@corp.example', organization: 'Corp' });
const team = await call('POST', '/api/v1/teams', { name: 'Blue team', kind: 'internal' });
await call('PUT', `/api/v1/teams/${team.id}/members`, { version: team.version, person_ids: [ada.id] });
const port = new URL(BASE).port || '80';
await call('POST', '/api/v1/atomic-tests', { name: 'Ports of this server', contract_key: 'nmap.tcp_connect', params: { profile: 'custom', ports: `22,${port}`, service_detection: false }, target_asset_id: me.id });
await call('POST', '/api/v1/atomic-tests', { name: 'Web checks of this server', contract_key: 'nuclei.scan', params: { severity: ['info', 'low', 'medium', 'high', 'critical'] }, target_asset_id: me.id });
const sc = await call('POST', '/api/v1/scenarios', { name: 'Scan then check web', severity: 'high', description: 'Port scan; if the web port is open, run Nuclei against it.' });
await call('PUT', `/api/v1/scenarios/${sc.id}/targets`, { version: sc.version, asset_ids: [me.id], group_ids: [] });
const s2 = await call('GET', `/api/v1/scenarios/${sc.id}`);
await call('PUT', `/api/v1/scenarios/${sc.id}/graph`, { version: s2.version, edges: [{ parent: 'scan', child: 'web' }], steps: [
  { key: 'scan', title: 'Port scan', contract_key: 'nmap.tcp_connect', trigger_mode: 'after_parents', is_enabled: true, delay_seconds: 0, params: { profile: 'custom', ports: `22,${port}` } },
  { key: 'web', title: 'Web checks', contract_key: 'nuclei.scan', trigger_mode: 'on_finding', trigger: { type: 'open_port', values: [`tcp/${port}`] }, is_enabled: true, delay_seconds: 0, params: { severity: ['info', 'low', 'medium', 'high', 'critical'] } }] });
await call('POST', '/api/v1/users', { email: 'analyst@orchis.local', display_name: 'Demo Analyst', role: 'analyst' }).catch(() => {});
console.log('data contoh siap');

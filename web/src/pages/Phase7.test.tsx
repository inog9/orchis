import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { CommandPalette, entriesFor, matches } from '../components/CommandPalette';
import { ForgotPage } from './LoginPage';
import { IntegrationsPanel } from './IntegrationsPanel';
import { MitrePage } from './MitrePage';
import { NotificationsPage } from './NotificationsPage';
import { PeoplePage } from './PeoplePage';
import { ProfilePage } from './ProfilePage';
import { ResetPage } from './ResetPage';
import { UsersPanel } from './UsersPanel';

const admin = { id: 'u1', email: 'a@b.c', display_name: 'Ada', role: 'admin', permissions: ['user.manage', 'integration.read', 'integration.manage', 'settings.manage', 'people.read', 'people.write', 'scenario.read', 'alert.read', 'asset.read', 'finding.read'] };
const viewer = { ...admin, id: 'u2', role: 'viewer', permissions: ['people.read', 'scenario.read', 'alert.read', 'integration.read'] };
const json = (v: unknown, status = 200) => new Response(JSON.stringify(v), { status });
const problem = (status: number, extra = {}) => json({ title: 'x', status, detail: 'detail text', ...extra }, status);

function route(user: object, handler: (url: string, init?: RequestInit) => Response | undefined) {
  return vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
    const url = String(input);
    if (url.includes('/auth/me')) return json(user);
    return handler(url, init) ?? json({}, 404);
  });
}
function Where() { return <p data-testid="where">{useLocation().pathname}</p>; }
function mount(ui: React.ReactNode, path = '/') {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}><MemoryRouter initialEntries={[path]}><Routes>
    <Route path="/people/:tab" element={ui} /><Route path="*" element={<>{ui}<Where /></>} /></Routes></MemoryRouter></QueryClientProvider>);
}
afterEach(() => vi.restoreAllMocks());

const urow = (extra = {}) => ({ id: 'x1', email: 'bob@co.test', display_name: 'Bob', role: 'analyst', role_name: 'Analyst', status: 'active', last_login_at: null, created_at: '2026-10-29T10:00:00Z', version: 1, has_password: true, ...extra });
const roles = { items: [{ key: 'admin', name: 'Admin', description: '', permissions: [], is_system: true, users: 1 }, { key: 'analyst', name: 'Analyst', description: '', permissions: [], is_system: true, users: 1 }, { key: 'viewer', name: 'Viewer', description: '', permissions: [], is_system: true, users: 0 }] };

describe('UsersPanel', () => {
  it('menandai Active, Invited (belum punya password), Disabled, dan akun sendiri', async () => {
    route(admin, (u) => (u.endsWith('/users') ? json({ items: [urow({ id: 'u1', display_name: 'Ada', email: 'a@b.c' }), urow({ id: 'x2', display_name: 'Cy', has_password: false }), urow({ id: 'x3', display_name: 'Di', status: 'disabled' })] }) : undefined));
    mount(<UsersPanel />);
    expect(await screen.findByText('Ada (you)')).toBeInTheDocument();
    const t = screen.getByRole('table', { name: 'Users' });
    expect(within(t).getByText('Invited')).toBeInTheDocument();
    expect(within(t).getByText('Disabled')).toBeInTheDocument();
    expect(within(t).queryByRole('button', { name: 'Reset link for Di' })).not.toBeInTheDocument(); // akun nonaktif tidak punya link
    expect(within(t).getByRole('button', { name: 'Reset link for Cy' })).toBeInTheDocument();
  });
  it('membuat user lalu menampilkan link sekali pakai, dan galat email muncul di kolomnya', async () => {
    let posts = 0;
    const f = route(admin, (u, init) => {
      if (u.endsWith('/roles')) return json(roles);
      if (u.endsWith('/users') && init?.method === 'POST') {
        posts++;
        return posts === 1 ? problem(422, { fields: [{ name: 'email', message: 'An account with this email already exists.' }] })
          : json({ user: urow({ display_name: 'New Person' }), reset_link: { token: 'orl_secret123', expires_at: '2026-10-29T10:30:00Z' } }, 201);
      }
      if (u.endsWith('/users')) return json({ items: [] });
    });
    const user = userEvent.setup();
    mount(<UsersPanel />);
    await user.click(await screen.findByRole('button', { name: /new user/i }));
    const dlg = await screen.findByRole('dialog');
    await user.type(within(dlg).getByLabelText('Email'), 'dup@co.test');
    await user.type(within(dlg).getByLabelText('Name'), 'New Person');
    await user.click(within(dlg).getByRole('button', { name: 'Create user' }));
    expect(await within(dlg).findByText('An account with this email already exists.')).toBeInTheDocument();
    await user.click(within(dlg).getByRole('button', { name: 'Create user' }));
    const link = await screen.findByTestId('reset-url');
    expect(link.textContent).toMatch(/\/reset\?token=orl_secret123$/);
    expect(screen.getByText(/It won’t be shown again/)).toBeInTheDocument();
    const body = JSON.parse(String(f.mock.calls.filter(([, i]) => i?.method === 'POST').pop()![1]!.body));
    expect(body).toEqual({ email: 'dup@co.test', display_name: 'New Person', role: 'analyst' });
    await user.click(screen.getByRole('button', { name: 'Done' }));
    expect(screen.queryByTestId('reset-url')).not.toBeInTheDocument();
  });
  it('mengubah akun mengirim versi yang dimuat; menolak ubah diri sendiri dari server ditampilkan', async () => {
    const f = route(admin, (u, init) => {
      if (u.endsWith('/roles')) return json(roles);
      if (u.includes('/users/x1') && init?.method === 'PATCH') return problem(422, { fields: [{ name: 'role', message: 'You can’t change your own role or status. Ask another administrator.' }] });
      if (u.endsWith('/users')) return json({ items: [urow({ version: 4 })] });
    });
    const user = userEvent.setup();
    mount(<UsersPanel />);
    await user.click(await screen.findByRole('button', { name: 'Edit Bob' }));
    const dlg = await screen.findByRole('dialog');
    await user.selectOptions(within(dlg).getByLabelText('Role'), 'viewer');
    await user.click(within(dlg).getByRole('button', { name: 'Save' }));
    expect(await within(dlg).findByText(/You can’t change your own role/)).toBeInTheDocument();
    expect(JSON.parse(String(f.mock.calls.find(([, i]) => i?.method === 'PATCH')![1]!.body))).toMatchObject({ version: 4, role: 'viewer', status: 'active' });
  });
});

describe('ResetPage and ForgotPage', () => {
  it('link tidak valid menjelaskan apa yang harus dilakukan', async () => {
    route(admin, (u) => (u.includes('/auth/reset/check') ? problem(400) : undefined));
    mount(<ResetPage />, '/reset?token=orl_bad');
    expect(await screen.findByText('This link can’t be used')).toBeInTheDocument();
    expect(screen.queryByLabelText('New password')).not.toBeInTheDocument();
  });
  it('tanpa token tidak memanggil server dan langsung menolak', () => {
    const f = route(admin, () => undefined);
    mount(<ResetPage />, '/reset');
    expect(screen.getByText('This link can’t be used')).toBeInTheDocument();
    expect(f.mock.calls.some(([u]) => String(u).includes('/auth/reset'))).toBe(false);
  });
  it('mengatur password: ketidakcocokan, kebijakan server, lalu berhasil menuju login', async () => {
    let tries = 0;
    const f = route(admin, (u, init) => {
      if (u.includes('/auth/reset/check')) return new Response(null, { status: 204 });
      if (u.endsWith('/auth/reset') && init?.method === 'POST') { tries++; return tries === 1 ? problem(422, { fields: [{ name: 'password', message: 'Use at least 12 characters.' }] }) : new Response(null, { status: 204 }); }
    });
    const user = userEvent.setup();
    mount(<ResetPage />, '/reset?token=orl_ok');
    await user.type(await screen.findByLabelText('New password'), 'short');
    await user.type(screen.getByLabelText('Repeat the password'), 'different');
    expect(screen.getByText('The passwords don’t match.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Set password' })).toBeEnabled();
    await user.clear(screen.getByLabelText('Repeat the password'));
    await user.type(screen.getByLabelText('Repeat the password'), 'short');
    await user.click(screen.getByRole('button', { name: 'Set password' }));
    expect(await screen.findByText('Use at least 12 characters.')).toBeInTheDocument();
    await user.clear(screen.getByLabelText('New password'));
    await user.clear(screen.getByLabelText('Repeat the password'));
    await user.type(screen.getByLabelText('New password'), 'Correct-Horse-9');
    await user.type(screen.getByLabelText('Repeat the password'), 'Correct-Horse-9');
    await user.click(screen.getByRole('button', { name: 'Set password' }));
    await waitFor(() => expect(screen.getByTestId('where')).toHaveTextContent('/login'));
    const post = f.mock.calls.filter(([u, i]) => String(u).endsWith('/auth/reset') && i?.method === 'POST').pop()!;
    expect(JSON.parse(String(post[1]!.body))).toEqual({ token: 'orl_ok', password: 'Correct-Horse-9' });
  });
  it('lupa password: jawaban sama untuk email apa pun', async () => {
    route(admin, (u) => (u.endsWith('/auth/forgot') ? new Response(null, { status: 202 }) : undefined));
    const user = userEvent.setup();
    mount(<ForgotPage />, '/forgot');
    await user.type(screen.getByLabelText('Email'), 'whoever@co.test');
    await user.click(screen.getByRole('button', { name: /ask for a reset link/i }));
    expect(await screen.findByText('Ask your administrator')).toBeInTheDocument();
    expect(screen.getByText(/If that email belongs to an account/)).toBeInTheDocument();
  });
});

describe('ProfilePage', () => {
  it('token: hanya awalan tampil di daftar, rahasia sekali di dialog, cabut meminta konfirmasi', async () => {
    let created = false;
    const f = route(admin, (u, init) => {
      if (u.endsWith('/me/tokens') && init?.method === 'POST') { created = true; return json({ id: 't2', name: 'CI', prefix: 'oat_abcd', secret: 'oat_abcdSECRETSECRET', created_at: '2026-10-29T10:00:00Z', last_used_at: null, expires_at: '2027-01-27T10:00:00Z' }, 201); }
      if (u.endsWith('/me/tokens') && (!init?.method || init.method === 'GET')) return json({ items: created ? [{ id: 't2', name: 'CI', prefix: 'oat_abcd', created_at: '2026-10-29T10:00:00Z', last_used_at: null, expires_at: '2027-01-27T10:00:00Z' }] : [] });
      if (u.includes('/me/tokens/t2') && init?.method === 'DELETE') return new Response(null, { status: 204 });
    });
    const user = userEvent.setup();
    mount(<ProfilePage />, '/profile');
    expect(await screen.findByText('You have no active tokens.')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /new token/i }));
    const dlg = await screen.findByRole('dialog');
    await user.type(within(dlg).getByLabelText('Name'), 'CI');
    await user.click(within(dlg).getByRole('button', { name: 'Create token' }));
    expect(await screen.findByTestId('api-token')).toHaveTextContent('oat_abcdSECRETSECRET');
    expect(JSON.parse(String(f.mock.calls.find(([u, i]) => String(u).endsWith('/me/tokens') && i?.method === 'POST')![1]!.body))).toEqual({ name: 'CI', expires_in_days: 90 });
    await user.click(screen.getByRole('button', { name: 'Done' }));
    expect(screen.queryByTestId('api-token')).not.toBeInTheDocument();
    const list = await screen.findByRole('list', { name: 'API tokens' });
    expect(list.textContent).toContain('oat_abcd…');
    expect(list.textContent).not.toContain('SECRETSECRET');
    await user.click(screen.getByRole('button', { name: 'Revoke CI' }));
    expect(f.mock.calls.some(([, i]) => i?.method === 'DELETE')).toBe(false); // belum, menunggu konfirmasi
    await user.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Revoke token' }));
    await waitFor(() => expect(f.mock.calls.some(([u, i]) => String(u).includes('/me/tokens/t2') && i?.method === 'DELETE')).toBe(true));
  });
  it('ganti password menampilkan galat kolom dari server', async () => {
    route(admin, (u, init) => {
      if (u.endsWith('/me/tokens')) return json({ items: [] });
      if (u.endsWith('/me/password') && init?.method === 'POST') return problem(422, { fields: [{ name: 'current_password', message: 'That isn’t your current password.' }] });
    });
    const user = userEvent.setup();
    mount(<ProfilePage />, '/profile');
    await user.type(await screen.findByLabelText('Current password'), 'nope');
    await user.type(screen.getByLabelText('New password'), 'Correct-Horse-9');
    await user.click(screen.getByRole('button', { name: 'Change password' }));
    expect(await screen.findByText('That isn’t your current password.')).toBeInTheDocument();
  });
});

describe('IntegrationsPanel', () => {
  const integ = (extra = {}) => ({ id: 'i1', kind: 'injector', key: 'nmap', name: 'Nmap', description: 'Port discovery.', is_builtin: true, enabled: true, status: 'online', software_version: '1.2', last_heartbeat_at: '2026-10-29T10:00:00Z', has_token: true, logs: [{ at: '2026-10-29T10:00:00Z', level: 'info', message: 'Injector connected.' }], ...extra });
  it('menampilkan status, log, dan memutar token setelah konfirmasi (rahasia sekali)', async () => {
    const f = route(admin, (u, init) => {
      if (u.endsWith('/integrations/i1/token') && init?.method === 'POST') return json({ token: 'oij_NEWTOKEN' }, 201);
      if (u.endsWith('/integrations')) return json({ items: [integ(), integ({ id: 'i2', key: 'nuclei', name: 'Nuclei', status: 'disabled', enabled: false, has_token: false, logs: [] })] });
    });
    const user = userEvent.setup();
    mount(<IntegrationsPanel />);
    expect(await screen.findByText('Running')).toBeInTheDocument();
    expect(screen.getByText('Disabled')).toBeInTheDocument();
    expect(screen.getByRole('list', { name: 'Nmap recent log' })).toHaveTextContent('Injector connected.');
    expect(screen.getByRole('list', { name: 'Nuclei recent log' })).toHaveTextContent('No log lines yet.');
    await user.click(screen.getByRole('button', { name: 'Rotate token' }));
    expect(f.mock.calls.some(([u]) => String(u).endsWith('/token'))).toBe(false);
    await user.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Rotate token' }));
    expect(await screen.findByTestId('injector-token')).toHaveTextContent('oij_NEWTOKEN');
  });
  it('viewer hanya melihat; tanpa tombol kelola', async () => {
    route(viewer, (u) => (u.endsWith('/integrations') ? json({ items: [integ()] }) : undefined));
    mount(<IntegrationsPanel />);
    await screen.findByText('Running');
    expect(screen.queryByRole('button', { name: /rotate|disable|enable|create token/i })).not.toBeInTheDocument();
  });
});

describe('PeoplePage', () => {
  const person = (extra = {}) => ({ id: 'p1', first_name: 'Ada', last_name: 'Lovelace', email: 'ada@co.test', organization: 'Corp', teams: [{ id: 't1', name: 'Blue' }], created_at: '2026-10-29T10:00:00Z', version: 2, ...extra });
  it('daftar orang dengan tim, dan galat email ganda tampil di kolomnya', async () => {
    const f = route(admin, (u, init) => {
      if (u.includes('/people') && init?.method === 'POST') return problem(422, { fields: [{ name: 'email', message: 'Someone with this email already exists.' }] });
      if (u.includes('/people')) return json({ items: [person()] });
    });
    const user = userEvent.setup();
    mount(<PeoplePage />, '/people');
    const t = await screen.findByRole('table', { name: 'People' });
    expect(within(t).getByText('Ada Lovelace')).toBeInTheDocument();
    expect(within(t).getByText('Blue')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /add person/i }));
    const dlg = await screen.findByRole('dialog');
    await user.type(within(dlg).getByLabelText('First name'), 'Eve');
    await user.type(within(dlg).getByLabelText('Email (optional)'), 'ada@co.test');
    await user.click(within(dlg).getByRole('button', { name: 'Save' }));
    expect(await within(dlg).findByText('Someone with this email already exists.')).toBeInTheDocument();
    expect(JSON.parse(String(f.mock.calls.find(([, i]) => i?.method === 'POST')![1]!.body))).toEqual({ first_name: 'Eve', last_name: '', email: 'ada@co.test', organization: null });
  });
  it('tim tanpa anggota diberi catatan, dan anggota disimpan dengan versi tim', async () => {
    const f = route(admin, (u, init) => {
      if (u.endsWith('/teams/t1/members') && init?.method === 'PUT') return json({ id: 't1', name: 'Blue', description: null, kind: 'internal', member_count: 1, created_at: '', version: 6, members: [] });
      if (u.endsWith('/teams/t1')) return json({ id: 't1', name: 'Blue', description: null, kind: 'internal', member_count: 0, created_at: '', version: 5, members: [] });
      if (u.endsWith('/teams')) return json({ items: [{ id: 't1', name: 'Blue', description: null, kind: 'internal', member_count: 0, created_at: '', version: 5 }] });
      if (u.includes('/people')) return json({ items: [person({ teams: [] })] });
    });
    const user = userEvent.setup();
    mount(<PeoplePage />, '/people/teams');
    expect(await screen.findByText('No members yet.')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Members' }));
    const dlg = await screen.findByRole('dialog');
    await user.click(await within(dlg).findByLabelText(/Ada Lovelace/));
    await user.click(within(dlg).getByRole('button', { name: 'Save members' }));
    await waitFor(() => expect(f.mock.calls.some(([, i]) => i?.method === 'PUT')).toBe(true));
    expect(JSON.parse(String(f.mock.calls.find(([, i]) => i?.method === 'PUT')![1]!.body))).toEqual({ version: 5, person_ids: ['p1'] });
  });
  it('viewer tidak melihat tombol tulis', async () => {
    route(viewer, (u) => (u.includes('/people') ? json({ items: [person()] }) : undefined));
    mount(<PeoplePage />, '/people');
    await screen.findByText('Ada Lovelace');
    expect(screen.queryByRole('button', { name: /add person|edit|delete/i })).not.toBeInTheDocument();
  });
});

describe('MitrePage', () => {
  const cell = (id: string, name: string, extra = {}) => ({ id, name, mapped: false, exercised: false, runs: 0, last_run_at: null, ...extra });
  const matrix = (extra = {}) => ({ imported: true, total: 3, mapped: 2, exercised: 1,
    last_run: { id: 'r', status: 'succeeded', source_version: '15.1', techniques: 3, tactics: 2, error: null, started_at: '', finished_at: '2026-10-29T10:00:00Z' },
    tactics: [{ id: 'TA0007', name: 'Discovery', shortname: 'discovery', techniques: [cell('T1046', 'Network Service Discovery', { mapped: true, exercised: true, runs: 2 }), cell('T1082', 'System Information Discovery')] },
      { id: 'TA0043', name: 'Reconnaissance', shortname: 'reconnaissance', techniques: [cell('T1595', 'Active Scanning', { mapped: true })] }], ...extra });
  it('mewarnai teknik dan menyaring "hanya yang bisa dilatih"', async () => {
    route(admin, (u) => (u.endsWith('/mitre/matrix') ? json(matrix()) : undefined));
    const user = userEvent.setup();
    mount(<MitrePage />, '/mitre');
    expect(await screen.findByRole('button', { name: /T1046 .*trained: an action was run successfully/ })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /T1595 .*mapped: an action can exercise it, not yet run/ })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /T1082 .*not covered/ })).toBeInTheDocument();
    expect(within(screen.getByRole('list', { name: 'Coverage' })).getByText('Trained').parentElement).toHaveTextContent('1');
    await user.click(screen.getByLabelText(/Only mapped techniques/));
    expect(screen.queryByRole('button', { name: /T1082/ })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: /T1046/ })).toBeInTheDocument();
  });
  it('detail teknik memuat tautan resmi dan aksi yang melatihnya', async () => {
    route(admin, (u) => {
      if (u.endsWith('/mitre/matrix')) return json(matrix());
      if (u.endsWith('/mitre/techniques/T1046')) return json({ id: 'T1046', name: 'Network Service Discovery', description: 'Adversaries may scan.', platforms: ['Linux'], tactics: ['Discovery'], url: 'https://attack.mitre.org/techniques/T1046/', actions: ['Nmap TCP connect scan'], runs: 2, last_run_at: '2026-10-29T10:00:00Z', subs: [] });
    });
    const user = userEvent.setup();
    mount(<MitrePage />, '/mitre');
    await user.click(await screen.findByRole('button', { name: /T1046/ }));
    const panel = await screen.findByRole('region', { name: 'Technique details' });
    expect(await within(panel).findByText(/Adversaries may scan/)).toBeInTheDocument();
    expect(within(panel).getByText(/Nmap TCP connect scan/)).toBeInTheDocument();
    expect(within(panel).getByRole('link', { name: /View on MITRE/ })).toHaveAttribute('href', 'https://attack.mitre.org/techniques/T1046/');
    expect(within(panel).getByRole('link', { name: /View on MITRE/ })).toHaveAttribute('rel', 'noopener noreferrer');
  });
  it('cakupan sintetis diberi label dan tidak menambah trained', async () => {
    route(admin, (u) => (u.endsWith('/mitre/matrix') ? json(matrix({ total: 1, mapped: 0, exercised: 0, simulated: 1, tactics: [{ id: 'TA0001', name: 'Initial Access', shortname: 'initial-access', techniques: [cell('T1190', 'Exploit Public-Facing Application', { simulated: true })] }] })) : undefined));
    mount(<MitrePage />, '/mitre');
    const simulated = await screen.findByRole('button', { name: /T1190 .*simulated: synthetic demo evidence/ });
    expect(simulated).toHaveClass('mcell--simulated');
    expect(within(simulated).getByText('Demo')).toBeInTheDocument();
    expect(within(screen.getByRole('list', { name: 'Coverage' })).getByText('Simulated').parentElement).toHaveTextContent('1');
    expect(within(screen.getByRole('list', { name: 'Coverage' })).getByText('Trained').parentElement).toHaveTextContent('0');
    expect(screen.getByText(/never counts as trained/)).toBeInTheDocument();
  });
  it('mencari dan menyaring per platform (induk tetap tampil bila sub-tekniknya cocok)', async () => {
    route(admin, (u) => (u.endsWith('/mitre/matrix') ? json(matrix({ platforms: ['Linux', 'Windows'], tactics: [{ id: 'TA0007', name: 'Discovery', shortname: 'discovery', techniques: [
      cell('T1046', 'Network Service Discovery', { platforms: ['Linux', 'Windows'] }),
      cell('T1082', 'System Information Discovery', { platforms: ['Windows'] }),
      cell('T1595', 'Active Scanning', { platforms: ['PRE'], subs: [cell('T1595.002', 'Vulnerability Scanning', { platforms: ['Linux'] })] })] }] })) : undefined));
    const user = userEvent.setup();
    mount(<MitrePage />, '/mitre');
    await screen.findByRole('button', { name: /T1046/ });
    await user.selectOptions(screen.getByLabelText('Platform'), 'Linux');
    expect(screen.queryByRole('button', { name: /T1082/ })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: /T1046/ })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /T1595 /})).toBeInTheDocument();
    await user.selectOptions(screen.getByLabelText('Platform'), '');
    await user.type(screen.getByLabelText('Search'), 'system info');
    expect(screen.getByRole('button', { name: /T1082/ })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /T1046/ })).not.toBeInTheDocument();
  });
  it('detail memuat pemetaan beralasan, sub-teknik, mitigasi dan catatan demo', async () => {
    route(admin, (u) => {
      if (u.endsWith('/mitre/matrix')) return json(matrix());
      if (u.endsWith('/mitre/techniques/T1046')) return json({ id: 'T1046', name: 'Network Service Discovery', description: 'Adversaries may scan.', platforms: ['Linux'], tactics: ['Discovery'], url: 'https://attack.mitre.org/techniques/T1046', actions: [], runs: 0, last_run_at: null,
        mappings: [{ action: 'Nmap TCP connect scan', reason: 'A TCP connect scan enumerates open ports.', basis: 'direct' }], subs: [{ id: 'T1046.001', name: 'Child', mapped: false, exercised: false, runs: 0, last_run_at: null }],
        mitigations: Array.from({ length: 8 }, (_, i) => ({ id: `M10${i}0`, name: `Mitigation ${i}`, description: 'Do the thing.' })), simulated: true, simulated_runs: 3, simulated_findings: 2 });
      return undefined;
    });
    const user = userEvent.setup();
    mount(<MitrePage />, '/mitre');
    await user.click(await screen.findByRole('button', { name: /T1046/ }));
    const panel = await screen.findByRole('region', { name: 'Technique details' });
    expect(await within(panel).findByText('A TCP connect scan enumerates open ports.')).toBeInTheDocument();
    expect(within(panel).getByText('direct')).toBeInTheDocument();
    expect(within(panel).getByRole('button', { name: 'T1046.001' })).toBeInTheDocument();
    expect(within(panel).getByText('Demo / synthetic evidence:')).toBeInTheDocument();
    expect(within(panel).getByText(/3 illustrative successful runs and 2 finding references/)).toBeInTheDocument();
    expect(within(panel).getByText(/These never count as trained/)).toBeInTheDocument();
    expect(within(panel).getAllByText(/^Mitigation \d$/)).toHaveLength(6);
    await user.click(within(panel).getByRole('button', { name: 'Show all 8' }));
    expect(within(panel).getAllByText(/^Mitigation \d$/)).toHaveLength(8);
  });
  it('belum diimpor: menjelaskan keadaan (kosong, berjalan, gagal)', async () => {
    const empty = { imported: false, total: 0, mapped: 0, exercised: 0, tactics: [] };
    route(admin, (u) => (u.endsWith('/mitre/matrix') ? json({ ...empty, last_run: null }) : undefined));
    const a = mount(<MitrePage />, '/mitre');
    expect(await screen.findByText('No ATT&CK data yet')).toBeInTheDocument();
    a.unmount(); vi.restoreAllMocks();
    route(admin, (u) => (u.endsWith('/mitre/matrix') ? json({ ...empty, last_run: { id: 'r', status: 'running', started_at: '', finished_at: null, error: null } }) : undefined));
    const b = mount(<MitrePage />, '/mitre');
    expect(await screen.findByText('Importing ATT&CK data…')).toBeInTheDocument();
    b.unmount(); vi.restoreAllMocks();
    route(admin, (u) => (u.endsWith('/mitre/matrix') ? json({ ...empty, last_run: { id: 'r', status: 'failed', started_at: '', finished_at: '', error: 'sumber menjawab 503' } }) : undefined));
    mount(<MitrePage />, '/mitre');
    expect(await screen.findByText(/The last import failed: sumber menjawab 503/)).toBeInTheDocument();
  });
});

describe('NotificationsPage', () => {
  const a = (extra = {}) => ({ id: 'a1', kind: 'simulation_finished', severity: 'info', title: 'Simulation OP-0001 finished', body: 'Run one', resource_type: 'simulation', resource_id: 'sim-1', created_at: '2026-10-29T10:00:00Z', read: false, ...extra });
  it('membuka alert menandainya dibaca dan membawa ke sumbernya; baca-semua dan singkirkan', async () => {
    const f = route(admin, (u, init) => {
      if (init?.method === 'POST') return new Response(null, { status: 204 });
      if (u.includes('/alerts')) return json({ items: [a(), a({ id: 'a2', title: 'Old news', read: true, resource_type: null, resource_id: null })], unread: 1 });
    });
    const user = userEvent.setup();
    mount(<NotificationsPage />, '/notifications');
    expect(await screen.findByText('1 unread')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Mark all as read' }));
    await waitFor(() => expect(f.mock.calls.some(([u]) => String(u).endsWith('/alerts/read-all'))).toBe(true));
    await user.click(screen.getByRole('button', { name: /Dismiss Old news/ }));
    await waitFor(() => expect(f.mock.calls.some(([u]) => String(u).endsWith('/alerts/a2/dismiss'))).toBe(true));
    await user.click(screen.getByRole('button', { name: /^Simulation OP-0001 finished/ }));
    await waitFor(() => expect(f.mock.calls.some(([u]) => String(u).endsWith('/alerts/a1/read'))).toBe(true));
    await waitFor(() => expect(screen.getByTestId('where')).toHaveTextContent('/simulations/sim-1'));
  });
  it('kosong dan galat', async () => {
    route(admin, (u) => (u.includes('/alerts') ? json({ items: [], unread: 0 }) : undefined));
    const x = mount(<NotificationsPage />, '/notifications');
    expect(await screen.findByText('You’re all caught up')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Mark all as read' })).toBeDisabled();
    x.unmount(); vi.restoreAllMocks();
    route(admin, (u) => (u.includes('/alerts') ? problem(500) : undefined));
    mount(<NotificationsPage />, '/notifications');
    expect(await screen.findByText('Couldn’t load notifications')).toBeInTheDocument();
  });
});

describe('CommandPalette', () => {
  it('entri mengikuti izin pengguna, dan pencarian menyaring label dan petunjuk', () => {
    const v = entriesFor({ ...viewer, permissions: ['atomic.read'] }).map((e) => e.id);
    expect(v).toContain('home'); expect(v).toContain('atomic'); expect(v).toContain('settings');
    expect(v).not.toContain('findings'); expect(v).not.toContain('new-asset'); expect(v).not.toContain('people');
    const all = entriesFor(admin);
    expect(all.filter((e) => matches(e, 'discover')).map((e) => e.id)).toEqual(['findings']);
    expect(all.filter((e) => matches(e, 'attack')).map((e) => e.id)).toContain('mitre');
    expect(all.filter((e) => matches(e, '  '))).toHaveLength(all.length);
  });
  it('Enter membuka entri terpilih, panah memindah pilihan, Esc menutup', async () => {
    route(admin, () => undefined);
    const onClose = vi.fn();
    const qc = new QueryClient();
    const user = userEvent.setup();
    render(<QueryClientProvider client={qc}><MemoryRouter><CommandPalette onClose={onClose} /><Where /></MemoryRouter></QueryClientProvider>);
    const box = await screen.findByRole('combobox', { name: 'Search or jump to' });
    await user.type(box, 'find');
    expect(await screen.findByRole('option', { name: /Findings/ })).toHaveAttribute('aria-selected', 'true');
    await user.keyboard('{Enter}');
    expect(onClose).toHaveBeenCalled();
    await waitFor(() => expect(screen.getByTestId('where')).toHaveTextContent('/findings'));
  });
  it('tanpa hasil memberi tahu, dan panah bawah memilih entri berikutnya', async () => {
    route(admin, () => undefined);
    const qc = new QueryClient();
    const user = userEvent.setup();
    render(<QueryClientProvider client={qc}><MemoryRouter><CommandPalette onClose={() => {}} /></MemoryRouter></QueryClientProvider>);
    const box = await screen.findByRole('combobox');
    await user.type(box, 'zzzz');
    expect(screen.getByText(/Nothing matches/)).toBeInTheDocument();
    await user.clear(box);
    await screen.findAllByRole('option');
    await waitFor(() => expect(screen.getAllByRole('option').length).toBeGreaterThan(3)); // izin sudah dimuat
    await user.keyboard('{ArrowDown}');
    const opts = screen.getAllByRole('option');
    expect(opts[1]).toHaveAttribute('aria-selected', 'true');
  });
});

// Tes halaman Profil hasil redesain: identitas, metrik, informasi akun, izin, status platform, token, logout.
// Semua respons API dipalsukan lewat fetch global; bahasa uji default EN (tidak ada navigator.languages di jsdom).
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ProfilePage } from './ProfilePage';

const json = (v: unknown, status = 200) => new Response(JSON.stringify(v), { status });

// 22 izin: 20 pertama tampil sebagai chip, 2 sisanya diringkas "+2 more permissions".
const PERMS = [
    'organization.write', 'assessment.run', 'report.generate', 'asset.write', 'asset.delete',
    'asset.authorize', 'scenario.write', 'simulation.run', 'finding.write', 'ai.manage',
    'agent.manage', 'integration.manage', 'user.manage', 'settings.manage', 'audit.read',
    'alert.read', 'alert.write', 'report.read', 'asset.read', 'finding.read',
    'people.read', 'extra.perm',
];
const user = { id: 'u1', email: 'ada@orchis.local', display_name: 'Ada Admin', role: 'admin', permissions: PERMS, ui_language: 'en', features: { advanced: false } };
const settings = {
    platform_name: 'Orchis Local', version: 'local',
    components: [
        { name: 'PostgreSQL', status: 'up' },
        { name: 'Elasticsearch', status: 'up' },
        { name: 'RabbitMQ', status: 'down' },
        { name: 'MinIO', status: 'not_configured' },
    ],
};
const token = { id: 't2', name: 'CI', prefix: 'oat_abcd', created_at: '2026-10-29T10:00:00Z', last_used_at: null, expires_at: '2027-01-27T10:00:00Z' };

function mount(handler: (url: string, init?: RequestInit) => Response | undefined) {
    const f = vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
        const url = String(input);
        if (url.includes('/auth/me')) return json(user);
        return handler(url, init) ?? json({}, 404);
    });
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(<QueryClientProvider client={qc}><MemoryRouter initialEntries={['/profile']}><Routes>
        <Route path="/profile" element={<ProfilePage/>}/>
        <Route path="*" element={<ProfilePage/>}/>
    </Routes></MemoryRouter></QueryClientProvider>);
    return f;
}

afterEach(() => vi.restoreAllMocks());

describe('ProfilePage', () => {
    it('menampilkan heading, deskripsi, metrik, informasi akun, izin, dan status platform', async () => {
        mount((u) => (u.endsWith('/settings') ? json(settings) : u.endsWith('/me/tokens') ? json({ items: [] }) : undefined));
        expect(await screen.findByRole('heading', { name: 'Ada Admin' })).toBeInTheDocument();
        expect(screen.getByText('ada@orchis.local · Role: Admin · UI language: EN')).toBeInTheDocument();
        expect(screen.getByRole('button', { name: 'Sign out' })).toBeInTheDocument();
        // Strip metrik: role, jumlah izin, status komponen 2/4, versi platform.
        expect(screen.getByText('Admin')).toBeInTheDocument();
        expect(screen.getByText('22')).toBeInTheDocument();
        // Kartu komponen/versi menunggu /settings, jadi pakai findBy.
        expect(await screen.findByText('2/4')).toBeInTheDocument();
        expect(screen.getByText('2 not up')).toBeInTheDocument();
        expect(screen.getByText('local')).toBeInTheDocument();
        // Panel informasi akun: lima field dari data /auth/me.
        expect(screen.getByText('Advanced features')).toBeInTheDocument();
        expect(screen.getByText('Disabled')).toBeInTheDocument();
        // Izin: 20 chip pertama + ringkasan, tanpa kebocoran izin ke-21.
        const chips = screen.getByRole('list', { name: 'Access permissions' });
        expect(within(chips).getAllByRole('listitem')).toHaveLength(21);
        expect(chips.textContent).toContain('+2 more permissions');
        expect(chips.textContent).not.toContain('extra.perm');
        // Status platform: kartu per komponen, label mengikuti status nyata.
        expect(screen.getByText('PostgreSQL')).toBeInTheDocument();
        expect(screen.getByText('RabbitMQ')).toBeInTheDocument();
        expect(screen.getByText('Not configured')).toBeInTheDocument();
        // Token kosong: empty state + tombol buat (endpoint POST /me/tokens ada).
        expect(await screen.findByText('You have no active tokens.')).toBeInTheDocument();
        expect(screen.getByRole('button', { name: 'New token' })).toBeInTheDocument();
    });

    it('token: buat dari dialog, rahasia sekali tampil, lalu revoke lewat konfirmasi', async () => {
        let created = false;
        const f = mount((u, init) => {
            if (u.endsWith('/me/tokens') && init?.method === 'POST') { created = true; return json({ ...token, secret: 'oat_abcdSECRETSECRET' }, 201); }
            if (u.endsWith('/me/tokens') && (!init?.method || init.method === 'GET')) return json({ items: created ? [token] : [] });
            if (u.includes('/me/tokens/t2') && init?.method === 'DELETE') return new Response(null, { status: 204 });
            return undefined;
        });
        const usr = userEvent.setup();
        await screen.findByText('You have no active tokens.');
        await usr.click(screen.getByRole('button', { name: 'New token' }));
        const dlg = await screen.findByRole('dialog');
        await usr.type(within(dlg).getByLabelText('Name'), 'CI');
        await usr.click(within(dlg).getByRole('button', { name: 'Create token' }));
        expect(JSON.parse(String(f.mock.calls.find(([u, i]) => String(u).endsWith('/me/tokens') && i?.method === 'POST')![1]!.body))).toEqual({ name: 'CI', expires_in_days: 90 });
        expect(await screen.findByTestId('api-token')).toHaveTextContent('oat_abcdSECRETSECRET');
        await usr.click(screen.getByRole('button', { name: 'Done' }));
        const list = await screen.findByRole('list', { name: 'API tokens' });
        expect(list.textContent).toContain('CI');
        expect(list.textContent).toContain('oat_abcd…');
        expect(list.textContent).not.toContain('SECRETSECRET');
        expect(list.textContent).toContain('never used');
        await usr.click(screen.getByRole('button', { name: 'Revoke CI' }));
        expect(f.mock.calls.some(([, i]) => i?.method === 'DELETE')).toBe(false); // menunggu konfirmasi
        await usr.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Revoke token' }));
        await waitFor(() => expect(f.mock.calls.some(([u, i]) => String(u).includes('/me/tokens/t2') && i?.method === 'DELETE')).toBe(true));
    });

    it('tombol Keluar memanggil POST /auth/logout', async () => {
        const f = mount((u) => (u.endsWith('/settings') ? json(settings) : u.endsWith('/me/tokens') ? json({ items: [] }) : u.endsWith('/auth/logout') ? new Response(null, { status: 204 }) : undefined));
        const usr = userEvent.setup();
        await screen.findByRole('button', { name: 'Sign out' });
        await usr.click(screen.getByRole('button', { name: 'Sign out' }));
        await waitFor(() => expect(f.mock.calls.some(([u, i]) => String(u).endsWith('/auth/logout') && i?.method === 'POST')).toBe(true));
    });

    it('galat memuat profil menampilkan pesan dan tombol coba lagi', async () => {
        vi.spyOn(globalThis, 'fetch').mockImplementation(async () => json({ title: 'x' }, 500));
        const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
        render(<QueryClientProvider client={qc}><MemoryRouter initialEntries={['/profile']}><ProfilePage/></MemoryRouter></QueryClientProvider>);
        expect(await screen.findByRole('alert')).toHaveTextContent("Couldn't load the profile");
        expect(screen.getByRole('button', { name: 'Try again' })).toBeInTheDocument();
    });
});

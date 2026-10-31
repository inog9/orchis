import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { AssetsPage } from './AssetsPage';

const me = { id: 'u', email: 'a@b.c', display_name: 'Ada', role: 'analyst', permissions: ['asset.read', 'asset.write'] };
const asset = (n: string, extra = {}) => ({
  id: n, category: 'host', name: n, description: null, platform: 'linux', arch: 'x86_64', criticality: 'high',
  is_internet_facing: false, testing_authorized: n === 'srv-ok', source: 'manual', attributes: {},
  addresses: [{ kind: 'ip', value: '10.0.0.5', is_primary: true }], tags: [], created_at: '2026-10-25T10:00:00Z', updated_at: '2026-10-25T10:00:00Z', version: 1, ...extra,
});
const json = (v: unknown, status = 200) => new Response(JSON.stringify(v), { status });

function route(handler: (url: string) => Response) {
  return vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
    const url = String(input);
    if (url.includes('/auth/me')) return json(me);
    return handler(url);
  });
}

function mount() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={['/assets']}>
        <AssetsPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

afterEach(() => vi.restoreAllMocks());

describe('AssetsPage', () => {
  it('menampilkan daftar aset dengan status otorisasi pengujian', async () => {
    route(() => json({ items: [asset('srv-ok'), asset('srv-no')], next_cursor: null }));
    mount();
    expect(await screen.findByText('srv-ok')).toBeInTheDocument();
    expect(screen.getByText('Authorized')).toBeInTheDocument();
    expect(screen.getByText('Not authorized')).toBeInTheDocument();
    expect(screen.getAllByText('10.0.0.5')).toHaveLength(2);
  });

  it('menampilkan keadaan kosong dengan ajakan menambah aset', async () => {
    route(() => json({ items: [], next_cursor: null }));
    mount();
    expect(await screen.findByText('No assets yet')).toBeInTheDocument();
    expect(screen.getAllByRole('button', { name: /add asset/i }).length).toBeGreaterThan(0);
  });

  it('menampilkan keadaan gagal dengan error_id dan bisa dicoba lagi', async () => {
    let calls = 0;
    route(() => {
      calls++;
      return calls === 1 ? json({ title: 'x', status: 500, error_id: 'err_7f3a91c2' }, 500) : json({ items: [asset('srv-ok')], next_cursor: null });
    });
    mount();
    expect(await screen.findByRole('alert')).toHaveTextContent('err_7f3a91c2');
    await userEvent.click(screen.getByRole('button', { name: /try again/i }));
    expect(await screen.findByText('srv-ok')).toBeInTheDocument();
  });

  it('mengirim pencarian dan filter kategori ke server, dan memuat halaman berikutnya', async () => {
    const spy = route((url) =>
      url.includes('cursor=abc') ? json({ items: [asset('hal-2')], next_cursor: null }) : json({ items: [asset('hal-1')], next_cursor: 'abc' }),
    );
    mount();
    expect(await screen.findByText('hal-1')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: /load more/i }));
    expect(await screen.findByText('hal-2')).toBeInTheDocument();
    await userEvent.selectOptions(screen.getByRole('combobox', { name: 'Category' }), 'web_application');
    await waitFor(() => expect(spy.mock.calls.some(([u]) => String(u).includes('category=web_application'))).toBe(true));
  });

  it('dialog tambah aset: pilih kategori lalu kirim; galat validasi ditampilkan di kolomnya', async () => {
    const spy = route((url) => json({ items: [], next_cursor: null }));
    mount();
    await userEvent.click((await screen.findAllByRole('button', { name: /add asset/i }))[0]!);
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(screen.getAllByRole('button').filter((b) => b.className.includes('category'))).toHaveLength(11);
    await userEvent.click(screen.getByRole('button', { name: /web application/i }));
    spy.mockImplementation(async (input, init) => {
      const url = String(input);
      if (url.includes('/auth/me')) return json(me);
      if (init?.method === 'POST')
        return json({ title: 'Invalid data', status: 422, error_id: 'err_1', fields: [{ name: 'name', message: 'Name is required (max 200 characters).' }] }, 422);
      return json({ items: [], next_cursor: null });
    });
    await userEvent.click(screen.getByRole('button', { name: /create asset/i }));
    expect(await screen.findByText('Name is required (max 200 characters).')).toBeInTheDocument();
    expect(screen.queryByLabelText(/I confirm this asset may be tested/i)).not.toBeInTheDocument(); // analyst tidak punya asset.authorize
  });
});

describe('AddAssetDialog', () => {
  it('menghapus galat validasi setelah pengguna mengubah isian', async () => {
    const spy = route(() => json({ items: [], next_cursor: null }));
    mount();
    await userEvent.click((await screen.findAllByRole('button', { name: /add asset/i }))[0]!);
    await userEvent.click(screen.getByRole('button', { name: /^Host/ }));
    spy.mockImplementation(async (input, init) => {
      const url = String(input);
      if (url.includes('/auth/me')) return json(me);
      if (init?.method === 'POST') return json({ title: 'Invalid data', status: 422, error_id: 'err_1', fields: [{ name: 'name', message: 'Name is required (max 200 characters).' }] }, 422);
      return json({ items: [], next_cursor: null });
    });
    await userEvent.click(screen.getByRole('button', { name: /create asset/i }));
    expect(await screen.findByText('Name is required (max 200 characters).')).toBeInTheDocument();
    await userEvent.type(screen.getByLabelText('Name', { exact: true }), 'srv-01');
    expect(screen.queryByText('Name is required (max 200 characters).')).not.toBeInTheDocument();
  });

  it('label kolom tidak memuat teks petunjuk (nama aksesibel bersih)', async () => {
    route(() => json({ items: [], next_cursor: null }));
    mount();
    await userEvent.click((await screen.findAllByRole('button', { name: /add asset/i }))[0]!);
    await userEvent.click(screen.getByRole('button', { name: /^Host/ }));
    expect(screen.getByRole('textbox', { name: 'Addresses' })).toHaveAccessibleDescription(/One per line/);
    expect(screen.getByRole('combobox', { name: 'Platform' })).toBeInTheDocument();
  });
});

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { AssetDetailPage } from './AssetDetailPage';
import { GroupDetailPage } from './GroupDetailPage';
import { GroupsPage } from './GroupsPage';
import { ImportDialog } from './ImportDialog';

const perms = (extra: string[] = []) => ['asset.read', 'asset.write', ...extra];
const meOf = (permissions: string[]) => ({ id: 'u', email: 'a@b.c', display_name: 'Ada', role: 'analyst', permissions });
const json = (v: unknown, status = 200) => new Response(JSON.stringify(v), { status });
const problem = (status: number, extra: object = {}) => json({ title: 't', detail: 'd', status, error_id: 'err_abc12345', ...extra }, status);

const asset = (over: object = {}) => ({
  id: 'a1', category: 'host', name: 'WINTERFELL', description: null, platform: 'windows', arch: 'x86_64', criticality: 'high',
  is_internet_facing: false, testing_authorized: true, source: 'manual', attributes: {}, version: 3,
  addresses: [{ kind: 'ip', value: '74.234.220.121', is_primary: true }], tags: [{ id: 't1', name: 'crown-jewel', color: '#FF0019' }],
  groups: [{ id: 'g1', name: 'All Windows x86_64', is_system: true }], created_at: '2026-10-25T10:00:00Z', updated_at: '2026-10-25T10:00:00Z', ...over,
});
const group = (over: object = {}) => ({
  id: 'g2', name: 'Crown jewels', description: 'main assets', kind: 'dynamic', is_system: false, member_count: 1, version: 2, total_assets: 3,
  created_at: '2026-10-25T10:00:00Z', updated_at: '2026-10-25T10:00:00Z', members_capped: false,
  rule: { mode: 'and', filters: [{ field: 'tag', operator: 'eq', values: ['crown-jewel'] }] },
  members: [{ id: 'a1', name: 'WINTERFELL', category: 'host', platform: 'windows', arch: 'x86_64', criticality: 'high' }], ...over,
});

type Handler = (url: string, init?: RequestInit) => Response | Promise<Response>;
function api(me: string[], handler: Handler) {
  return vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
    const url = String(input);
    if (url.endsWith('/auth/me')) return json(meOf(me));
    if (url.includes('/api/v1/tags') && !init?.method) return json({ items: [{ id: 't1', name: 'crown-jewel', color: '#FF0019' }] });
    return handler(url, init);
  });
}
function mount(ui: React.ReactNode, path = '/') {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={qc}><MemoryRouter initialEntries={[path]}>{ui}</MemoryRouter></QueryClientProvider>);
}
const detailRoute = (el: React.ReactNode, p: string) => <Routes><Route path={p} element={el} /><Route path="*" element={<p>dialihkan</p>} /></Routes>;

afterEach(() => vi.restoreAllMocks());

describe('AssetDetailPage', () => {
  it('menampilkan detail, tag, alamat dan grup anggota', async () => {
    api(perms(), () => json(asset()));
    mount(detailRoute(<AssetDetailPage />, '/assets/:id'), '/assets/a1');
    expect(await screen.findByRole('heading', { name: 'WINTERFELL' })).toBeInTheDocument();
    expect(screen.getByText('crown-jewel')).toBeInTheDocument();
    expect(screen.getByText('74.234.220.121')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'All Windows x86_64' })).toHaveAttribute('href', '/assets/groups/g1');
    expect(screen.getByText('Authorized')).toBeInTheDocument();
    expect(screen.getByText(/Your role cannot read findings/)).toBeInTheDocument();
    expect(screen.queryByText(/No findings yet/)).not.toBeInTheDocument();
  });

  it('menampilkan detail khusus kategori (atribut dan catatan) dengan label yang terbaca', async () => {
    api(perms(), () => json(asset({ category: 'cloud_resource', name: 'prod-bucket', platform: null, arch: null, description: 'Holds invoices', attributes: { provider: 'aws', account_id: '123456789012', resource_type: 'S3 bucket', resource_id: 'arn:aws:s3:::prod-bucket' }, addresses: [] })));
    mount(detailRoute(<AssetDetailPage />, '/assets/:id'), '/assets/a1');
    await screen.findByRole('heading', { name: 'prod-bucket' });
    const panel = screen.getByRole('heading', { name: 'Cloud resource', level: 2 }).closest('section')!;
    expect(panel).toHaveTextContent('Provider');
    expect(panel).toHaveTextContent('AWS');
    expect(panel).toHaveTextContent('123456789012');
    expect(panel).toHaveTextContent('arn:aws:s3:::prod-bucket');
    expect(panel).toHaveTextContent('Holds invoices');
  });

  it('aset agent: versi OS dari agent terisi otomatis; peran dan lokasi dijelaskan sebagai isian manual', async () => {
    api(perms(), () => json(asset({ source: 'agent', platform: 'macos', arch: 'x86_64', attributes: { agent_os_version: 'macOS 10.15.7' } })));
    mount(detailRoute(<AssetDetailPage />, '/assets/:id'), '/assets/a1');
    await screen.findByRole('heading', { name: 'WINTERFELL' });
    const panel = screen.getByRole('heading', { name: 'Host', level: 2 }).closest('section')!;
    expect(panel).toHaveTextContent('macOS 10.15.7');
    expect(panel).toHaveTextContent('(from the agent)');
    expect(panel).toHaveTextContent('Not set. Add it with Edit.');
    expect(panel).toHaveTextContent(/Role and location can.t be detected/);
    expect(panel).not.toHaveTextContent('—');
  });

  it('versi OS isian manual menang atas laporan agent; agent yang belum melapor diberi keterangan', async () => {
    const spy = api(perms(), () => json(asset({ source: 'agent', attributes: { os_version: 'Custom OS 1', agent_os_version: 'macOS 10.15.7' } })));
    const { unmount } = mount(detailRoute(<AssetDetailPage />, '/assets/:id'), '/assets/a1');
    await screen.findByRole('heading', { name: 'WINTERFELL' });
    let panel = screen.getByRole('heading', { name: 'Host', level: 2 }).closest('section')!;
    expect(panel).toHaveTextContent('Custom OS 1');
    expect(panel).not.toHaveTextContent('macOS 10.15.7');
    unmount();
    spy.mockRestore();
    api(perms(), () => json(asset({ source: 'agent', attributes: {} })));
    mount(detailRoute(<AssetDetailPage />, '/assets/:id'), '/assets/a1');
    await screen.findByRole('heading', { name: 'WINTERFELL' });
    panel = screen.getByRole('heading', { name: 'Host', level: 2 }).closest('section')!;
    expect(panel).toHaveTextContent("Waiting for the agent's next report.");
  });

  it('dialog ubah aset agent: platform dan arsitektur hanya baca, versi OS dari agent jadi petunjuk dan tetap tersimpan', async () => {
    const spy = api(perms(), () => json(asset({ source: 'agent', testing_authorized: false, platform: 'macos', arch: 'x86_64', addresses: [], attributes: { agent_os_version: 'macOS 10.15.7' } })));
    mount(detailRoute(<AssetDetailPage />, '/assets/:id'), '/assets/a1');
    await userEvent.click(await screen.findByRole('button', { name: /edit/i }));
    expect(screen.getByLabelText('Platform')).toBeDisabled();
    expect(screen.getByLabelText('Architecture')).toBeDisabled();
    expect(screen.getByText(/Reported by the agent: macOS 10.15.7/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: /save changes/i }));
    await waitFor(() => expect(spy.mock.calls.some(([, i]) => i?.method === 'PATCH')).toBe(true));
    const [, init] = spy.mock.calls.find(([, i]) => i?.method === 'PATCH')!;
    const body = JSON.parse(init!.body as string);
    expect(body.attributes.agent_os_version).toBe('macOS 10.15.7');
    expect(body.attributes.os_version).toBeUndefined();
  });

  it('tombol ubah dan hapus mengikuti izin', async () => {
    api(['asset.read'], () => json(asset()));
    mount(detailRoute(<AssetDetailPage />, '/assets/:id'), '/assets/a1');
    await screen.findByRole('heading', { name: 'WINTERFELL' });
    expect(screen.queryByRole('button', { name: /edit/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /remove asset/i })).not.toBeInTheDocument();
  });

  it('menampilkan 404 yang jelas', async () => {
    api(perms(), () => problem(404));
    mount(detailRoute(<AssetDetailPage />, '/assets/:id'), '/assets/x');
    expect(await screen.findByText('Asset not found')).toBeInTheDocument();
  });

  it('konflik versi: menampilkan pesan dan memuat ulang versi terbaru', async () => {
    let gets = 0;
    api(perms(), (url, init) => {
      if (init?.method === 'PATCH') return problem(409, { detail: 'This asset was changed by someone else. Reload and try again.', code: 'asset_conflict' });
      gets++;
      return json(asset({ version: gets === 1 ? 3 : 4, name: gets === 1 ? 'WINTERFELL' : 'WINTERFELL-2' }));
    });
    mount(detailRoute(<AssetDetailPage />, '/assets/:id'), '/assets/a1');
    await userEvent.click(await screen.findByRole('button', { name: /edit/i }));
    await userEvent.click(screen.getByRole('button', { name: /save changes/i }));
    expect(await screen.findByText('This asset changed')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: /reload the latest version/i }));
    expect(await screen.findByRole('heading', { name: 'WINTERFELL-2' })).toBeInTheDocument();
  });

  it('mengirim versi dan hanya kolom yang relevan saat menyimpan', async () => {
    const spy = api(perms(), (url, init) => (init?.method === 'PATCH' ? json(asset()) : json(asset())));
    mount(detailRoute(<AssetDetailPage />, '/assets/:id'), '/assets/a1');
    await userEvent.click(await screen.findByRole('button', { name: /edit/i }));
    const name = screen.getByLabelText('Name', { exact: true });
    await userEvent.clear(name);
    await userEvent.type(name, 'NEW-NAME');
    await userEvent.click(screen.getByRole('button', { name: /save changes/i }));
    await waitFor(() => expect(spy.mock.calls.some(([, i]) => i?.method === 'PATCH')).toBe(true));
    const [, init] = spy.mock.calls.find(([, i]) => i?.method === 'PATCH')!;
    const body = JSON.parse(init!.body as string);
    expect(body).toMatchObject({ version: 3, name: 'NEW-NAME', platform: 'windows', arch: 'x86_64', tag_ids: ['t1'] });
    expect(body.testing_authorized).toBeUndefined(); // tanpa izin asset.authorize tidak dikirim
  });

  it('aset dari agent boleh disimpan tanpa alamat (alamat bukan wajib)', async () => {
    const spy = api(perms(), (url, init) => json(asset({ source: 'agent', testing_authorized: false, addresses: [] })));
    mount(detailRoute(<AssetDetailPage />, '/assets/:id'), '/assets/a1');
    await userEvent.click(await screen.findByRole('button', { name: /edit/i }));
    expect(screen.getByText(/created by an agent/i)).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: /save changes/i }));
    await waitFor(() => expect(spy.mock.calls.some(([, i]) => i?.method === 'PATCH')).toBe(true));
    expect(screen.queryByText(/add at least one address/i)).toBeNull();
  });

  it('aset manual tetap wajib punya alamat', async () => {
    const spy = api(perms(), () => json(asset({ testing_authorized: false, addresses: [] })));
    mount(detailRoute(<AssetDetailPage />, '/assets/:id'), '/assets/a1');
    await userEvent.click(await screen.findByRole('button', { name: /edit/i }));
    await userEvent.click(screen.getByRole('button', { name: /save changes/i }));
    expect(await screen.findByText(/add at least one address/i)).toBeInTheDocument();
    expect(spy.mock.calls.some(([, i]) => i?.method === 'PATCH')).toBe(false);
  });

  it('hapus meminta konfirmasi lalu kembali ke daftar', async () => {
    const spy = api(perms(['asset.delete']), (url, init) => (init?.method === 'DELETE' ? new Response(null, { status: 204 }) : json(asset())));
    mount(detailRoute(<AssetDetailPage />, '/assets/:id'), '/assets/a1');
    await userEvent.click(await screen.findByRole('button', { name: /remove asset/i }));
    expect(spy.mock.calls.some(([, i]) => i?.method === 'DELETE')).toBe(false); // belum dikirim sebelum konfirmasi
    const dlg = screen.getByRole('dialog');
    await userEvent.click(within(dlg).getByRole('button', { name: /remove asset/i }));
    expect(await screen.findByText('dialihkan')).toBeInTheDocument();
  });
});

describe('Groups', () => {
  it('daftar grup menampilkan aturan dalam kalimat dan jumlah anggota', async () => {
    api(perms(), () => json({ items: [group({ id: 'g1', name: 'All endpoints', is_system: true, member_count: 3, rule: { mode: 'and', filters: [{ field: 'platform', operator: 'is_not_empty', values: [] }] } }), group()] }));
    mount(<GroupsPage />, '/assets/groups');
    expect(await screen.findByRole('link', { name: 'All endpoints' })).toBeInTheDocument();
    expect(screen.getByText('Platform is not empty')).toBeInTheDocument();
    expect(screen.getByText('Tag is crown-jewel')).toBeInTheDocument();
    expect(screen.getByText('Built-in')).toBeInTheDocument();
  });

  it('membuat grup: pratinjau langsung dan nilai kosong ditolak sebelum dikirim', async () => {
    const spy = api(perms(), (url, init) => {
      if (url.endsWith('/preview')) return json({ count: 2, total: 5, sample: [{ id: 'a', name: 'srv-1' }, { id: 'b', name: 'srv-2' }] });
      if (init?.method === 'POST') return json(group());
      return json({ items: [] });
    });
    mount(detailRoute(<GroupsPage />, '/assets/groups'), '/assets/groups');
    await userEvent.click(await screen.findByRole('button', { name: /new group/i }));
    const dlg = screen.getByRole('dialog');
    await userEvent.type(within(dlg).getByLabelText('Name', { exact: true }), 'Linux boxes');
    await userEvent.click(within(dlg).getByRole('button', { name: /add rule/i }));
    await userEvent.selectOptions(within(dlg).getByLabelText('Rule 1 field'), 'hostname');
    await userEvent.selectOptions(within(dlg).getByLabelText('Rule 1 operator'), 'contains');
    expect(within(dlg).getByRole('status')).toHaveTextContent('Enter a value for “Hostname”.');
    expect(within(dlg).getByRole('button', { name: /create group/i })).toBeDisabled();
    await userEvent.type(within(dlg).getByLabelText('Rule 1 value'), 'srv');
    await waitFor(() => expect(within(dlg).getByRole('status')).toHaveTextContent('2 of 5 assets match'));
    expect(within(dlg).getByRole('status')).toHaveTextContent('srv-1, srv-2');
    await userEvent.click(within(dlg).getByRole('button', { name: /create group/i }));
    await waitFor(() => expect(spy.mock.calls.some(([u, i]) => i?.method === 'POST' && String(u).endsWith('/asset-groups'))).toBe(true));
    const [, init] = spy.mock.calls.find(([u, i]) => i?.method === 'POST' && String(u).endsWith('/asset-groups'))!;
    expect(JSON.parse(init!.body as string)).toEqual({ name: 'Linux boxes', description: null, rule: { mode: 'and', filters: [{ field: 'hostname', operator: 'contains', values: ['srv'] }] } });
  });

  it('grup bawaan tidak punya tombol ubah atau hapus', async () => {
    api(perms(['asset.delete']), () => json(group({ is_system: true, name: 'All endpoints' })));
    mount(detailRoute(<GroupDetailPage />, '/assets/groups/:id'), '/assets/groups/g1');
    await screen.findByRole('heading', { name: 'All endpoints' });
    expect(screen.queryByRole('button', { name: /edit/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /delete group/i })).not.toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'WINTERFELL' })).toHaveAttribute('href', '/assets/a1');
  });

  it('grup kustom bisa diubah dan dihapus, dengan keadaan kosong yang jelas', async () => {
    api(perms(['asset.delete']), () => json(group({ members: [], member_count: 0 })));
    mount(detailRoute(<GroupDetailPage />, '/assets/groups/:id'), '/assets/groups/g2');
    await screen.findByRole('heading', { name: 'Crown jewels' });
    expect(screen.getByText('No assets match yet')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /edit/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /delete group/i })).toBeInTheDocument();
  });
});

describe('ImportDialog', () => {
  const preview = (over: object = {}) => ({
    columns: [{ name: 'hostname', sample: ['srv-1'] }, { name: 'os', sample: ['linux'] }], suggested_mapping: { hostname: 'host', os: 'platform' },
    rows: 3, validated: false, valid: 0, invalid: [], duplicates: [], ...over,
  });
  const file = (text: string) => new File([text], 'aset.csv', { type: 'text/csv' });

  it('alur: pilih berkas, lihat pemetaan, ringkasan, lewati baris tak valid, impor', async () => {
    const spy = api(perms(), (url, init) => {
      const body = init?.body ? JSON.parse(init.body as string) : {};
      if (url.endsWith('/import/preview')) {
        return json(body.mapping
          ? preview({ validated: true, valid: 2, invalid: [{ row: 4, message: 'platform "solaris" isn\'t supported. Use windows, linux or macos.' }], duplicates: [{ row: 3, message: 'srv-1 already exists.' }] })
          : preview());
      }
      if (url.endsWith('/assets/import')) return json({ created: 2, skipped_invalid: 1, skipped_duplicates: 1, tags_created: 0 }, 201);
      return json({});
    });
    mount(<ImportDialog onClose={() => {}} />);
    await userEvent.upload(screen.getByLabelText(/choose a csv file/i), file('hostname,os\nsrv-1,linux\n'));
    expect(await screen.findByText('Match your columns')).toBeInTheDocument();
    expect(screen.getByLabelText('Import column hostname as')).toHaveValue('host');
    expect(screen.getByLabelText('Import column os as')).toHaveValue('platform');
    expect(await screen.findByText('2 valid')).toBeInTheDocument();
    expect(screen.getByText('1 need fixing')).toBeInTheDocument();
    expect(screen.getByText(/isn't supported. Use windows, linux or macos/)).toBeInTheDocument();
    const go = screen.getByRole('button', { name: /import 2 assets/i });
    expect(go).toBeDisabled(); // ada baris yang perlu diperbaiki: harus memilih melewatinya
    await userEvent.click(screen.getByLabelText(/skip the rows that need fixing/i));
    expect(go).toBeEnabled();
    await userEvent.click(go);
    expect(await screen.findByText('Imported 2 assets')).toBeInTheDocument();
    expect(screen.getByText(/not authorized for testing/i)).toBeInTheDocument();
    const call = spy.mock.calls.find(([u]) => String(u).endsWith('/assets/import'))!;
    expect(JSON.parse(call[1]!.body as string)).toMatchObject({ skip_invalid: true, mapping: { hostname: 'host', os: 'platform' } });
  });

  it('menolak berkas yang terlalu besar tanpa mengirim ke server', async () => {
    const spy = api(perms(), () => json(preview()));
    mount(<ImportDialog onClose={() => {}} />);
    const big = new File(['x'], 'besar.csv', { type: 'text/csv' });
    Object.defineProperty(big, 'size', { value: 2 * 1024 * 1024 });
    await userEvent.upload(screen.getByLabelText(/choose a csv file/i), big);
    expect(await screen.findByRole('alert')).toHaveTextContent('too large');
    expect(spy.mock.calls.some(([u]) => String(u).includes('/import'))).toBe(false);
  });

  it('menampilkan galat server dengan pesan yang bisa dipahami', async () => {
    api(perms(), (url) => (url.endsWith('/import/preview') ? problem(422, { fields: [{ name: 'csv', message: 'The file is empty.', code: 'file_empty' }] }) : json({})));
    mount(<ImportDialog onClose={() => {}} />);
    await userEvent.upload(screen.getByLabelText(/choose a csv file/i), file('x'));
    expect(await screen.findByText('The file is empty.')).toBeInTheDocument();
  });
});

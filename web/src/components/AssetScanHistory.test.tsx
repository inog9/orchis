import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, expect, it, vi } from 'vitest';
import type { Scan } from '../api/scans';
import { AssetScanHistory, scansForAsset } from './AssetScanHistory';

const scan = (id: string, asset_ids: string[], status = 'finished'): Scan => ({ id, organization_id: 'o1', name: 'n', status, runner: 'injector', options: { organization_id: 'o1', asset_ids, range_ids: [], tools: ['nmap'], preset: 'quick' }, created_at: '2026-01-01T00:00:00Z', ended_at: '2026-01-01T00:01:00Z' });
afterEach(() => vi.restoreAllMocks());
const mount = (allowed = true, org: string | null = 'o1') => render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><MemoryRouter><AssetScanHistory assetID="a1" organizationID={org} allowed={allowed} /></MemoryRouter></QueryClientProvider>);

it('scansForAsset hanya mengambil scan yang memuat aset', () => {
  expect(scansForAsset([scan('s1', ['a1']), scan('s2', ['a2']), scan('s3', [])], 'a1').map((s) => s.id)).toEqual(['s1']);
});
it('menampilkan riwayat dari /scans organisasi yang disaring menurut aset', async () => {
  const f = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({ items: [scan('aaaaaaaa-1111-2222-3333-444444444444', ['a1']), scan('bbbbbbbb-1111-2222-3333-444444444444', ['zz'])], next_cursor: null })));
  mount();
  expect(await screen.findByRole('link', { name: 'aaaaaaaa-1111' })).toHaveAttribute('href', '/simulations/aaaaaaaa-1111-2222-3333-444444444444');
  expect(screen.queryByRole('link', { name: 'bbbbbbbb-1111' })).toBeNull();
  expect(String(f.mock.calls[0]![0])).toContain('/api/v1/scans?org=o1');
  expect(screen.getByText('Finished')).toBeInTheDocument();
});
it('kosong menjelaskan batas 25 scan; tanpa izin atau organisasi tidak ada panel dan tidak memanggil API', async () => {
  const f = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({ items: [], next_cursor: null })));
  const { unmount } = mount();
  expect(await screen.findByText(/latest 25 scans/)).toBeInTheDocument();
  unmount(); f.mockClear();
  const r = mount(false); expect(r.container.firstChild).toBeNull(); r.unmount();
  mount(true, null);
  expect(f).not.toHaveBeenCalled();
});

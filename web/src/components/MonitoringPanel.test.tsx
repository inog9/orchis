import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { MonitoringPanel } from './MonitoringPanel';

const json = (v: unknown, status = 200) => new Response(JSON.stringify(v), { status, headers: { 'Content-Type': 'application/json' } });
afterEach(() => vi.restoreAllMocks());
function mount(items: object[]) {
  const urls: string[] = [];
  vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => { urls.push(String(input)); return json({ items, next_cursor: null, search: '' }); });
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><MemoryRouter><MonitoringPanel assetID="a1" allowed/></MemoryRouter></QueryClientProvider>);
  return urls;
}
const finding = (over: object) => ({ id: 'f1', asset_id: 'a1', asset_name: 'host', type: 'c2_suspect_connection', value: '203.0.113.9', title: 'Suspicious outbound connections to 203.0.113.9 (12 conns, ports [8443])', severity: 'high',
  details: { dst: '203.0.113.9', ports: [8443], processes: ['python.exe'], connections: 12 }, first_seen_at: '2026-11-13T10:00:00Z', last_seen_at: '2026-11-13T10:05:00Z', occurrence_count: 3, ...over });

describe('MonitoringPanel', () => {
  it('shows an empty state that says what will appear', async () => {
    const urls = mount([]);
    expect(await screen.findByText(/No detections/)).toBeInTheDocument();
    expect(urls[0]).toContain('type=monitoring');
    expect(urls[0]).toContain('asset_id=a1');
  });
  it('lists each detection with its type, destination, ports and process', async () => {
    mount([finding({}), finding({ id: 'f2', type: 'new_listening_port', title: 'New listening port 9999', severity: 'high', details: { row: { port: '9999', address: '0.0.0.0', process: 'python.exe' } } })]);
    expect(await screen.findByText(/Suspicious outbound connections to 203/)).toBeInTheDocument();
    expect(screen.getByText(/^Suspicious outbound connection ·/)).toBeInTheDocument();
    expect(screen.getByText('203.0.113.9')).toBeInTheDocument();
    expect(screen.getByText('8443')).toBeInTheDocument();
    expect(screen.getByText(/^New listening port ·/)).toBeInTheDocument();
    expect(screen.getByText(/port=9999 address=0.0.0.0 process=python.exe/)).toBeInTheDocument();
    expect(screen.getByText('2 detections')).toBeInTheDocument();
  });
});

import { useEffect, useState } from 'react';
import { useQueryClient, type QueryClient } from '@tanstack/react-query';

type Subscription = {source: EventSource; connected: boolean; listeners: Set<(ready: boolean) => void>};
const subscriptions = new Map<string, Subscription>();

// One connection per asset is shared by all mounted consumers. EventSource handles reconnects.
function subscribe(asset: string, client: QueryClient, update: (ready: boolean) => void): () => void {
  if (typeof EventSource === 'undefined') { update(false); return () => {}; }
  let sub = subscriptions.get(asset);
  if (!sub) {
    const source = new EventSource(`/api/v1/assets/${encodeURIComponent(asset)}/endpoint-data/stream`);
    sub = { source, connected: false, listeners: new Set() };
    subscriptions.set(asset, sub);
    const active = sub;
    const set = (value: boolean) => { active.connected = value; active.listeners.forEach(fn => fn(value)); };
    const refresh = () => { void client.invalidateQueries({queryKey:['asset-endpoint',asset]}); void client.invalidateQueries({queryKey:['findings']}); };
    source.onopen = () => { set(true); refresh(); };
    source.onerror = () => set(false);
    source.addEventListener('endpoint-data-updated', refresh);
  }
  sub.listeners.add(update); update(sub.connected);
  const active = sub;
  return () => { active.listeners.delete(update); if (active.listeners.size === 0) { active.source.close(); subscriptions.delete(asset); } };
}

export function useEndpointStream(asset: string, enabled: boolean): boolean {
  const client = useQueryClient();
  const [connected,setConnected] = useState(false);
  useEffect(() => { if (!enabled || !asset) { setConnected(false); return; } return subscribe(asset,client,setConnected); },[asset,enabled,client]);
  return connected;
}

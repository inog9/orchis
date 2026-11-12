import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { useEndpointStream } from './endpointStream';

class MockEventSource {
 static instances: MockEventSource[] = [];
 onopen: (() => void) | null = null;
 onerror: (() => void) | null = null;
 handlers = new Map<string,() => void>();
 close = vi.fn();
 constructor(public url: string) { MockEventSource.instances.push(this); }
 addEventListener(type: string, fn: () => void) { this.handlers.set(type,fn); }
}
afterEach(() => { vi.unstubAllGlobals(); MockEventSource.instances=[]; });
it('shares one SSE connection, refreshes datasets and findings, and closes on unmount', () => {
 vi.stubGlobal('EventSource',MockEventSource);
 const client=new QueryClient();const invalidate=vi.spyOn(client,'invalidateQueries').mockResolvedValue();
 const wrapper=({children}:{children:React.ReactNode})=><QueryClientProvider client={client}>{children}</QueryClientProvider>;
 const a=renderHook(()=>useEndpointStream('a1',true),{wrapper});
 const b=renderHook(()=>useEndpointStream('a1',true),{wrapper});
 expect(MockEventSource.instances).toHaveLength(1);const source=MockEventSource.instances[0]!;
 expect(source.url).toBe('/api/v1/assets/a1/endpoint-data/stream');
 act(()=>source.onopen?.());expect(a.result.current).toBe(true);expect(b.result.current).toBe(true);
 invalidate.mockClear();act(()=>source.handlers.get('endpoint-data-updated')?.());
 expect(invalidate).toHaveBeenCalledTimes(2);
 expect(invalidate).toHaveBeenCalledWith({queryKey:['asset-endpoint','a1']});expect(invalidate).toHaveBeenCalledWith({queryKey:['findings']});
 act(()=>source.onerror?.());expect(a.result.current).toBe(false);
 a.unmount();expect(source.close).not.toHaveBeenCalled();b.unmount();expect(source.close).toHaveBeenCalledOnce();
});

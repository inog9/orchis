import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, expect, it, vi } from 'vitest';
import { AssetFindings } from './AssetFindings';
function mount(allowed=true){return render(<QueryClientProvider client={new QueryClient({defaultOptions:{queries:{retry:false}}})}><MemoryRouter><AssetFindings assetID="asset-one" allowed={allowed}/></MemoryRouter></QueryClientProvider>);}
const page={items:[{id:'f1',title:'Critical demo finding',severity:'critical',last_seen_at:'2026-10-30T00:00:00Z'}],next_cursor:''};
afterEach(()=>vi.restoreAllMocks());
it('shows actual findings scoped to the asset and links to its filtered list',async()=>{
 const fetch=vi.spyOn(globalThis,'fetch').mockResolvedValue(new Response(JSON.stringify(page)));mount();
 await waitFor(()=>expect(screen.queryByRole('status')).toBeNull());
 expect(screen.queryByText('Critical demo finding')).not.toBeNull();
 expect(fetch).toHaveBeenCalledWith('/api/v1/findings?asset_id=asset-one&limit=5',expect.anything());
 expect(screen.getByRole('link')).toHaveAttribute('href','/findings?asset_id=asset-one');
 expect(screen.queryByText(/No findings recorded/)).toBeNull();
});
it('explains an empty result without claiming the asset is safe',async()=>{
 vi.spyOn(globalThis,'fetch').mockResolvedValue(new Response(JSON.stringify({items:[]})));mount();
 expect(await screen.findByText(/Failed or skipped checks do not prove/)).toBeInTheDocument();
});
it('distinguishes a load failure and retries it',async()=>{
 const fetch=vi.spyOn(globalThis,'fetch').mockResolvedValueOnce(new Response('{}',{status:500})).mockResolvedValueOnce(new Response(JSON.stringify(page)));mount();
 expect(await screen.findByRole('alert')).toHaveTextContent('Couldn’t load');
 expect(screen.queryByText(/No findings recorded/)).toBeNull();
 await userEvent.setup().click(screen.getByRole('button',{name:'Retry asset findings'}));
 expect(await screen.findByText('Critical demo finding')).toBeInTheDocument();expect(fetch).toHaveBeenCalledTimes(2);
});
it('does not request findings without permission',()=>{
 const fetch=vi.spyOn(globalThis,'fetch');mount(false);
 expect(screen.getByText(/Your role cannot read findings/)).toBeInTheDocument();expect(fetch).not.toHaveBeenCalled();expect(screen.queryByRole('link')).toBeNull();
});

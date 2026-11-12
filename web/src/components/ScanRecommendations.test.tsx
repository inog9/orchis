import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { ScanRecommendations } from './ScanRecommendations';
const input={coverage:'incomplete',omitted:12,findings:[{id:'f1',title:'<script>fixture</script>',severity:'critical'}]};
const output={summary:'Prioritise the recorded injection issue.',items:[{finding_id:'f1',priority:'now',reason:'Critical observed issue.',action:'Add parameterised queries.',validation:'Retest the same endpoint.'}]};
function mount(job:unknown){vi.spyOn(globalThis,'fetch').mockResolvedValue(Response.json({job}));return render(<QueryClientProvider client={new QueryClient({defaultOptions:{queries:{retry:false}}})}><ScanRecommendations scanID="s1"/></QueryClientProvider>)}
afterEach(()=>vi.restoreAllMocks());
it('shows advice and verification separately from evidence, escapes finding titles and discloses coverage limits',async()=>{
 const {container}=mount({status:'ready',input,output});
 expect(await screen.findByText(output.summary)).toBeVisible();
 expect(screen.getByText('Address now')).toBeVisible();
 expect(screen.getByText(output.items[0]!.action)).toBeVisible();
 expect(screen.getByText(/12 additional findings remain/)).toBeVisible();
 expect(screen.getByText(/This scan has incomplete coverage/)).toBeVisible();
 expect(screen.getByText('<script>fixture</script>')).toBeVisible();expect(container.querySelector('script')).toBeNull();
 expect(screen.getByText('How to verify the fix')).toBeVisible();
});
it.each(['queued','running'])('shows %s without presenting a recommendation as complete',async status=>{
 mount({status,input});expect(await screen.findByText(status==='queued'?'Queued':'Reviewing')).toBeVisible();
 expect(screen.queryByText('Address now')).toBeNull();
});
it('keeps scan results available when a model review fails',async()=>{
 mount({status:'failed',input});expect(await screen.findByText(/OpenCode could not complete a valid review/)).toBeVisible();
});
it('explains why an old scan has no automatic review',async()=>{
 mount(null);expect(await screen.findByText(/after this feature was enabled/)).toBeVisible();
});

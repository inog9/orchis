import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { ScanControls } from './ScanControls';
const org='11111111-1111-4111-8111-111111111111';
function mount(permissions:string[]){
 let paused=false;let version=0;
 const fetch=vi.spyOn(globalThis,'fetch').mockImplementation(async(input,init)=>{
  const path=String(input);if(path.includes('/auth/me'))return Response.json({permissions});
  if(init?.method==='POST'){paused=path.endsWith('/stop');version++;}
  return Response.json({paused,global_paused:false,window:{},version,allowed_now:!paused,paused_by:paused?'Administrator':undefined,restriction:paused?'Administrator must resume scanning.':undefined});
 });
 render(<QueryClientProvider client={new QueryClient({defaultOptions:{queries:{retry:false}}})}><ScanControls org={org}/></QueryClientProvider>);return fetch;
}
afterEach(()=>vi.restoreAllMocks());
it('requires explicit confirmation, then displays the hold and resumes without a launch request',async()=>{
 const fetch=mount(['organization.read','settings.manage']);const user=userEvent.setup();
 await user.click(await screen.findByRole('button',{name:'Stop all scans'}));
 expect(fetch.mock.calls.filter(([,init])=>init?.method==='POST')).toHaveLength(0);
 await user.click(screen.getByRole('button',{name:'Keep scanning'}));
 expect(fetch.mock.calls.filter(([,init])=>init?.method==='POST')).toHaveLength(0);
 await user.click(screen.getByRole('button',{name:'Stop all scans'}));await user.click(screen.getByRole('button',{name:'Confirm stop all scans'}));
 await screen.findByText('Scanning held',{exact:true});expect(screen.getByText(/Stopped by Administrator/)).toBeVisible();
 await user.click(screen.getByRole('button',{name:'Resume scanning'}));
 await waitFor(()=>expect(screen.queryByText('Scanning held',{exact:true})).not.toBeInTheDocument());
 expect(fetch.mock.calls.filter(([,init])=>init?.method==='POST').map(([path])=>String(path))).toEqual([`/api/v1/organizations/${org}/scan-control/stop`,`/api/v1/organizations/${org}/scan-control/resume`]);
});
it('lets an Analyst read policy without offering administrative controls',async()=>{
 mount(['organization.read']);await screen.findByText('No time restriction. Scans can run at any time unless held.');
 expect(screen.queryByRole('button',{name:'Stop all scans'})).not.toBeInTheDocument();expect(screen.queryByRole('button',{name:'Set scan window'})).not.toBeInTheDocument();
});

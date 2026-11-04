import {QueryClient,QueryClientProvider} from '@tanstack/react-query';
import {render,screen,waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {MemoryRouter,Route,Routes} from 'react-router-dom';
import {afterEach,expect,it,vi} from 'vitest';
import {RetentionPanel,type RetentionPolicy} from './RetentionPanel';
import {DeleteOrganization,type DeletePreview} from './DeleteOrganization';
const policy:RetentionPolicy={findings_days:180,scan_days:90,report_days:180,endpoint_days:30,audit_days:365,version:4,cleanup_pending:0};
const org='11111111-1111-4111-8111-111111111111';
const preview:DeletePreview={id:org,name:'Fixture organisation',version:2,counts:{assets:2,agents:2,findings:3,reports:1,endpoint_datasets:6,scans:1,executions:2},blockers:[]};
function mount(ui:React.ReactNode){render(<QueryClientProvider client={new QueryClient({defaultOptions:{queries:{retry:false},mutations:{retry:false}}})}><MemoryRouter>{ui}</MemoryRouter></QueryClientProvider>)}
afterEach(()=>vi.restoreAllMocks());
it('sends the frozen policy version and refuses an audit retention below 90 days',async()=>{
 const fetch=vi.spyOn(globalThis,'fetch').mockImplementation(async(_,init)=>Response.json(init?.method==='PUT'?{...policy,version:5}:policy));
 mount(<RetentionPanel/>);const user=userEvent.setup();const audit=await screen.findByLabelText('Audit log (days)');expect(audit).toHaveAttribute('min','90');
 await user.clear(audit);await user.type(audit,'89');await user.click(screen.getByRole('button',{name:'Save retention'}));expect(fetch.mock.calls.filter(([,i])=>i?.method==='PUT')).toHaveLength(0);
 await user.clear(audit);await user.type(audit,'90');await user.click(screen.getByRole('button',{name:'Save retention'}));await screen.findByText('Retention settings saved.');
 const save=fetch.mock.calls.find(([,i])=>i?.method==='PUT');expect(JSON.parse(String(save?.[1]?.body))).toMatchObject({version:4,audit_days:90});
});
it('requires the exact organisation name, submits preview counts, and shows external cleanup pending',async()=>{
 const fetch=vi.spyOn(globalThis,'fetch').mockImplementation(async(input,init)=>{const path=String(input);if(path.endsWith('/deletion-preview'))return Response.json(preview);if(init?.method==='DELETE')return Response.json({id:'operation',organization_name:preview.name,status:'external_cleanup_pending',cleanup_pending:3});if(path.includes('/data-deletions/'))return Response.json({id:'operation',organization_name:preview.name,status:'external_cleanup_pending',cleanup_pending:3});return Response.json(policy)});
 mount(<Routes><Route path="/" element={<DeleteOrganization org={org}/>}/><Route path="/settings" element={<RetentionPanel/>}/></Routes>);const user=userEvent.setup();await user.click(screen.getByRole('button',{name:'Review deletion'}));const input=await screen.findByLabelText('Type the exact organisation name to confirm');
 const button=screen.getByRole('button',{name:'Delete organisation'});expect(button).toBeDisabled();await user.type(input,'fixture organisation');expect(button).toBeDisabled();await user.clear(input);await user.type(input,preview.name);expect(button).toBeEnabled();await user.click(button);
 await screen.findByText(/External cleanup is pending \(3 items\)/);expect(screen.queryByText(/external artifacts have been deleted/)).not.toBeInTheDocument();
 const call=fetch.mock.calls.find(([,i])=>i?.method==='DELETE');expect(JSON.parse(String(call?.[1]?.body))).toEqual({name:preview.name,version:preview.version,counts:preview.counts});expect(call?.[1]?.headers).toMatchObject({'X-Requested-With':'orchis-web'});
});
it('blocks deletion when a retained reference or active renderer is reported',async()=>{
 const fetch=vi.spyOn(globalThis,'fetch').mockResolvedValue(Response.json({...preview,blockers:['A PDF is rendering.']}));
 mount(<DeleteOrganization org={org}/>);const user=userEvent.setup();await user.click(screen.getByRole('button',{name:'Review deletion'}));await screen.findByText('A PDF is rendering.');await user.type(screen.getByLabelText('Type the exact organisation name to confirm'),preview.name);expect(screen.getByRole('button',{name:'Delete organisation'})).toBeDisabled();await user.click(screen.getByRole('button',{name:'Cancel'}));await waitFor(()=>expect(screen.queryByRole('dialog')).not.toBeInTheDocument());expect(fetch.mock.calls.some(([,i])=>i?.method==='DELETE')).toBe(false);
});

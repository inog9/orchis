import {QueryClient,QueryClientProvider} from '@tanstack/react-query';
import {render,screen} from '@testing-library/react';
import {MemoryRouter,Route,Routes} from 'react-router-dom';
import {afterEach,expect,it,vi} from 'vitest';
import {ReportDetailPage} from './ReportsPage';
afterEach(()=>vi.restoreAllMocks());
const method=(outcome:string,error='')=>({action:'nuclei.scan',version:1,target:'http://127.0.0.1',outcome,error});
function mount(assets:unknown[]){const report={id:'r',organization_id:'o',assessment_id:'a',status:'ready',version:1,created_at:'2026-10-31T00:00:00Z',snapshot:{organization:'LOCAL FIXTURE',risk:'No observed vulnerabilities',profile:'quick',by_severity:{},comparison:'Incomplete assets excluded.',remediation:'Review evidence',assets}};vi.spyOn(globalThis,'fetch').mockImplementation(async(input)=>new Response(JSON.stringify(String(input).includes('/auth/me')?{permissions:['report.read']}:report)));render(<QueryClientProvider client={new QueryClient({defaultOptions:{queries:{retry:false}}})}><MemoryRouter initialEntries={['/reports/r']}><Routes><Route path="/reports/:id" element={<ReportDetailPage/>}/></Routes></MemoryRouter></QueryClientProvider>);}
it('shows tested/skipped/incomplete counts and failure before opening methodology',async()=>{
 const asset=(id:string,status:string,methods:unknown[],skip_reason='')=>({id,name:id,status,methods,skip_reason,findings:[]});
 mount([asset('tested','finished',[method('success')]),asset('failed','finished',[method('error','Web endpoint unreachable <img src=x>')]),asset('unapproved','skipped',[],'Not authorized'),asset('no-address','skipped',[],'No address')]);
 expect(await screen.findByText('4 assets in scope · 1 fully tested · 2 skipped · 1 incomplete')).toBeInTheDocument();
 expect(screen.getByText('nuclei.scan: Web endpoint unreachable <img src=x>')).toBeVisible();expect(document.querySelector('img')).toBeNull();
 expect(screen.getByRole('heading',{name:'Vulnerabilities by severity'})).toBeInTheDocument();
 expect(screen.getAllByText('Methodology and execution')).toHaveLength(2);expect(document.querySelector('details[open]')).toBeNull();
});
it('does not count a finished asset without methods as fully tested',async()=>{
 mount([{id:'empty',name:'No methods',status:'finished',methods:[],skip_reason:'',findings:[]}]);
 expect(await screen.findByText('1 assets in scope · 0 fully tested · 0 skipped · 1 incomplete')).toBeInTheDocument();expect(screen.queryByText('Methodology and execution')).toBeNull();
});

it('renders a finding that has no evidence object without crashing',async()=>{
 mount([{id:'a',name:'Asset A',status:'skipped',methods:[],skip_reason:'No scan',findings:[{id:'f',type:'text',value:'v',title:'Finding without evidence',severity:'low'}]}]);
 expect(await screen.findByText('Finding without evidence')).toBeInTheDocument();
});

it('shows a MITRE technique on a finding as an estimate',async()=>{
 mount([{id:'a',name:'Asset A',status:'finished',methods:[],skip_reason:'',findings:[{id:'f',type:'vulnerability',value:'v',title:'Exposed panel',severity:'high',evidence:{},mitre:[{id:'T1190',name:'Exploit Public-Facing Application',basis:'estimate',reason:'Estimate only.'}]}]}]);
 expect(await screen.findByText(/MITRE ATT&CK \(estimate\): T1190 Exploit Public-Facing Application\. Estimate only\./)).toBeInTheDocument();
});

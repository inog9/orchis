import {QueryClient,QueryClientProvider} from '@tanstack/react-query';
import {render,screen} from '@testing-library/react';
import {MemoryRouter,Route,Routes} from 'react-router-dom';
import {afterEach,expect,it,vi} from 'vitest';
import {ReportDetailPage} from './ReportsPage';
afterEach(()=>vi.restoreAllMocks());
const method=(outcome:string,error='')=>({action:'nuclei.scan',version:1,target:'http://127.0.0.1',outcome,error});
function mount(assets:unknown[],snapExtra:object={}){const report={id:'r',organization_id:'o',assessment_id:'a',status:'ready',version:1,created_at:'2026-10-31T00:00:00Z',snapshot:{...snapExtra,organization:'LOCAL FIXTURE',risk:'No observed vulnerabilities',profile:'quick',by_severity:{},comparison:'Incomplete assets excluded.',remediation:'Review evidence',assets}};vi.spyOn(globalThis,'fetch').mockImplementation(async(input)=>new Response(JSON.stringify(String(input).includes('/auth/me')?{permissions:['report.read']}:report)));render(<QueryClientProvider client={new QueryClient({defaultOptions:{queries:{retry:false}}})}><MemoryRouter initialEntries={['/reports/r']}><Routes><Route path="/reports/:id" element={<ReportDetailPage/>}/></Routes></MemoryRouter></QueryClientProvider>);}
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

it('shows endpoint posture with the outside-vs-inside port comparison',async()=>{
 mount([{id:'a',name:'Asset A',status:'finished',methods:[],skip_reason:'',findings:[],endpoint:{collected_at:'2026-11-01T00:00:00Z',source:'osquery',attributes:{os_name:'Ubuntu',os_version:'26.04',kernel:'7.0'},findings:[{severity:'high',title:'ASLR is weak',query:'osquery.hardening'}],ports:{both:[22],scan_only:[80],host_only:[5432],note:'x'}}}],{endpoint_summary:{assets_total:2,assets_with_data:1,by_severity:{high:1}}});
 expect(await screen.findByText(/1 of 2 assets reported data from inside the machine/)).toBeInTheDocument();
 expect(screen.getByText('Ubuntu 26.04, kernel 7.0')).toBeInTheDocument();
 expect(screen.getByText('high: ASLR is weak')).toBeInTheDocument();
 expect(screen.getByText('Both: 22. Scan only: 80. Inside only: 5432.')).toBeInTheDocument();
 expect(screen.getByText(/not proof of a problem/)).toBeInTheDocument();
});
it('has no endpoint section when no asset reported endpoint data',async()=>{
 mount([{id:'a',name:'Asset A',status:'finished',methods:[],skip_reason:'',findings:[]}]);
 await screen.findByText('Vulnerabilities by severity');
 expect(screen.queryByText('Endpoint posture')).toBeNull();
});
it('caps a long list of endpoint findings in the report table',async()=>{
 const findings=Array.from({length:11},(_,i)=>({severity:'info',title:`Port ${i}/tcp listens`,query:'osquery.network'}));
 mount([{id:'a',name:'Asset A',status:'finished',methods:[],skip_reason:'',findings:[],endpoint:{collected_at:'2026-11-01T00:00:00Z',source:'osquery',attributes:{os_name:'Ubuntu'},findings}}],{endpoint_summary:{assets_total:1,assets_with_data:1,by_severity:{info:11}}});
 expect(await screen.findByText(/and 3 more/)).toBeInTheDocument();
 expect(screen.getAllByText(/^info: Port /)).toHaveLength(8);
});

it('offers only PDF and JSON exports and uses a PDF preview',async()=>{
 mount([]);await screen.findByRole('heading',{name:'LOCAL FIXTURE'});
 expect(screen.getByRole('link',{name:'Download PDF'})).toHaveAttribute('href','/api/v1/reports/r/export/pdf');
 expect(screen.getByRole('link',{name:'Download JSON'})).toHaveAttribute('href','/api/v1/reports/r/export/json');
 expect(screen.getByRole('link',{name:'Open PDF preview'})).toHaveAttribute('href','/api/v1/reports/r/export/pdf?preview=1');
 expect(screen.queryByRole('link',{name:/Download (HTML|CSV)/})).toBeNull();
});

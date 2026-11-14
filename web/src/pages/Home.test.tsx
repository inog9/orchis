import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, within, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, expect, it, vi } from 'vitest';
import { HomePage } from './HomePage';
const permissions=['organization.read','asset.read','finding.read','integration.read','assessment.read','simulation.read','report.read','assessment.run','simulation.run'];
const me={id:'u',display_name:'Ada',permissions};
const summary={organizations:[{id:'o1',name:'Acme'}],assets:{total:4,authorized:3,internet_facing:2},findings:{total:183,vulnerabilities:180,open_ports:3,vulnerable_assets:3,new_7d:100,by_severity:{critical:8,high:70,medium:75,low:15,info:12}},atomic:null,chains:{simulations:42,running:0,scenarios:0},latest_simulations:[],latest_runs:[],most_exposed:[],injectors:[{key:'hackbot',name:'Hackbot',status:'online'}],generated_at:'2026-11-10T02:00:00Z'};
const finding=(id:string,severity='critical')=>({id,asset_id:'a1',asset_name:'Public app',type:'vulnerability',value:'https://fixture.invalid',title:`Finding ${id}`,severity,details:{template_id:'fixture'},first_seen_at:'2026-11-01T00:00:00Z',last_seen_at:'2026-11-09T00:00:00Z',occurrence_count:1,triage_status:'open'});
const schedule=(id:string,at:string,paused=false)=>({id,name:`Schedule ${id}`,organization:'Acme',paused,next_run_at:at,config:{rule:{timezone:'Asia/Jakarta'}}});
const json=(v:unknown,status=200)=>new Response(JSON.stringify(v),{status});
function stub({home=summary,user=me,critical=[finding('critical')],high=[finding('high','high')],scans=[{id:'scan1',name:'Acme audit',status:'incomplete',created_at:'2026-11-09T00:00:00Z'}],schedules=[] as unknown[]}={}){
 return vi.spyOn(globalThis,'fetch').mockImplementation(async(input)=>{const url=new URL(String(input),'http://x');if(url.pathname.endsWith('/auth/me'))return json(user);if(url.pathname==='/api/v1/home')return json(home);if(url.pathname==='/api/v1/findings')return json({items:url.searchParams.get('severity')==='critical'?critical:high,next_cursor:null});if(url.pathname==='/api/v1/scans')return json({items:scans,next_cursor:null});if(url.pathname==='/api/v1/audit-schedules')return json({items:schedules,next_cursor:null});return json({items:[]});});
}
function mount(){render(<QueryClientProvider client={new QueryClient({defaultOptions:{queries:{retry:false}}})}><MemoryRouter><HomePage/></MemoryRouter></QueryClientProvider>);}
afterEach(()=>vi.restoreAllMocks());
it('shows each finding once, a single severity summary, real audits and no duplicate legacy panels',async()=>{
 const fetch=stub();mount();expect(await screen.findByRole('heading',{name:'Overview'})).toBeVisible();expect(await screen.findByRole('button',{name:/Finding critical/})).toBeVisible();expect(screen.getAllByText('Finding critical')).toHaveLength(1);expect(screen.queryByText('Finding high')).toBeNull();
 expect(screen.getByRole('heading',{name:'Vulnerability landscape'})).toBeVisible();expect(screen.getByRole('heading',{name:'Review first'})).toBeVisible();expect(await screen.findByRole('link',{name:/Acme audit/})).toHaveAttribute('href','/simulations/scan1');expect(screen.getByText('Incomplete coverage')).toBeVisible();
 expect(screen.queryByText('Live scan')).toBeNull();expect(screen.queryByText('EVENT STREAM')).toBeNull();expect(screen.queryByText('Most exposed assets')).toBeNull();expect(screen.queryByText('Engine status')).toBeNull();expect(screen.queryByText('Atomic tests')).toBeNull();expect(screen.queryByText('Total findings')).toBeNull();
 expect(fetch.mock.calls.some(([u])=>String(u).includes('/api/v1/simulations')||String(u).includes('/api/v1/assets'))).toBe(false);
});
it('requests only open critical findings of every type in a single call',async()=>{
 const fetch=stub({critical:[finding('old-critical')],high:[{...finding('new-high','high'),last_seen_at:'2026-11-10T02:00:00Z'}]});mount();await screen.findByRole('button',{name:/Finding old-critical/});expect(screen.queryByRole('button',{name:/Finding new-high/})).toBeNull();
 const calls=fetch.mock.calls.filter(([u])=>String(u).includes('/api/v1/findings'));expect(calls).toHaveLength(1);const url=new URL(String(calls[0]![0]),'http://x');expect(url.searchParams.get('triage')).toBe('open');expect(url.searchParams.get('severity')).toBe('critical');expect(url.searchParams.get('type')).toBeNull();
});
it('opens evidence in the shared finding drawer and restores the queue on close',async()=>{
 stub();mount();await userEvent.click(await screen.findByRole('button',{name:/Finding critical/}));const drawer=screen.getByRole('dialog');expect(within(drawer).getByRole('heading',{name:'Finding critical'})).toBeVisible();expect(drawer).toHaveTextContent('fixture');await userEvent.click(within(drawer).getByRole('button',{name:'Close details'}));expect(screen.queryByRole('dialog')).toBeNull();
});
it('finds the earliest active audit across schedule pages and ignores paused schedules',async()=>{
 const fetch=stub();const base=fetch.getMockImplementation()!;fetch.mockImplementation(async(input,init)=>{const u=new URL(String(input),'http://x');if(u.pathname==='/api/v1/audit-schedules')return json(u.searchParams.get('cursor')?{items:[schedule('soon','2026-11-12T00:00:00Z')],next_cursor:null}:{items:[schedule('later','2026-12-12T00:00:00Z'),schedule('paused','2026-11-11T00:00:00Z',true)],next_cursor:'older'});return base(input,init);});mount();expect(await screen.findByRole('link',{name:'Schedule soon'})).toHaveAttribute('href','/simulations/schedules/soon');expect(screen.queryByRole('link',{name:'Schedule later'})).toBeNull();expect(screen.queryByRole('link',{name:'Schedule paused'})).toBeNull();expect(screen.getByText('Asia/Jakarta')).toBeVisible();
});
it('empty results do not claim safety and authorisation is distinct from audit coverage',async()=>{
 stub({critical:[],high:[],scans:[]});mount();expect(await screen.findByText('No critical findings awaiting triage.')).toBeVisible();expect(screen.getByText(/Incomplete checks can leave gaps/)).toBeVisible();expect(await screen.findByRole('link',{name:'Set up a schedule'})).toHaveAttribute('href','/simulations/schedules?new=1');expect(screen.getByRole('progressbar',{name:'Testing authorisation'})).toHaveAttribute('aria-valuenow','3');expect(screen.getByText(/It does not confirm recent audit coverage/)).toBeVisible();
});
it('permission-limited accounts do not fetch scans, schedules or assets',async()=>{
 const fetch=stub({user:{...me,permissions:['finding.read']},home:{...summary,assets:null,findings:null} as unknown as typeof summary});mount();await screen.findByRole('heading',{name:'Overview'});await screen.findByRole('button',{name:/Finding critical/});expect(screen.queryByRole('link',{name:'Run an audit'})).toBeNull();expect(screen.queryByRole('heading',{name:'Recent audits'})).toBeNull();expect(fetch.mock.calls.some(([u])=>String(u).includes('/api/v1/scans')||String(u).includes('/audit-schedules')||String(u).includes('/api/v1/assets'))).toBe(false);
});
it('shows engine connection problems once without an engine status card',async()=>{
 stub({home:{...summary,injectors:[{key:'hackbot',name:'Hackbot',status:'offline'}]}});mount();await screen.findByRole('heading',{name:'Overview'});expect(screen.getAllByText('Checks unavailable: orchis-redteam-agent')).toHaveLength(1);expect(screen.getByRole('link',{name:'Review connections'})).toHaveAttribute('href','/settings/integrations');expect(screen.queryByText('Engine status')).toBeNull();
});
it('refresh updates both the workspace summary and the queue',async()=>{
 const fetch=stub();mount();await screen.findByRole('button',{name:/Finding critical/});const before=fetch.mock.calls.length;await userEvent.click(screen.getByRole('button',{name:'Refresh'}));await waitFor(()=>expect(fetch.mock.calls.length).toBeGreaterThan(before+2));
});
it('a queue failure stays visible rather than appearing as an empty queue',async()=>{
 const fetch=stub();const base=fetch.getMockImplementation()!;fetch.mockImplementation(async(input,init)=>String(input).includes('/api/v1/findings')?json({error_id:'err_queue'},500):base(input,init));mount();expect(await screen.findByText(/err_queue/)).toBeVisible();expect(screen.queryByText('No critical findings awaiting triage.')).toBeNull();
});
it('overview errors offer a retry',async()=>{
 const fetch=stub();const base=fetch.getMockImplementation()!;let failed=true;fetch.mockImplementation(async(input,init)=>String(input).includes('/api/v1/home')&&failed?json({error_id:'err_home'},500):base(input,init));mount();expect(await screen.findByRole('heading',{name:'Overview could not be loaded'})).toBeVisible();failed=false;await userEvent.click(screen.getByRole('button',{name:'Try again'}));expect(await screen.findByRole('heading',{name:'Overview'})).toBeVisible();
});
it('new workspaces avoid rows of zero statistics',async()=>{
 stub({critical:[],high:[],scans:[],home:{...summary,organizations:[],assets:{total:0,authorized:0,internet_facing:0},findings:{...summary.findings,total:0,vulnerabilities:0,by_severity:{critical:0,high:0,medium:0,low:0,info:0}}}});mount();
 expect(await screen.findByText('No vulnerabilities recorded yet. Run an audit to establish coverage.')).toBeVisible();expect(screen.queryByRole('progressbar')).toBeNull();expect(screen.queryByRole('link',{name:'Run an audit'})).toBeNull();expect(screen.getByRole('link',{name:'Add your first asset'})).toHaveAttribute('href','/assets?new=1');
});
it('a finding returned by both requests is shown once',async()=>{
 const f=finding('same');stub({critical:[f],high:[f]});mount();expect(await screen.findByRole('button',{name:/Finding same/})).toBeVisible();expect(screen.getAllByText('Finding same')).toHaveLength(1);
});

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { entriesFor } from './CommandPalette';
import { NavDock } from './Shell';
import { advancedEnabled, navigationFor } from './navigation';
import { guideFor, PageAccess } from './PageGuide';
import { setupSteps } from './OrganisationSetup';
const me={id:'u',email:'u@example.test',display_name:'Admin',role:'admin',permissions:['settings.manage','asset.read','organization.read','atomic.read','finding.read','report.read','scenario.read','simulation.read']};
function mount(children:React.ReactNode,path='/') {vi.spyOn(globalThis,'fetch').mockResolvedValue(new Response(JSON.stringify(me)));const qc=new QueryClient({defaultOptions:{queries:{retry:false}}});return render(<QueryClientProvider client={qc}><MemoryRouter initialEntries={[path]}>{children}</MemoryRouter></QueryClientProvider>);}
afterEach(()=>vi.restoreAllMocks());
describe('Navigasi tugas',()=>{
 it('menyatukan pintu masuk tanpa menghilangkan kemampuan',()=>{const nav=navigationFor(me);expect(nav.map(n=>n.id)).not.toContain('atomic');expect(nav.map(n=>n.id)).not.toContain('chains');expect(nav.map(n=>n.id)).not.toContain('mitre');expect(nav.map(n=>n.id)).toEqual(['overview','organizations','assets','simulations','findings','reports','settings']);for(const n of nav)expect(n.hint.length).toBeGreaterThan(20);expect(navigationFor({...me,permissions:[]}).map(n=>n.id)).toEqual(['overview']);});
 it('Viewer navigation contains only findings and reports',()=>{expect(navigationFor({...me,role:'viewer',permissions:['finding.read','report.read']}).map(n=>n.id)).toEqual(['overview','findings','reports']);});
 it('fitur advanced tertutup tanpa flag server meskipun permission tersedia',()=>{expect(advancedEnabled(me)).toBe(false);for(const id of ['chains','atomic','mitre'])expect(entriesFor(me).map(e=>e.id)).not.toContain(id);expect(entriesFor(me).map(e=>e.id)).toContain('overview');const enabled={...me,features:{advanced:true}};for(const id of ['chains','atomic','mitre'])expect(entriesFor(enabled).map(e=>e.id)).toContain(id);expect(entriesFor(enabled).some(e=>e.to.startsWith('/scenarios'))).toBe(false);const keys=entriesFor(enabled).flatMap(e=>e.keys?[e.keys]:[]);expect(new Set(keys).size).toBe(keys.length);});
 it('tooltip tampil saat hover dan fokus, dapat ditutup Escape',async()=>{mount(<NavDock/>);const user=userEvent.setup();const link=await screen.findByRole('link',{name:'Organisations'});await user.hover(link);expect(screen.getByRole('tooltip')).toHaveTextContent('Bring company assets together');await user.unhover(link);expect(screen.queryByRole('tooltip')).not.toBeInTheDocument();link.focus();expect(await screen.findByRole('tooltip')).toHaveTextContent('Organisations');expect(link).toHaveAttribute('aria-describedby','nav-tip-organizations');await user.keyboard('{Escape}');expect(screen.queryByRole('tooltip')).not.toBeInTheDocument();});
 it('menandai Organisations saat meninjau pekerjaan AI organisasi',async()=>{mount(<NavDock/>,'/ai/jobs/example');expect(await screen.findByRole('link',{name:'Organisations'})).toHaveAttribute('aria-current','page');});
 it('route lama dan detail memperoleh konteks yang tepat',()=>{expect(guideFor('/assets/agents')?.perm).toBe('agent.read');expect(guideFor('/assets/abc')?.purpose).toMatch(/approved address/);expect(guideFor('/chains/abc')?.purpose).toMatch(/mapping/);expect(guideFor('/settings/users')?.perm).toBe('user.manage');});
 it('menjelaskan tautan terlarang sebelum merender halaman yang meminta API',async()=>{mount(<PageAccess><p>Privileged content</p></PageAccess>,'/settings/users');expect(await screen.findByRole('heading',{name:'Access needed'})).toBeInTheDocument();expect(screen.queryByText('Privileged content')).not.toBeInTheDocument();expect(screen.getByRole('link',{name:'Go home'})).toBeInTheDocument();});
});
describe('Checklist berdasarkan hasil nyata',()=>{
 const base={organization:true,assets:2,authorized:2,ready:2,assessments:0,reports:0};
 it('menunggu peluncuran dan laporan, bukan jumlah definisi test',()=>{expect(setupSteps(base)).toEqual([true,true,true,false,false]);expect(setupSteps({...base,assessments:1,reports:1})).toEqual([true,true,true,true,true]);});
 it('alamat ditolak atau belum ada tidak menyelesaikan langkah izin',()=>{expect(setupSteps({...base,ready:0})[2]).toBe(false);expect(setupSteps({...base,authorized:0})[2]).toBe(false);expect(setupSteps({...base,assets:0})[1]).toBe(false);expect(setupSteps({...base,organization:false})[0]).toBe(false);});
});

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { App } from './App';
const state=vi.hoisted(()=>({advanced:false}));
vi.mock('./api/hooks',()=>({useMe:()=>({isPending:false,isError:false,data:{permissions:['organization.read','scenario.read','atomic.read','ai.read'],features:{advanced:state.advanced}}})}));
vi.mock('./components/Shell',async()=>{const {Outlet}=await import('react-router-dom');return {Shell:()=> <Outlet/>}});
vi.mock('./pages/OrganizationsPage',()=>({OrganizationsPage:()=> <h1>Organisation hub</h1>}));
vi.mock('./pages/HomePage',()=>({NotFoundPage:()=> <h1>Page missing</h1>,HomePage:()=> <h1>Advanced overview</h1>}));
vi.mock('./pages/ChainWorkspacePage',()=>({ChainWorkspacePage:()=> <h1>Advanced chain editor</h1>}));
function mount(path:string){render(<QueryClientProvider client={new QueryClient()}><MemoryRouter initialEntries={[path]}><App/></MemoryRouter></QueryClientProvider>)}
afterEach(()=>{state.advanced=false});
describe('Runtime advanced routes',()=>{
 it.each(['/chains','/chains/example','/atomic','/mitre','/organizations/example/ai','/ai/jobs/example','/overview'])('hides %s with the default server flag',async path=>{mount(path);expect(await screen.findByRole('heading',{name:'Organisation hub'})).toBeVisible()});
 it('opens a retained advanced editor after an explicit server opt in',async()=>{state.advanced=true;mount('/chains/example');expect(await screen.findByRole('heading',{name:'Advanced chain editor'})).toBeVisible()});
 it.each([false,true])('never restores the removed scenario route (advanced=%s)',async advanced=>{state.advanced=advanced;mount('/scenarios');expect(await screen.findByRole('heading',{name:'Page missing'})).toBeVisible()});
});

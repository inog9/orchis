import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { MemoryRouter, Routes, Route, useLocation } from 'react-router-dom';
import { OrganizationDetailPage } from '../pages/OrganizationsPage';
import { AssetsPage } from '../pages/AssetsPage';
import { FindingsPage } from '../pages/FindingsPage';

const A='11111111-1111-4111-8111-111111111111',B='22222222-2222-4222-8222-222222222222';
const response=(data:unknown,status=200)=>new Response(JSON.stringify(data),{status,headers:{'Content-Type':'application/json'}});
function Location(){return <output aria-label="Current URL">{useLocation().search}</output>}
function mount(page:React.ReactNode,path:string){return render(<QueryClientProvider client={new QueryClient({defaultOptions:{queries:{retry:false}}})}><MemoryRouter initialEntries={[path]}>{page}<Location/></MemoryRouter></QueryClientProvider>)}
function stub(){return vi.spyOn(globalThis,'fetch').mockImplementation(async(input)=>{const url=String(input);if(url.includes('/auth/me'))return response({permissions:['asset.read','asset.write','organization.read','organization.write','finding.read']});if(url.includes('/organizations?'))return response({items:[{id:A,name:'Acme',slug:'acme'},{id:B,name:'Beta',slug:'beta'}],next_cursor:null});return response({items:[],next_cursor:null});})}
afterEach(()=>vi.restoreAllMocks());

describe('Organisation scope in shared asset/finding views',()=>{
 it('restores the asset organisation from a shared URL and sends it to the server',async()=>{const fetch=stub();mount(<AssetsPage/>,`/assets?org=${A}`);await screen.findByRole('option',{name:'Acme · acme'});expect(screen.getByRole('combobox',{name:'Organisation'})).toHaveValue(A);expect(fetch.mock.calls.some(([url])=>String(url).includes(`/assets?limit=25&org=${A}`))).toBe(true);await userEvent.selectOptions(screen.getByRole('combobox',{name:'Organisation'}),B);await waitFor(()=>expect(screen.getByLabelText('Current URL')).toHaveTextContent(`org=${B}`));});
 it('clears the asset filter when changing organisation to avoid conflicting scopes',async()=>{const fetch=stub();mount(<FindingsPage/>,`/findings?org=${A}&asset_id=asset-one`);await screen.findByRole('option',{name:'Acme · acme'});await userEvent.selectOptions(screen.getByRole('combobox',{name:'Organisation'}),B);await waitFor(()=>expect(fetch.mock.calls.some(([url])=>String(url).includes(`/findings?limit=25&org=${B}`))).toBe(true));expect(screen.getByLabelText('Current URL')).not.toHaveTextContent('asset_id=asset-one');});
 it('locks the organisation tab despite a conflicting URL parameter',async()=>{const fetch=stub();mount(<FindingsPage organizationID={A} organizationName="Acme"/>,`/organizations/${A}?org=${B}`);expect(await screen.findByRole('combobox',{name:'Organisation'})).toBeDisabled();await waitFor(()=>expect(fetch.mock.calls.some(([url])=>String(url).includes(`/findings?limit=25&org=${A}`))).toBe(true));expect(fetch.mock.calls.some(([url])=>String(url).includes(`/findings?limit=25&org=${B}`))).toBe(false);});
 it('does not clear the selected scope when organisation options fail',async()=>{stub().mockImplementation(async(input)=>String(input).includes('/auth/me')?response({permissions:['asset.read','organization.read']}):String(input).includes('/organizations?')?response({title:'Unavailable'},503):response({items:[],next_cursor:null}));mount(<AssetsPage/>,`/assets?org=${A}`);await screen.findByRole('alert');expect(screen.getByRole('combobox',{name:'Organisation'})).toHaveValue(A);expect(screen.getByLabelText('Current URL')).toHaveTextContent(`org=${A}`);});
 it('uses the shared locked views inside the organisation tabs and restores the chosen tab',async()=>{
  const fetch=stub().mockImplementation(async(input)=>{const url=String(input);if(url.includes('/auth/me'))return response({permissions:['organization.read','asset.read','asset.write','organization.write','finding.read']});if(url.endsWith('/scan-control'))return response({paused:false,global_paused:false,window:{},version:0,allowed_now:true});if(url.endsWith(`/organizations/${A}`))return response({id:A,name:'Acme',slug:'acme',version:1});return response({items:[],next_cursor:null});});
  mount(<Routes><Route path="/organizations/:id" element={<OrganizationDetailPage/>}/></Routes>,`/organizations/${A}?tab=assets&org=${B}`);
  await screen.findByRole('heading',{name:'Acme'});
  expect(screen.getByRole('combobox',{name:'Organisation'})).toBeDisabled();
  expect(screen.getByRole('button',{name:'Add asset'})).toBeEnabled();
  await waitFor(()=>expect(fetch.mock.calls.some(([url])=>String(url).includes(`/assets?limit=25&org=${A}`))).toBe(true));
  await userEvent.click(screen.getByRole('button',{name:'Findings'}));
  await waitFor(()=>expect(fetch.mock.calls.some(([url])=>String(url).includes(`/findings?limit=25&org=${A}`))).toBe(true));
  expect(screen.getByLabelText('Current URL')).toHaveTextContent('tab=findings');
  expect(screen.getByRole('combobox',{name:'Organisation'})).toBeDisabled();
 });

});

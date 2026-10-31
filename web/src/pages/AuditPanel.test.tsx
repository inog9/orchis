import {QueryClient,QueryClientProvider} from '@tanstack/react-query';
import {render,screen,waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {afterEach,expect,it,vi} from 'vitest';
import {AuditPanel} from './AuditPanel';
const event={id:'event-one',at:'2026-10-31T10:00:00Z',actor_user_id:'user-one',actor:'<img src=x onerror=alert(1)>',action:'auth.login_failed',resource_type:'session',resource_id:null,ip:'192.0.2.5',request_id:'req_audit',reason:'invalid_credentials'};
function mount(permissions=['audit.read'],handler:(url:string)=>Response=()=>new Response(JSON.stringify({items:[event],next_cursor:null}))){
 const fetch=vi.spyOn(globalThis,'fetch').mockImplementation(async input=>String(input).includes('/auth/me')?new Response(JSON.stringify({id:'u',permissions})):handler(String(input)));
 render(<QueryClientProvider client={new QueryClient({defaultOptions:{queries:{retry:false}}})}><AuditPanel/></QueryClientProvider>);return fetch;
}
afterEach(()=>vi.restoreAllMocks());
it('escapes audit actor content and exposes safe metadata',async()=>{mount();expect(await screen.findByText(event.actor)).toBeInTheDocument();expect(document.querySelector('img')).toBeNull();expect(screen.getByText('req_audit')).toBeInTheDocument();expect(screen.getByText('invalid credentials')).toBeInTheDocument();});
it('does not request audit data without permission',async()=>{const fetch=mount([]);expect(await screen.findByText('Audit access required')).toBeInTheDocument();expect(fetch.mock.calls.some(call=>String(call[0]).startsWith('/api/v1/audit'))).toBe(false);});
it('paginates and resets the cursor when event filter changes',async()=>{
 const fetch=mount(['audit.read'],url=>new Response(JSON.stringify({items:[{...event,id:url.includes('cursor=page-two')?'event-two':'event-one',actor:url.includes('cursor=page-two')?'Second actor':'First actor'}],next_cursor:url.includes('cursor=page-two')?null:'page-two'})));
 await screen.findByText('First actor');await userEvent.click(screen.getByRole('button',{name:'Next'}));expect(await screen.findByText('Second actor')).toBeInTheDocument();expect(screen.getByText('Page 2')).toBeInTheDocument();await userEvent.selectOptions(screen.getByLabelText('Event type'),'auth.login');await waitFor(()=>expect(fetch.mock.calls.some(call=>String(call[0]).includes('action=auth.login&cursor='))).toBe(true));expect(screen.getByText('Page 1')).toBeInTheDocument();
});
it('shows an explicit empty state',async()=>{mount(['audit.read'],()=>new Response(JSON.stringify({items:[],next_cursor:null})));expect(await screen.findByText('No events match')).toBeInTheDocument();expect(screen.getByRole('button',{name:'Next'})).toBeDisabled();});

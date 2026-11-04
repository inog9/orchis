import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import {act, render, screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {QueryClient, QueryClientProvider} from '@tanstack/react-query';
import {LanguagePreference, LanguageSession} from './LanguagePreference';
import {language, setLanguage} from './runtime';
import type {User} from '../api/types';
beforeEach(()=>{act(()=>setLanguage('en'));vi.spyOn(navigator,'languages','get').mockReturnValue(['en-US']);});
afterEach(()=>{act(()=>setLanguage('en'));vi.restoreAllMocks();});
const user=(extra:Partial<User>={}):User=>({id:'user-a',email:'a@fixture.test',display_name:'Fixture',role:'viewer',permissions:['report.read','finding.read'],ui_language:'en',...extra});
function mount(initial:User|null){const client=new QueryClient({defaultOptions:{queries:{retry:false},mutations:{retry:false}}});client.setQueryData(['me'],initial);render(<QueryClientProvider client={client}><LanguageSession/><LanguagePreference/><input aria-label="unsaved fixture" defaultValue="Tetap ada"/></QueryClientProvider>);return client;}
describe('Account language preference',()=>{
 it('restores the saved language and changes it through the authenticated CSRF API',async()=>{
  const fetch=vi.spyOn(globalThis,'fetch').mockImplementation(async(_input,init)=>{const body=JSON.parse(String(init?.body));return new Response(JSON.stringify(user({ui_language:body.ui_language})),{status:200})});
  mount(user({ui_language:'id'}));await waitFor(()=>expect(language()).toBe('id'));
  const input=screen.getByRole('textbox');await userEvent.selectOptions(screen.getByRole('combobox',{name:'Bahasa'}),'en');
  await waitFor(()=>expect(language()).toBe('en'));expect(screen.getByRole('combobox',{name:'Language'})).toHaveValue('en');expect(screen.getByRole('textbox')).toBe(input);expect(input).toHaveValue('Tetap ada');
  const [path,init]=fetch.mock.calls[0]!;expect(path).toBe('/api/v1/auth/preferences');expect(init?.method).toBe('PATCH');expect(init?.headers).toMatchObject({'X-Requested-With':'orchis-web'});
 });
 it('keeps the previous choice on failure and ignores server diagnostic text',async()=>{
  vi.spyOn(globalThis,'fetch').mockResolvedValue(new Response(JSON.stringify({code:'request_failed',detail:'SECRET internal failure'}),{status:500}));mount(user());
  await userEvent.selectOptions(screen.getByRole('combobox',{name:'Language'}),'id');expect(await screen.findByRole('alert')).toHaveTextContent('Could not save your language preference');expect(screen.getByRole('alert')).not.toHaveTextContent('SECRET');expect(language()).toBe('en');
 });
 it('resets the language between accounts and on logout',async()=>{
  const client=mount(user({ui_language:'id'}));await waitFor(()=>expect(language()).toBe('id'));
  act(()=>client.setQueryData(['me'],user({id:'user-b',ui_language:''})));await waitFor(()=>expect(language()).toBe('en'));
  act(()=>client.setQueryData(['me'],user({id:'user-b',ui_language:'id'})));await waitFor(()=>expect(language()).toBe('id'));
  act(()=>client.setQueryData(['me'],null));await waitFor(()=>expect(language()).toBe('en'));
 });
});

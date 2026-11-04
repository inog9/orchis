import { afterEach, describe, expect, it } from 'vitest';
import { render, screen, act } from '@testing-library/react';
import { en } from './en';
import { id } from './id';
import { browserLanguage, language, locale, setLanguage, t, tr, fv, relativeTime, useLocale } from './runtime';
import { installCommand } from '../lib/agentInstall';

function keyFor(english:string):keyof typeof en {
 const key=(Object.keys(en) as (keyof typeof en)[]).find(key=>en[key]===english);
 if(!key)throw Error('Missing test copy '+english);return key;
}
afterEach(()=>act(()=>setLanguage('en')));
describe('Complete EN/ID catalogues',()=>{
 it('has identical, nonempty keys and exactly matching interpolation slots',()=>{
  expect(Object.keys(id).sort()).toEqual(Object.keys(en).sort());
  for(const key of Object.keys(en) as (keyof typeof en)[]){
   expect(id[key].trim(),key).not.toBe('');
   expect(id[key].match(/\{\d+\}/g)?.sort()??[],key).toEqual(en[key].match(/\{\d+\}/g)?.sort()??[]);
  }
 });
 it('has no orphan catalogue entries',()=>{
  const sources=import.meta.glob('../**/*.{ts,tsx}',{query:'?raw',import:'default',eager:true});
  const source=Object.entries(sources).filter(([path])=>!path.includes('.test.')&&!path.includes('/test/')&&!path.endsWith('/en.ts')&&!path.endsWith('/id.ts')).map(([,text])=>String(text)).join('\n');
  for(const key of Object.keys(en))expect(source.includes(`'${key}'`)||source.includes(`"${key}"`),key).toBe(true);
 });
});
it('negotiates browser language and falls back to English',()=>{
 expect(browserLanguage(['id-ID','en-US'])).toBe('id');expect(browserLanguage(['en-GB','id'])).toBe('en');expect(browserLanguage(['fr-FR'])).toBe('en');expect(browserLanguage([])).toBe('en');
});
it('updates subscribed components without replacing their user content',()=>{
 const key=keyFor('Assets');
 function View(){useLocale();return <><h1>{t(key)}</h1><input aria-label="fixture input" defaultValue="Nama pengguna tetap"/></>}
 setLanguage('en');render(<View/>);const input=screen.getByRole('textbox');
 expect(screen.getByRole('heading').textContent).toBe('Assets');
 act(()=>setLanguage('id'));expect(screen.getByRole('heading').textContent).toBe('Aset');expect(screen.getByRole('textbox')).toBe(input);expect(input).toHaveValue('Nama pengguna tetap');expect(document.documentElement.lang).toBe('id');
});
it('keeps rich message values escaped and preserves element identity',()=>{
 const key=keyFor('Update requested: {0}');setLanguage('id');
 const {container}=render(<div>{tr(key,['<img src=x onerror=alert(1)>'])}</div>);
 expect(container.querySelector('img')).toBeNull();expect(container.textContent).toContain('<img src=x onerror=alert(1)>');expect(container.textContent).toContain('Pembaruan diminta');
});
it('formats numeric and relative time values with the selected locale',()=>{
 setLanguage('en');expect(fv(12345)).toBe('12,345');expect(relativeTime(-1,'day')).toBe('yesterday');
 setLanguage('id');expect(locale()).toBe('id-ID');expect(language()).toBe('id');expect(fv(12345)).toBe('12.345');expect(relativeTime(-1,'day')).toBe('kemarin');expect(new Date('2026-11-03T00:00:00Z').toLocaleDateString(locale(),{month:'long',timeZone:'UTC'})).toBe('November');
});

it('leaves executable commands and authorization headers unchanged across languages',()=>{
 const token={id:'fixture',token:'oen_fixture',os:'windows',arch:'x86_64',mode:'service',expires_at:'',version:'1.0.0',sha256:'a'.repeat(64),file_name:'fixture'} as const;
 setLanguage('en');const enCommand=installCommand('https://fixture.invalid',token,'instance-a');
 setLanguage('id');expect(installCommand('https://fixture.invalid',token,'instance-a')).toBe(enCommand);expect(enCommand).toContain('Bearer oen_fixture');expect(enCommand).toContain('New-Item');expect(enCommand).toContain('Invoke-WebRequest');
});

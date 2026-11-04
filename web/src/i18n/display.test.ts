import { afterEach, expect, it } from 'vitest';
import { act } from '@testing-library/react';
import { capitalLabel, displayLabel } from './display';
import { setLanguage } from './runtime';

afterEach(()=>act(()=>setLanguage('en')));
const severities=['critical','high','medium','low','info','unknown'];
it('keeps English labels byte-identical to the previous rendering',()=>{
 for(const v of severities){expect(displayLabel(v)).toBe(v);expect(capitalLabel(v)).toBe(v[0]!.toUpperCase()+v.slice(1));}
 expect(displayLabel('Ports')).toBe('Ports');
 expect(displayLabel('CVE-2024-0001 on 10.0.0.5')).toBe('CVE-2024-0001 on 10.0.0.5');
});
it('translates controlled labels to Indonesian and leaves free text untouched',()=>{
 act(()=>setLanguage('id'));
 expect(displayLabel('critical')).toBe('kritis');
 expect(capitalLabel('high')).toBe('Tinggi');
 expect(displayLabel('Ports')).toBe('Port');
 expect(displayLabel('CVE-2024-0001 on 10.0.0.5')).toBe('CVE-2024-0001 on 10.0.0.5');
});

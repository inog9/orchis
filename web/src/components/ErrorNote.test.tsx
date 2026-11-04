import {afterEach,describe,expect,it} from 'vitest';
import {ApiError} from '../api/client';
import {describeError} from './ErrorNote';
import {setLanguage} from '../i18n/runtime';
afterEach(()=>setLanguage('en'));
describe('API messages use stable codes',()=>{
 it('rejects unknown server field messages and detail',()=>{
  const e=new ApiError(422,'untrusted title','secret detail','err_1','req',[{name:'scenario',message:'plain-box has no agent.'}]);
  expect(describeError(e).message).toBe('Check this field.');expect(e.fields[0]?.message).not.toContain('plain-box');
  expect(describeError(new ApiError(409,'Conflict','server secret')).message).not.toContain('server secret');
 });
 it('translates known codes and never exposes a 5xx detail',()=>{
  const e=new ApiError(409,'ignored','ignored',undefined,undefined,[],undefined,'already_running');
  expect(describeError(e).message).toBe('Already running.');setLanguage('id');expect(describeError(e).message).toBe('Sudah berjalan.');
  expect(describeError(new ApiError(500,'x','stack trace here','err_9')).message).not.toContain('stack');
 });
});

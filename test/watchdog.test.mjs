import {test} from 'node:test';
import assert from 'node:assert/strict';
import {StallMonitor, pollStalls} from '../src/watchdog.mjs';

const msg = content => ({type:'assistant',time:{created:1},content});
test('interrupts a silent child, never its waiting parent or another job', () => {
  const m = new StallMonitor({timeoutMs:100});
  const sessions=[{id:'root'},{id:'child',parentID:'root'},{id:'unrelated'}];
  const msgs=new Map([['root',msg([{type:'tool',name:'subagent',state:{status:'running'}}])],['child',msg([{type:'reasoning',text:'partial'}])],['unrelated',msg([])]]);
  assert.deepEqual(m.evaluate(sessions,msgs,'root',0),[]);
  assert.deepEqual(m.evaluate(sessions,msgs,'root',101),['child']);
});
test('activity resets timer and an active tool is not killed', () => {
  const m=new StallMonitor({timeoutMs:100});const sessions=[{id:'root'}];
  let msgs=new Map([['root',msg([{type:'reasoning',text:'first'}])]]);
  assert.deepEqual(m.evaluate(sessions,msgs,'root',0),[]);
  msgs=new Map([['root',msg([{type:'reasoning',text:'more'}])]]);
  assert.deepEqual(m.evaluate(sessions,msgs,'root',99),[]);
  assert.deepEqual(m.evaluate(sessions,msgs,'root',150),[]);
  msgs=new Map([['root',msg([{type:'tool',name:'shell',state:{status:'running'}}])]]);
  m.evaluate(sessions,msgs,'root',200);
  assert.deepEqual(m.evaluate(sessions,msgs,'root',1000),[]);
});
test('poll uses session tree and interrupts only stalled child', async () => {
  const m=new StallMonitor({timeoutMs:0});let requests=[];
  const fetcher=async (url,opts) => {
    requests.push([url,opts.method || 'GET']);
    const data=url.includes('/api/session?') ? [{id:'root'},{id:'child',parentID:'root'}] : [msg([])];
    return {ok:true,json:async()=>({data})};
  };
  assert.deepEqual(await pollStalls({address:'http://localhost',password:'test',rootId:'root',monitor:m,fetcher}),['child']);
  assert.ok(requests.some(([url,method])=>url.endsWith('/child/interrupt') && method==='POST'));
  assert.ok(!requests.some(([url,method])=>url.endsWith('/root/interrupt') && method==='POST'));
});

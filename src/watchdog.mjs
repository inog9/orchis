import {createHash} from 'node:crypto';

// Track only sessions descended from this job. A parent waiting for an active
// child must not be interrupted merely because the parent's own output is quiet.
export class StallMonitor {
  constructor({timeoutMs = 10 * 60 * 1000} = {}) { this.timeoutMs = timeoutMs; this.observed = new Map(); }
  evaluate(sessions, messages, rootId, now = Date.now()) {
    const relevant = new Set([rootId]);
    let changed = true;
    while (changed) {
      changed = false;
      for (const s of sessions) if (relevant.has(s.parentID) && !relevant.has(s.id)) { relevant.add(s.id); changed = true; }
    }
    const active = new Map();
    for (const s of sessions) {
      if (!relevant.has(s.id)) continue;
      const m = messages.get(s.id);
      if (m?.type !== 'assistant' || !m.time || m.time.completed) { this.observed.delete(s.id); continue; }
      const fingerprint = createHash('sha256').update(JSON.stringify(m)).digest('hex');
      const previous = this.observed.get(s.id);
      if (!previous || previous.fingerprint !== fingerprint) this.observed.set(s.id, {fingerprint, since:now});
      active.set(s.id, m);
    }
    for (const id of [...this.observed.keys()]) if (!relevant.has(id)) this.observed.delete(id);
    const stalled = [];
    for (const [id, m] of active) {
      // Do not treat an executing tool or a parent waiting for a child as a silent model stream.
      if ([...active.keys()].some(child => sessions.some(s => s.id === child && s.parentID === id))) continue;
      if ((m.content || []).some(p => p.type === 'tool' && (p.state?.status === 'running' || p.state === 'running'))) continue;
      if (now - this.observed.get(id).since >= this.timeoutMs) { stalled.push(id); this.observed.delete(id); }
    }
    return stalled;
  }
}

export async function pollStalls({address, password, rootId, monitor, fetcher = fetch}) {
  if (!rootId) return [];
  const headers = {Authorization:'Basic '+Buffer.from('opencode:'+password).toString('base64')};
  const get = async path => {
    const r = await fetcher(address+path, {headers, signal:AbortSignal.timeout(5000)});
    if (!r.ok) throw Error('OpenCode monitoring unavailable');
    return r.json();
  };
  const sessions = [];
  let cursor = '';
  for (let page=0; page<20; page++) {
    const response = await get('/api/session?limit=100'+(cursor ? '&cursor='+encodeURIComponent(cursor) : ''));
    if (!Array.isArray(response.data)) throw Error('invalid session list');
    sessions.push(...response.data);
    cursor = response.cursor?.next || '';
    if (!cursor) break;
  }
  if (!sessions.some(s => s.id === rootId)) throw Error('OpenCode session not found');
  const relevant = new Set([rootId]);
  for (let i=0; i<sessions.length; i++) for (const s of sessions) if (relevant.has(s.parentID)) relevant.add(s.id);
  const messages = new Map();
  for (const s of sessions.filter(s => relevant.has(s.id))) {
    const data = await get(`/api/session/${encodeURIComponent(s.id)}/message?limit=1&order=desc`);
    messages.set(s.id, data.data?.[0]);
  }
  const stalled = monitor.evaluate(sessions, messages, rootId);
  for (const id of stalled) {
    const r = await fetcher(address+`/api/session/${encodeURIComponent(id)}/interrupt`, {method:'POST',headers:{...headers,'Content-Type':'application/json'},body:'{}',signal:AbortSignal.timeout(5000)});
    if (!r.ok) throw Error('OpenCode session interrupt failed');
  }
  return stalled;
}

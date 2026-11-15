#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import net from 'node:net';
import {randomBytes} from 'node:crypto';
import {spawn, spawnSync} from 'node:child_process';
import {copyScannerInputs} from './scanners.mjs';
import {prepareResult} from './result.mjs';
import {OPENCODE_TIMEOUT_MS} from './deadline.mjs';
import {StallMonitor, pollStalls} from './watchdog.mjs';
import {restrictSkills} from './skills.mjs';

process.umask(0o077);
const args = process.argv.slice(2);
const arg = name => { const i = args.indexOf(name); return i < 0 ? '' : args[i + 1]; };
const root = path.resolve(arg('--dir') || '.');
const session = path.basename(root);
const emit = (event, extra = {}) => console.log(JSON.stringify({schema: 1, event, sessionId: session, ...extra}));
let stack, child, server, interrupted = false;
const stopChild = () => { if (child && child.exitCode === null) { const pid = child.pid; try { process.kill(-pid, 'SIGTERM'); } catch {} const watchdog = setTimeout(() => { try { process.kill(-pid, 'SIGKILL'); } catch {} }, 5000); watchdog.unref(); } };
const stop = () => { interrupted = true; stopChild(); if (server && server.exitCode === null) server.kill('SIGTERM'); if (stack && stack.exitCode === null) stack.kill('SIGTERM'); };
process.on('SIGTERM', stop); process.on('SIGINT', stop);
const wait = p => new Promise(resolve => { p.once('error', () => resolve(null)); p.once('close', code => resolve(code)); });
const delay = ms => new Promise(resolve => setTimeout(resolve, ms));
const listening = port => new Promise(resolve => { const s = net.connect(port, '127.0.0.1'); s.setTimeout(300); s.once('connect', () => { s.destroy(); resolve(true); }); s.once('error', () => resolve(false)); s.once('timeout', () => { s.destroy(); resolve(false); }); });
try {
  if (!/^[a-zA-Z0-9-]{1,80}$/.test(session)) throw Error('invalid session');
  const prompt = fs.readFileSync(0, 'utf8');
  const target = prompt.match(/^Run pentest this website (\S+), use skill pentest\.$/)?.[1];
  const url = new URL(target);
  if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password) throw Error('invalid target');
  emit('starting');
  copyScannerInputs(arg('--inputs-dir'), root);
  if (arg('--language')) {
    fs.mkdirSync(path.join(root, 'inputs'), {recursive:true});
    fs.writeFileSync(path.join(root, 'inputs/report-options.json'), JSON.stringify({language:arg('--language')}));
  }
  const init = spawnSync('pentest-harnes', ['init', '--dir', root, '--agent', 'agent'], {stdio: 'ignore'});
  if (init.status !== 0) throw Error('skill installation failed');
  const git = spawnSync('git', ['init', '-q', root], {stdio:'ignore'});
  if (git.status !== 0) throw Error('cannot establish OpenCode workspace');
  // The harness's Pi-specific agent instructions are not part of the OpenCode prompt.
  fs.rmSync(path.join(root, 'AGENTS.md'), {force:true});
  fs.copyFileSync('/opt/orchis-opencode/skills/pentest-report/SKILL.md', path.join(root, '.agent/skills/pentest-report/SKILL.md'));
  restrictSkills(path.join(root, '.agent/skills'));
  fs.mkdirSync(path.join(root, '.opencode'), {recursive: true});
  fs.appendFileSync(path.join(root, '.agent/skills/pentest/SKILL.md'), '\nRun at most one subagent at a time; wait for it to finish before starting another.\n');
  fs.cpSync(path.join(root, '.agent/skills'), path.join(root, '.opencode/skills'), {recursive:true});
  fs.writeFileSync(path.join(root, 'state.json'), JSON.stringify({mode:'running', scope:{origins:[url.origin], methods:['GET','HEAD','OPTIONS','POST','PUT','PATCH','DELETE']}}));
  const stackDir = path.join(root, 'private/stack');
  fs.mkdirSync(stackDir, {recursive:true});
  fs.writeFileSync(path.join(root, 'opencode.json'), JSON.stringify({
    experimental:{subagent_depth:1},
    mcp:{servers:{gori:{type:'local',command:['gori','mcp','--db',path.join(stackDir,'gori.db')],environment:{GORI_HOME:path.join(stackDir,'gori')}}}}
  }));
  const stackLog = fs.openSync(path.join(stackDir,'supervisor.log'), 'a', 0o600);
  stack = spawn('python3', ['/opt/pentest/docker/stack.py', root], {stdio:['ignore',stackLog,stackLog]});
  fs.closeSync(stackLog);
  let ready = false;
  for (let i=0; i<120 && !interrupted; i++) {
    if (stack.exitCode !== null) break;
    if (fs.existsSync(path.join(stackDir,'ready.json')) && await listening(3333)) { ready = true; break; }
    await delay(500);
  }
  if (!ready) throw Error('browser and Gori stack failed to start');
  emit('started', {phase:'exploration'});
  const auth = JSON.parse(fs.readFileSync(path.join(process.env.HOME,'.pi/agent/auth.json'), 'utf8'));
  const key = auth['opencode-go']?.key;
  if (!key) throw Error('OpenCode Go credential unavailable');
  if (interrupted) throw Error('cancelled');
  const log = fs.openSync(path.join(root,'opencode.jsonl'), 'a', 0o600);
  const probe = net.createServer();
  await new Promise((resolve,reject) => { probe.once('error',reject); probe.listen(0,'127.0.0.1',resolve); });
  const port = probe.address().port;
  await new Promise(resolve => probe.close(resolve));
  const address = `http://127.0.0.1:${port}`;
  const password = randomBytes(24).toString('hex');
  const env = {...process.env,PWD:root,OPENCODE_API_KEY:key,OPENCODE_SERVER_PASSWORD:password,INTERCEPTOR_TEMP:path.join(stackDir,'interceptor'),GORI_HOME:path.join(stackDir,'gori')};
  const startServer = async () => {
    server = spawn('opencode',['serve','--hostname','127.0.0.1','--port',String(port)],{cwd:root,env,stdio:['ignore',log,log]});
    let spawnFailed = false;
    server.once('error', () => { spawnFailed = true; });
    for (let i=0; i<120 && !interrupted && !spawnFailed && server.exitCode === null && server.signalCode === null; i++) {
      try {
        const response = await fetch(address+'/api/skill',{headers:{Authorization:'Basic '+Buffer.from('opencode:'+password).toString('base64')},signal:AbortSignal.timeout(1000)});
        const data = await response.json();
        if (response.ok && ['pentest','pentest-report'].every(id => data.data?.some(s => s.id===id))) return true;
      } catch {}
      await delay(500);
    }
    if (server.exitCode === null && server.signalCode === null) server.kill('SIGTERM');
    return false;
  };
  if (!await startServer()) { fs.closeSync(log); throw Error('OpenCode could not discover pentest skills'); }
  let timedOut = false;
  const timeout = setTimeout(() => { timedOut = true; stopChild(); }, OPENCODE_TIMEOUT_MS);
  let rootSession = '', code = null, monitorBusy = false, rapidFailures = 0, reporting = false;
  const monitor = new StallMonitor();
  const watchdog = setInterval(async () => {
    if (monitorBusy || interrupted || timedOut || !rootSession) return;
    monitorBusy = true;
    try {
      const stopped = await pollStalls({address,password,rootId:rootSession,monitor});
      if (stopped.length) console.error(`Interrupted ${stopped.length} stalled OpenCode session(s); preserving workspace.`);
    } catch { /* Retry monitoring on the next interval without disrupting an active scan. */ }
    finally { monitorBusy = false; }
  }, 15000);
  try {
    while (!interrupted && !timedOut) {
      if (server.exitCode !== null || server.signalCode !== null) {
        console.error('OpenCode server stopped; restarting the workspace server.');
        if (!await startServer()) { await delay(10000); continue; }
      }
      const cliArgs = ['run','--server',address,'--auto','--model','opencode-go/deepseek-v4.1-flash','--format','json'];
      if (rootSession) cliArgs.push('--session',rootSession);
      cliArgs.push(reporting ? 'Use skill pentest-report to review the saved pentest report and evidence and write result-draft.json for Orchis.' : prompt);
      const startedAt = Date.now();
      child = spawn('opencode', cliArgs, {cwd:root,env,stdio:['ignore','pipe','pipe'],detached:true});
      let pending = '';
      child.stdout.on('data', chunk => {
        fs.writeSync(log, chunk);
        pending += chunk.toString('utf8');
        let newline;
        while ((newline = pending.indexOf('\n')) >= 0) {
          const line = pending.slice(0,newline); pending = pending.slice(newline+1);
          try { const event = JSON.parse(line); if (!rootSession && /^ses_[a-zA-Z0-9]+$/.test(event.sessionID || '')) rootSession = event.sessionID; } catch {}
        }
        if (pending.length > 4*1024*1024) pending = '';
      });
      child.stderr.on('data', chunk => fs.writeSync(log, chunk));
      code = await wait(child);
      if (interrupted || timedOut) break;
      if (code === 0) {
        if (reporting) break;
        // An independent OpenCode session must review the evidence and produce
        // the machine-readable result, even when the pentester wrote a draft.
        reporting = true; rootSession = ''; rapidFailures = 0;
        emit('state',{phase:'report'});
        continue;
      }
      if (!rootSession) break; // No known session can be resumed safely.
      console.error('OpenCode stopped without a result; resuming the same session.');
      rapidFailures = Date.now() - startedAt > 120000 ? 0 : rapidFailures+1;
      await delay(Math.min(60000, 10000 * Math.max(1, rapidFailures)));
    }
  } finally { clearInterval(watchdog); clearTimeout(timeout); fs.closeSync(log); }
  if (interrupted) throw Error('cancelled');
  if (timedOut) throw Error('OpenCode time limit reached before the independent report review completed; workspace retained for recovery.');
  if (!reporting || code !== 0) throw Error('OpenCode did not complete the independent report review; workspace retained for recovery.');
  if (!fs.existsSync(path.join(root,'result-draft.json'))) throw Error('OpenCode completed without the required JSON result; workspace retained for recovery.');
  emit('state',{phase:'report'});
  const draft = JSON.parse(fs.readFileSync(path.join(root,'result-draft.json'),'utf8'));
  const result = prepareResult(draft, root, session);
  // Set only after the separate reporting session exits successfully.
  result.review_completed = true;
  const disposition = result.disposition;
  const file = path.join(root,'result.json'); fs.writeFileSync(file,JSON.stringify(result));
  emit(disposition,{phase:'report',resultFile:file});
  process.exitCode = disposition === 'complete' ? 0 : 2;
} catch (error) {
  console.error('OpenCode job:', error.message);
  const disposition = interrupted ? 'cancelled' : 'failed';
  const file = path.join(root,'result.json');
  fs.writeFileSync(file,JSON.stringify({schema:1,session_id:session,disposition,phase:'setup',summary:'The OpenCode pentest did not complete.',phases:[],verified_tasks:0,blockers:[error.message],findings:[],report:'The pentest did not complete.',validation:'No findings were validated.'}));
  emit(disposition,{phase:'setup',resultFile:file}); process.exitCode = interrupted ? 130 : 1;
} finally {
  if (server && server.exitCode === null) { server.kill('SIGTERM'); }
  if (stack && stack.exitCode === null) { stack.kill('SIGTERM'); let timer; await Promise.race([wait(stack),new Promise(resolve => { timer = setTimeout(resolve, 50000); })]); clearTimeout(timer); if (stack.exitCode === null) stack.kill('SIGKILL'); }
}

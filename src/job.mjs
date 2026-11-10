#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import net from 'node:net';
import {spawn, spawnSync} from 'node:child_process';
import {copyScannerInputs} from './scanners.mjs';
import {prepareResult} from './result.mjs';
import {OPENCODE_TIMEOUT_MS} from './deadline.mjs';

process.umask(0o077);
const args = process.argv.slice(2);
const arg = name => { const i = args.indexOf(name); return i < 0 ? '' : args[i + 1]; };
const root = path.resolve(arg('--dir') || '.');
const session = path.basename(root);
const emit = (event, extra = {}) => console.log(JSON.stringify({schema: 1, event, sessionId: session, ...extra}));
let stack, child, interrupted = false;
const stopChild = () => { if (child && child.exitCode === null) { const pid = child.pid; try { process.kill(-pid, 'SIGTERM'); } catch {} const watchdog = setTimeout(() => { try { process.kill(-pid, 'SIGKILL'); } catch {} }, 5000); watchdog.unref(); } };
const stop = () => { interrupted = true; stopChild(); if (stack && stack.exitCode === null) stack.kill('SIGTERM'); };
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
  // The harness's Pi-specific agent instructions are not part of the OpenCode prompt.
  fs.rmSync(path.join(root, 'AGENTS.md'), {force:true});
  fs.copyFileSync('/opt/orchis-opencode/skills/pentest-report/SKILL.md', path.join(root, '.agent/skills/pentest-report/SKILL.md'));
  fs.mkdirSync(path.join(root, '.opencode'), {recursive: true});
  fs.symlinkSync('../.agent/skills', path.join(root, '.opencode/skills'), 'dir');
  fs.writeFileSync(path.join(root, 'state.json'), JSON.stringify({mode:'running', scope:{origins:[url.origin], methods:['GET']}}));
  const stackDir = path.join(root, 'private/stack');
  fs.mkdirSync(stackDir, {recursive:true});
  fs.writeFileSync(path.join(root, 'opencode.json'), JSON.stringify({
    permissions:[{action:'subagent',resource:'*',effect:'deny'}],
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
  child = spawn('opencode', ['run','--standalone','--auto','--model','opencode-go/deepseek-v4.1-flash','--format','json',prompt], {cwd:root,env:{...process.env,OPENCODE_API_KEY:key},stdio:['ignore',log,log],detached:true});
  let timedOut = false;
  const timeout = setTimeout(() => { timedOut = true; stopChild(); }, OPENCODE_TIMEOUT_MS);
  const code = await wait(child); clearTimeout(timeout); fs.closeSync(log);
  if (interrupted) throw Error('cancelled');
  if (timedOut) throw Error('OpenCode time limit reached');
  if (code !== 0) throw Error('OpenCode did not complete');
  emit('state',{phase:'report'});
  const draft = JSON.parse(fs.readFileSync(path.join(root,'result-draft.json'),'utf8'));
  const result = prepareResult(draft, root, session);
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
  if (stack && stack.exitCode === null) { stack.kill('SIGTERM'); let timer; await Promise.race([wait(stack),new Promise(resolve => { timer = setTimeout(resolve, 50000); })]); clearTimeout(timer); if (stack.exitCode === null) stack.kill('SIGKILL'); }
}

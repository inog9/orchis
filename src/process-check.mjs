import fs from 'node:fs';
import path from 'node:path';

// Return true only when we can reliably conclude that the exact compound
// shell command is no longer running. Unknown must never be treated as gone.
export function orphanedShell(command, {proc = '/proc', uid = process.getuid?.()} = {}) {
  if (typeof command !== 'string' || !command || command.includes('\0') || uid === undefined) return null;
  // Single commands can replace the shell via exec. Background jobs can outlive it.
  if (!/[;\n|]/.test(command) || /(^|[^&])&(?!&)/.test(command) || /\bexec\b/.test(command)) return null;
  let entries;
  try { entries = fs.readdirSync(proc); } catch { return null; }
  let unknown = false;
  for (const pid of entries) {
    if (!/^\d+$/.test(pid)) continue;
    const dir = path.join(proc,pid);
    try {
      if (fs.statSync(dir).uid !== uid) continue;
      const argv = fs.readFileSync(path.join(dir,'cmdline')).toString('utf8').split('\0');
      const name = path.basename(argv[0] || '');
      if (!['sh','bash','dash','zsh'].includes(name)) continue;
      const index = argv.indexOf('-c');
      if (index >= 0 && argv[index+1] === command) return false;
    } catch (error) {
      if (!['ENOENT','ESRCH'].includes(error.code)) unknown = true;
    }
  }
  return unknown ? null : true;
}

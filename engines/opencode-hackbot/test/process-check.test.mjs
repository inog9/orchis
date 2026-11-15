import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {orphanedShell} from '../src/process-check.mjs';

test('only declares the exact compound shell command orphaned when its process is absent', () => {
  const dir=fs.mkdtempSync(path.join(os.tmpdir(),'orchis-proc-'));
  try {
    const p=path.join(dir,'123');fs.mkdirSync(p);
    const command='cd /tmp/js; for f in *.js; do grep x "$f" | sort; done';
    fs.writeFileSync(path.join(p,'cmdline'),['/usr/bin/bash','-c',command,''].join('\0'));
    assert.equal(orphanedShell(command,{proc:dir}),false);
    fs.rmSync(p,{recursive:true});
    assert.equal(orphanedShell(command,{proc:dir}),true);
    assert.equal(orphanedShell('curl https://example.com/',{proc:dir}),null);
    assert.equal(orphanedShell('run & wait; echo done',{proc:dir}),null);
    assert.equal(orphanedShell('exec curl x; echo done',{proc:dir}),null);
    assert.equal(orphanedShell(command,{proc:path.join(dir,'missing')}),null);
  } finally { fs.rmSync(dir,{recursive:true,force:true}); }
});

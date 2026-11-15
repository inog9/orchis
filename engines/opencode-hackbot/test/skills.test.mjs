import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {restrictSkills} from '../src/skills.mjs';

test('prepares the real bundled skills without removed specialists', t => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'orchis-skills-'));
  t.after(() => fs.rmSync(root, {recursive:true, force:true}));
  const bundled = fileURLToPath(new URL('../../hackbot-lite/skills/', import.meta.url));
  const skills = path.join(root, 'skills');
  fs.cpSync(bundled, skills, {recursive:true});
  assert.equal(fs.existsSync(path.join(skills, 'pentest-gadgets')), false);
  assert.equal(fs.existsSync(path.join(skills, 'wordpress-pentest')), false);
  restrictSkills(skills);
  for (const name of ['pentest-fuzzing','pentest-endpoint-fuzz-full','pentest-gadgets','pentest-dependency-confusion','wordpress-pentest']) {
    assert.equal(fs.existsSync(path.join(skills, name)), false, `${name} should not be installed`);
  }
  const main = fs.readFileSync(path.join(skills, 'pentest/SKILL.md'), 'utf8');
  assert.match(main, /Exclude endpoint fuzzing, gadget hunting and dependency-confusion checks/);
  assert.doesNotMatch(main, /wordpress-pentest|pentest-gadgets|pentest-dependency-confusion|pentest-fuzzing|pentest-endpoint-fuzz-full/);
  assert.ok(fs.existsSync(path.join(skills, 'pentest-report/SKILL.md')));
});

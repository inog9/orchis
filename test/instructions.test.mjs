import {test} from 'node:test';
import assert from 'node:assert/strict';
import {buildInstructions} from '../src/instructions.mjs';

test('pentest prompt continues useful coverage without an early-finish instruction', () => {
  const prompt = buildInstructions('Authorized target: https://example.com/');
  assert.ok(prompt.startsWith('Authorized target: https://example.com/'));
  assert.match(prompt, /Continue investigating and validating independent leads/);
  assert.match(prompt, /until the available in-scope work is covered or a specific blocker/);
  assert.doesNotMatch(prompt, /return promptly|bounded (?:security checks|testing)/i);
  assert.match(prompt, /result-draft\.json/);
});

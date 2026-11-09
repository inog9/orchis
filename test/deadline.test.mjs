import {test} from 'node:test';
import assert from 'node:assert/strict';
import {OPENCODE_TIMEOUT_MS} from '../src/deadline.mjs';

test('the OpenCode run is allowed four hours', () => {
  assert.equal(OPENCODE_TIMEOUT_MS, 14_400_000);
});

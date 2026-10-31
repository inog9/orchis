import { describe, expect, it } from 'vitest';
import { ApiError } from '../api/client';
import { describeError } from './ErrorNote';

describe('describeError', () => {
  it('422 dengan fields menampilkan alasan sebenarnya, bukan teks umum', () => {
    const e = new ApiError(422, 'Validation failed', 'One or more fields are invalid.', 'err_1', 'req', [{ name: 'scenario', message: 'plain-box has no agent.' }]);
    expect(describeError(e).message).toBe('plain-box has no agent.');
  });
  it('tanpa fields memakai detail; 5xx tidak membocorkan detail server', () => {
    expect(describeError(new ApiError(409, 'Conflict', 'Already running.')).message).toBe('Already running.');
    expect(describeError(new ApiError(500, 'x', 'stack trace here', 'err_9')).message).not.toContain('stack');
  });
});

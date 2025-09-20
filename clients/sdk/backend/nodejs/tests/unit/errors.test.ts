import { describe, it, expect } from 'vitest';
import { ApiError } from '../../src/errors';

describe('ApiError', () => {
  it('retains status and body', () => {
    const err = new ApiError(400, { message: 'bad' });
    expect(err.status).toBe(400);
    expect(err.body).toEqual({ message: 'bad' });
    expect(err.message).toContain('API error');
  });
});


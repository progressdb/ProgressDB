import { describe, it, expect } from 'vitest';
import { server, rest } from '../setup';
import { httpRequest } from '../../src/http';
import { ApiError } from '../../src/errors';

const baseUrl = 'http://example.test';

describe('httpRequest', () => {
  it('parses JSON responses', async () => {
    server.use(rest.get(`${baseUrl}/json`, (_req, res, ctx) => res(ctx.json({ ok: true }))));
    const res = await httpRequest<{ ok: boolean }>(baseUrl, 'GET', '/json');
    expect(res.ok).toBe(true);
  });

  it('returns text for non-json responses', async () => {
    server.use(rest.get(`${baseUrl}/text`, (_req, res, ctx) => res(ctx.text('hello'))));
    const res = await httpRequest<string>(baseUrl, 'GET', '/text');
    expect(res).toBe('hello');
  });

  it('throws ApiError on non-2xx', async () => {
    server.use(rest.get(`${baseUrl}/bad`, (_req, res, ctx) => res(ctx.status(400), ctx.json({ err: 'x' }))));
    await expect(httpRequest(baseUrl, 'GET', '/bad')).rejects.toBeInstanceOf(ApiError);
  });

  it('retries on transient network errors', async () => {
    let called = 0;
    server.use(rest.get(`${baseUrl}/flaky`, (_req, res, ctx) => {
      called += 1;
      if (called < 2) return res.networkError('timeout');
      return res(ctx.json({ ok: true }));
    }));
    const res = await httpRequest<{ ok: boolean }>(baseUrl, 'GET', '/flaky', undefined, {}, { timeoutMs: 1000, maxRetries: 2 });
    expect(res.ok).toBe(true);
    expect(called).toBeGreaterThanOrEqual(2);
  });
});


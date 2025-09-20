import { describe, it, expect } from 'vitest';
import { server, rest } from '../setup';
import BackendClient from '../../src/client';

const baseUrl = 'http://api.test';

describe('BackendClient.request', () => {
  it('includes Authorization header and merges extra headers', async () => {
    const client = new BackendClient({ baseUrl, apiKey: 'secret' });
    server.use(rest.get(`${baseUrl}/hello`, (req, res, ctx) => {
      const auth = req.headers.get('authorization');
      const extra = req.headers.get('x-custom');
      return res(ctx.json({ auth, extra }));
    }));

    const res = await client.request<{ auth?: string; extra?: string }>('GET', '/hello', undefined, { 'X-Custom': 'v' });
    expect(res.auth).toBe('Bearer secret');
    expect(res.extra).toBe('v');
  });
});


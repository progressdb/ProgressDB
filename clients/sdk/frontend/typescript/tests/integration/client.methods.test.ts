import { describe, it, expect } from 'vitest';
import { server, rest } from '../setup';
import ProgressDBClient from '../../src/index';

const baseUrl = 'http://api.test';

describe('Frontend SDK integration (mocked)', () => {
  it('createThread posts and returns thread', async () => {
    server.use(rest.post(`${baseUrl}/frontend/v1/threads`, async (req, res, ctx) => {
      const body = await req.json();
      return res(ctx.json({ key: 't1' }));
    }));
    const client = new ProgressDBClient({ baseUrl, apiKey: 'k' });
    const t = await client.createThread({ title: 'hello' });
    expect(t.key).toBe('t1');
  });
});


import { describe, it, expect } from 'vitest';
import { server, rest } from '../setup';
import BackendClient from '../../src/client';

const baseUrl = 'http://api.test';

describe('BackendClient methods (integration - mocked)', () => {
  it('createThread sends X-User-ID header and POST body', async () => {
    const client = new BackendClient({ baseUrl, apiKey: 'k' });
    server.use(rest.post(`${baseUrl}/v1/threads`, async (req, res, ctx) => {
      const author = req.headers.get('x-user-id');
      const body = await req.json();
      return res(ctx.json({ id: 't1', ...body, author }));
    }));

    const t = await client.createThread({ title: 'hello' }, 'author1');
    expect(t.id).toBe('t1');
    expect(t.title).toBe('hello');
    expect(t.author).toBe('author1');
  });

  it('listThreads requires author for backend', async () => {
    const client = new BackendClient({ baseUrl, apiKey: 'k' });
    await expect(client.listThreads({ author: '' } as any)).rejects.toBeTruthy();
  });

  it('addOrUpdateReaction posts reaction object', async () => {
    const client = new BackendClient({ baseUrl, apiKey: 'k' });
    server.use(rest.post(`${baseUrl}/v1/threads/t1/messages/m1/reactions`, async (req, res, ctx) => {
      const body = await req.json();
      return res(ctx.json({ id: 'm1', body }));
    }));

    const out = await client.addOrUpdateReaction('t1', 'm1', { id: 'u1', reaction: 'like' });
    expect(out.id).toBe('m1');
  });
});


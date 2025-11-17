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

  it('listThreads returns threads list', async () => {
    server.use(rest.get(`${baseUrl}/frontend/v1/threads`, (_req, res, ctx) => 
      res(ctx.json({ 
        threads: [{ key: 't1', title: 'Thread 1' }],
        pagination: { before_anchor: 't1', after_anchor: 't1', has_before: false, has_after: false, count: 1, total: 1 }
      }))
    ));
    const client = new ProgressDBClient({ baseUrl, apiKey: 'k' });
    const result = await client.listThreads();
    expect(result.threads).toHaveLength(1);
    expect(result.threads[0].key).toBe('t1');
    expect(result.pagination).toBeDefined();
  });

  it('createThreadMessage creates message in thread', async () => {
    server.use(rest.post(`${baseUrl}/frontend/v1/threads/t1/messages`, async (req, res, ctx) => {
      const body = await req.json();
      return res(ctx.json({ message: { key: 'm1', body: body.body } }));
    }));
    const client = new ProgressDBClient({ baseUrl, apiKey: 'k' });
    const result = await client.createThreadMessage('t1', { body: 'Hello world' });
    expect(result.message.key).toBe('m1');
    expect(result.message.body).toBe('Hello world');
  });

  it('listThreadMessages returns messages list', async () => {
    server.use(rest.get(`${baseUrl}/frontend/v1/threads/t1/messages`, (_req, res, ctx) => 
      res(ctx.json({ 
        thread: 't1',
        messages: [{ key: 'm1', body: 'Hello' }],
        pagination: { before_anchor: 'm1', after_anchor: 'm1', has_before: false, has_after: false, count: 1, total: 1 }
      }))
    ));
    const client = new ProgressDBClient({ baseUrl, apiKey: 'k' });
    const result = await client.listThreadMessages('t1');
    expect(result.messages).toHaveLength(1);
    expect(result.messages[0].key).toBe('m1');
    expect(result.thread).toBe('t1');
    expect(result.pagination).toBeDefined();
  });

  it('healthz returns health status', async () => {
    server.use(rest.get(`${baseUrl}/healthz`, (_req, res, ctx) => 
      res(ctx.json({ status: 'ok' }))
    ));
    const client = new ProgressDBClient({ baseUrl, apiKey: 'k' });
    const result = await client.healthz();
    expect(result.status).toBe('ok');
  });

  it('readyz returns readiness status', async () => {
    server.use(rest.get(`${baseUrl}/readyz`, (_req, res, ctx) => 
      res(ctx.json({ status: 'ok', version: '1.0.0' }))
    ));
    const client = new ProgressDBClient({ baseUrl, apiKey: 'k' });
    const result = await client.readyz();
    expect(result.status).toBe('ok');
    expect(result.version).toBe('1.0.0');
  });
});


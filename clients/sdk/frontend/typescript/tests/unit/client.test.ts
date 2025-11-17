import { describe, it, expect } from 'vitest';
import { server, rest } from '../setup';
import ProgressDBClient from '../../src/index';

const baseUrl = 'http://api.test';

describe('Frontend ProgressDBClient', () => {
  it('creates client with default options', () => {
    const client = new ProgressDBClient();
    expect(client.baseUrl).toBe('');
    expect(client.apiKey).toBeUndefined();
    expect(client.defaultUserId).toBeUndefined();
    expect(client.defaultUserSignature).toBeUndefined();
  });

  it('creates client with custom options', () => {
    const client = new ProgressDBClient({ 
      baseUrl: 'https://api.example.com',
      apiKey: 'test-key',
      defaultUserId: 'user123',
      defaultUserSignature: 'sig456'
    });
    expect(client.baseUrl).toBe('https://api.example.com');
    expect(client.apiKey).toBe('test-key');
    expect(client.defaultUserId).toBe('user123');
    expect(client.defaultUserSignature).toBe('sig456');
  });

  it('listThreadMessages parses response', async () => {
    server.use(rest.get(`${baseUrl}/frontend/v1/threads/thread1/messages`, (_req, res, ctx) => 
      res(ctx.json({ 
        thread: 'thread1',
        messages: [{ key: 'm1', body: 'test' }],
        pagination: { before_anchor: 'm1', after_anchor: 'm1', has_before: false, has_after: false, count: 1, total: 1 }
      }))
    ));
    const client = new ProgressDBClient({ baseUrl, apiKey: 'k' });
    const res = await client.listThreadMessages('thread1', {}, undefined, undefined);
    expect(res.messages).toBeDefined();
    expect(res.messages).toHaveLength(1);
    expect(res.thread).toBe('thread1');
    expect(res.pagination).toBeDefined();
  });

  it('listThreads parses response', async () => {
    server.use(rest.get(`${baseUrl}/frontend/v1/threads`, (_req, res, ctx) => 
      res(ctx.json({ 
        threads: [{ key: 't1', title: 'Test Thread' }],
        pagination: { before_anchor: 't1', after_anchor: 't1', has_before: false, has_after: false, count: 1, total: 1 }
      }))
    ));
    const client = new ProgressDBClient({ baseUrl, apiKey: 'k' });
    const res = await client.listThreads({}, undefined, undefined);
    expect(res.threads).toBeDefined();
    expect(res.threads).toHaveLength(1);
    expect(res.pagination).toBeDefined();
  });

  it('getThread parses response', async () => {
    server.use(rest.get(`${baseUrl}/frontend/v1/threads/t1`, (_req, res, ctx) => 
      res(ctx.json({ thread: { key: 't1', title: 'Test Thread' } }))
    ));
    const client = new ProgressDBClient({ baseUrl, apiKey: 'k' });
    const res = await client.getThread('t1', {}, undefined, undefined);
    expect(res.thread).toBeDefined();
    expect(res.thread.key).toBe('t1');
  });

  it('createThreadMessage parses response', async () => {
    server.use(rest.post(`${baseUrl}/frontend/v1/threads/t1/messages`, async (req, res, ctx) => {
      const body = await req.json();
      return res(ctx.json({ message: { key: 'm1', body: body.body } }));
    }));
    const client = new ProgressDBClient({ baseUrl, apiKey: 'k' });
    const res = await client.createThreadMessage('t1', { body: 'Hello' }, undefined, undefined);
    expect(res.message).toBeDefined();
    expect(res.message.key).toBe('m1');
    expect(res.message.body).toBe('Hello');
  });

  it('getThreadMessage parses response', async () => {
    server.use(rest.get(`${baseUrl}/frontend/v1/threads/t1/messages/m1`, (_req, res, ctx) => 
      res(ctx.json({ message: { key: 'm1', body: 'Test message' } }))
    ));
    const client = new ProgressDBClient({ baseUrl, apiKey: 'k' });
    const res = await client.getThreadMessage('t1', 'm1', undefined, undefined);
    expect(res.message).toBeDefined();
    expect(res.message.key).toBe('m1');
  });
});


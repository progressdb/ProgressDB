import { describe, it, expect } from 'vitest';
import { server, rest } from '../setup';
import ProgressDBClient from '../../src/index';

const baseUrl = 'http://api.test';

describe('Frontend ProgressDBClient', () => {
  it('listMessages parses response', async () => {
    server.use(rest.get(`${baseUrl}/v1/messages`, (_req, res, ctx) => res(ctx.json({ messages: [] }))));
    const client = new ProgressDBClient({ baseUrl, apiKey: 'k' });
    const res = await client.listMessages({}, undefined, undefined);
    expect(res.messages).toBeDefined();
  });
});


import React from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import { describe, it, expect } from 'vitest';
import { server } from '../setup';
import { rest } from 'msw';
import ProgressDBProvider, { useMessages } from '../../src/index';

describe('useMessages integration (mocked)', () => {
  it('fetches messages for a thread', async () => {
    server.use(rest.get('http://api.test/frontend/v1/threads/t1/messages', (_req, res, ctx) => res(ctx.json({ messages: [{ id: 'm1', body: { text: 'hi' } }] }))));

    const TestComp: React.FC = () => {
      const { messages, loading } = useMessages('t1');
      if (loading) return <div>loading</div>;
      return <div data-testid="count">{messages ? messages.length : 0}</div>;
    };

    render(
      <ProgressDBProvider options={{ baseUrl: 'http://api.test', apiKey: 'k' }} getUserSignature={() => ({ userId: 'u', signature: 's' })}>
        <TestComp />
      </ProgressDBProvider>
    );

    await waitFor(() => expect(screen.getByTestId('count').textContent).toBe('1'));
  });
});


import React from 'react';
import { render } from '@testing-library/react';
import { describe, it, expect } from 'vitest';
import ProgressDBProvider, { useMessages } from '../../src/index';
import React from 'react';

// Minimal smoke test to ensure hooks mount correctly by rendering a
// component that uses the hook inside the provider.
describe('useMessages hook', () => {
  it('returns initial state when no thread provided', () => {
    const TestComp: React.FC = () => {
      const { messages } = useMessages(undefined);
      return <div data-testid="val">{messages === null ? 'null' : 'ok'}</div>;
    };

    const wrapper = ({ children }: any) => (
      <ProgressDBProvider options={{ baseUrl: 'http://api.test', apiKey: 'k' }} getUserSignature={() => ({ userId: 'u', signature: 's' })}>{children}</ProgressDBProvider>
    );

    const { getByTestId } = render(<TestComp />, { wrapper });
    expect(getByTestId('val').textContent).toBe('null');
  });
});

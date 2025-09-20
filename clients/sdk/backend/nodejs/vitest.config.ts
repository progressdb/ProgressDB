import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    environment: 'node',
    setupFiles: './tests/setup.ts',
    globals: false,
    coverage: {
      reporter: ['text', 'lcov'],
    },
  },
});


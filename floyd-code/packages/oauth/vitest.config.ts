import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    name: 'floyd-oauth',
    include: ['test/**/*.test.ts'],
  },
});

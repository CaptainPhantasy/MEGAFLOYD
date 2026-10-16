import { defineConfig } from 'tsdown';

import { rawTextPlugin } from '../../build/raw-text-plugin.mjs';

export default defineConfig({
  entry: ['./src/index.ts'],
  format: ['esm'],
  dts: true,
  outDir: 'dist',
  clean: true,
  plugins: [rawTextPlugin()],
  deps: {
    neverBundle: [
      '@legacy-ai/floyd-code-oauth',
      '@legacy-ai/floyd-telemetry',
    ],
  },
});

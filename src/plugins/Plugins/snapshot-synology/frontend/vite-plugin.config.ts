// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import path from 'path';
import fs from 'fs';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const slug = 'snapshot-synology';

const repoRoot = path.resolve(__dirname, '../../../');
const hostSrc = path.resolve(repoRoot, '..', 'frontend');

if (!fs.existsSync(hostSrc)) {
  throw new Error(
    `vite-plugin.config.ts: host frontend not found at ${hostSrc}. ` +
    `Adjust the relative offset if the plugin moved.`,
  );
}

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, 'src'),
      '@core': path.resolve(hostSrc, 'core'),
      '@resources': path.resolve(hostSrc, 'resources'),
      '@modules': path.resolve(hostSrc, 'modules'),
      '@tabler/icons-react': path.resolve(__dirname, 'node_modules/@tabler/icons-react'),
      react: path.resolve(__dirname, 'node_modules/react'),
      'react/jsx-runtime': path.resolve(__dirname, 'node_modules/react/jsx-runtime.js'),
      'react-dom': path.resolve(__dirname, 'node_modules/react-dom'),
      'react-dom/client': path.resolve(__dirname, 'node_modules/react-dom/client'),
      'react-i18next': path.resolve(__dirname, 'node_modules/react-i18next'),
      i18next: path.resolve(__dirname, 'node_modules/i18next'),
    },
    dedupe: ['react', 'react-dom', 'react-i18next', 'i18next'],
  },
  build: {
    outDir: path.resolve(__dirname, '../frontend'),
    emptyOutDir: false,
    banner:
      "var process = { env: { NODE_ENV: 'production' }, emit: function () {} };",
    lib: {
      entry: path.resolve(__dirname, 'src/index.tsx'),
      formats: ['umd'],
      name: `McharborPlugin_${slug.replace(/-/g, '_')}`,
      fileName: () => `${slug}.umd.cjs`,
    },
    rollupOptions: {
      output: {
        banner:
          "globalThis.process = { env: { NODE_ENV: 'production' }, emit: function () {} };",
        inlineDynamicImports: true,
      },
    },
  },
  server: {
    fs: {
      allow: [__dirname, hostSrc, repoRoot, path.resolve(repoRoot, '..')],
    },
  },
});
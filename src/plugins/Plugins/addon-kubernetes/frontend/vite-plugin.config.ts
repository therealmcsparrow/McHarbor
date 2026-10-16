// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import path from 'path';
import fs from 'fs';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const slug = 'addon-kubernetes';
const repoRoot = path.resolve(__dirname, '../../../');
const hostSrc = path.resolve(repoRoot, '..', 'frontend');

if (!fs.existsSync(hostSrc)) {
  throw new Error(`vite-plugin.config.ts: host frontend not found at ${hostSrc}.`);
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
      // Host components reached through `@resources` import react-query too —
      // `<Button permission="...">` renders a gated variant whose
      // `usePermissionGate` chain ends in `useQuery`. Without these aliases
      // rollup resolves that import against the host's node_modules and the
      // bundle ends up carrying two react-query instances: the plugin's
      // `QueryClientProvider` fills the context of one, the host button reads
      // the other, and the page dies with "No QueryClient set". Same story for
      // react-table, which backs the shared `TablePage`.
      '@tanstack/react-query': path.resolve(__dirname, 'node_modules/@tanstack/react-query'),
      '@tanstack/react-table': path.resolve(__dirname, 'node_modules/@tanstack/react-table'),
    },
    dedupe: [
      'react',
      'react-dom',
      'react-i18next',
      'i18next',
      '@tanstack/react-query',
      '@tanstack/react-table',
    ],
  },
  // Vite leaves `process.env.NODE_ENV` alone in library mode, so React's
  // dev-only branches survive the build and React DevTools' `checkDCE` probe
  // throws "production mode, but dead code elimination has not been applied".
  // The runtime banner below already makes React take the production path;
  // this makes the dead code actually go away.
  define: {
    'process.env.NODE_ENV': JSON.stringify('production'),
  },
  build: {
    outDir: path.resolve(__dirname, '../frontend'),
    emptyOutDir: false,
    lib: {
      entry: path.resolve(__dirname, 'src/index.tsx'),
      formats: ['umd'],
      name: `McharborPlugin_${slug.replace(/-/g, '_')}`,
      fileName: () => `${slug}.umd.cjs`,
    },
    rollupOptions: {
      output: {
        banner: "globalThis.process = { env: { NODE_ENV: 'production' }, emit: function () {} };",
        inlineDynamicImports: true,
      },
    },
  },
  server: {
    fs: { allow: [__dirname, hostSrc, repoRoot, path.resolve(repoRoot, '..')] },
  },
});

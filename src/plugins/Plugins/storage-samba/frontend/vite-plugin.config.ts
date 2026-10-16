// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// vite-plugin.config.ts — Samba storage plugin UMD bundle.
//
// Host loads this via <PluginBundleLoader src="/api/plugins/loader/bundle?slug=storage-samba" />.
// Global must be `McharborPlugin_storage_samba` (slugs use underscores, not hyphens).
// File name must end in `.umd.cjs` (NOT `.umd.js`) — the plugin loader
// enforces that exact filename. See mcharbor-plugin-frontend-fix.

import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

const slug = 'storage-samba';

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: '.',
    emptyOutDir: false,
    lib: {
      entry: 'src/index.tsx',
      name: `McharborPlugin_${slug.replace(/-/g, '_')}`,
      formats: ['umd'],
      fileName: () => `${slug}.umd.cjs`,
    },
    rollupOptions: {
      // Both banners keep React 19's scheduler off the
      // "preloaded via script tag" warning at load time.
      // The host loads the bundle via <script>, so
      // `process.versions` + `process.env.NODE_ENV` are not
      // defined and the react-dom scheduler complains.
      // Defining them as no-op globals silences the warning
      // without touching react-dom internals.
      output: {
        banner: `
          var process = (typeof globalThis !== 'undefined' ? globalThis : window).process || { env: { NODE_ENV: 'production' }, versions: {} };
          process.env = process.env || { NODE_ENV: 'production' };
        `,
        intro: `
          var process = (typeof globalThis !== 'undefined' ? globalThis : window).process || { env: { NODE_ENV: 'production' }, versions: {} };
          process.env = process.env || { NODE_ENV: 'production' };
        `,
      },
      // Host provides these at load time — never bundle them.
      external: [
        'react',
        'react-dom',
        'react-dom/client',
        'react/jsx-runtime',
        'react-i18next',
        'i18next',
        '@tabler/icons-react',
      ],
    },
  },
  resolve: {
    alias: {
      // The host also provides these from its node_modules.
      // We alias them so any deep import resolves to the
      // externals list above rather than trying to bundle.
      react: 'react',
      'react-dom': 'react-dom',
      'react-i18next': 'react-i18next',
      i18next: 'i18next',
    },
  },
});
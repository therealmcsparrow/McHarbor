// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
//
// Vite config for the identity-generic-oidc plugin UMD bundle. Mirrors
// comm-signal's setup so the host loader can resolve the bundle at
// /api/plugins/loader/bundle?slug=identity-generic-oidc and the global
// symbol stays McharborPlugin_identity_generic_oidc.
//
// `process.env.NODE_ENV` banner polyfill is required because react-dom
// references it at module top level; without it the UMD factory throws
// `process is not defined` before assigning `e.mount`.

import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const slug = 'identity-generic-oidc';

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      react: path.resolve(__dirname, 'node_modules/react'),
      'react-dom': path.resolve(__dirname, 'node_modules/react-dom'),
      'react-i18next': path.resolve(__dirname, 'node_modules/react-i18next'),
      i18next: path.resolve(__dirname, 'node_modules/i18next'),
    },
  },
  build: {
    outDir: path.resolve(__dirname),
    emptyOutDir: false,
    cssCodeSplit: false,
    assetsInlineLimit: 100000000,
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
});

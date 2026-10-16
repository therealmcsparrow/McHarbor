// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
//
// Vite config for the comm-signal plugin UMD bundle. Mirrors
// comm-telegram's setup so the host loader can resolve the bundle
// at /api/plugins/loader/bundle?slug=comm-signal and the global
// symbol stays McharborPlugin_comm_signal.
//
// `process.env.NODE_ENV` banner polyfill is required because some
// upstream dependencies still call it during the production
// bundle's evaluation; without it the host sees `process is not
// defined` on first render.

import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const slug = 'comm-signal';

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
    // The scheduler chunk pulled in by react-dom references
    // `process.env.NODE_ENV` and `process.emit(...)` at module
    // top level. Without a polyfill the UMD factory throws
    // `process is not defined` before assigning `e.mount`, so
    // PluginFormOutlet sees an empty exports object and renders
    // the "plugin X did not export a mount() function" error.
    // Prepending a banner before the IIFE gives every bundle a
    // browser-safe `process` without rewriting every callsite.
    banner:
      "var process = { env: { NODE_ENV: 'production' }, emit: function () {} };",
    lib: {
      entry: path.resolve(__dirname, 'src/index.tsx'),
      formats: ['umd'],
      name: `McharborPlugin_${slug.replace(/-/g, '_')}`,
      fileName: () => `${slug}.umd.cjs`,
    },
    rollupOptions: {
      // Hooking `globalThis.process` survives the IIFE parameter
      // rebinding because the UMD wrapper IIFE binds its first
      // parameter to `process` and immediately overwrites it with
      // `globalThis`. Assigning to `process` directly gets
      // clobbered; the global hook survives.
      output: {
        banner:
          "globalThis.process = { env: { NODE_ENV: 'production' }, emit: function () {} };",
        inlineDynamicImports: true,
      },
    },
  },
});
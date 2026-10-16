// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Vite config for the storage-nfs plugin frontend UMD bundle.
// The host loads this via
//   <script src="/api/plugins/loader/bundle?slug=storage-nfs">
// and looks up the global `window.McharborPlugin_storage_nfs`.
//
// React + react-dom + react-i18next + i18next are aliased to the
// plugin's local node_modules and bundled inline. The host does
// NOT expose `React` / `ReactDOM` / `ReactDOMClient` as globals,
// so anything declared `external` here would resolve to
// `undefined` at runtime — the UMD wrapper would never assign
// `mount`, and PluginFormOutlet would render "did not export a
// mount() function". This pattern matches the working
// storage-sharepoint-online plugin.

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const slug = 'storage-nfs';

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      // Bundle React + i18next inline so the UMD wrapper does not
      // depend on the host providing any of these as globals.
      react: path.resolve(__dirname, 'node_modules/react'),
      'react/jsx-runtime': path.resolve(__dirname, 'node_modules/react/jsx-runtime.js'),
      'react-dom': path.resolve(__dirname, 'node_modules/react-dom'),
      'react-dom/client': path.resolve(__dirname, 'node_modules/react-dom/client'),
      'react-i18next': path.resolve(__dirname, 'node_modules/react-i18next'),
      i18next: path.resolve(__dirname, 'node_modules/i18next'),
      '@tabler/icons-react': path.resolve(__dirname, 'node_modules/@tabler/icons-react'),
    },
    dedupe: ['react', 'react-dom', 'react-i18next', 'i18next'],
  },
  build: {
    outDir: path.resolve(__dirname),
    emptyOutDir: false,
    lib: {
      entry: path.resolve(__dirname, 'src/index.tsx'),
      name: `McharborPlugin_${slug.replace(/-/g, '_')}`,
      formats: ['umd'],
      fileName: () => `${slug}.umd.cjs`,
    },
    rollupOptions: {
      output: {
        // The bundle is loaded via <script> inside the host document,
        // which doesn't define `process`. Stub it so react-dom's
        // scheduler chunk doesn't crash on `process.env` /
        // `process.emit`. Hooking globalThis survives the UMD IIFE
        // parameter rebinding (assigning to `process` directly gets
        // clobbered by the wrapper).
        banner: "globalThis.process = { env: { NODE_ENV: 'production' }, emit: function () {} };",
        inlineDynamicImports: true,
      },
    },
  },
  define: {
    'process.env.NODE_ENV': JSON.stringify('production'),
  },
});

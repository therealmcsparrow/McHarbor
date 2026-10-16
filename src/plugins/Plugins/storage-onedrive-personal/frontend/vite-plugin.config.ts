// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import path from 'path';
import fs from 'fs';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const slug = 'storage-onedrive-personal';

// Resolve the host's frontend source so we can bundle the shared
// UI primitives (@resources/components/ui/Input, Layout, Form,
// Typography, etc.) into the UMD. The plugin lives at
// `src/plugins/Plugins/<slug>/frontend/` — four levels up reaches
// `src/`, and `src/frontend/` sits next to `src/plugins/`.
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
    },
    dedupe: ['react', 'react-dom', 'react-i18next', 'i18next'],
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
      // @resources / @core / @modules are bundled in. React and
      // i18next are bundled in too — matches the working plugin
      // pattern (email-smtp, comm-slack). The host cannot expose
      // them as globals because its main bundle is an ES module.
      output: {
        // The scheduler chunk pulled in by react-dom references
        // `process.env.NODE_ENV` and `process.emit(...)` at module
        // top level. Without a polyfill the UMD factory throws
        // `process is not defined` before assigning `e.mount`, so
        // PluginFormOutlet sees an empty exports object. Assigning
        // to globalThis.process (not bare `var process = ...`)
        // survives the UMD IIFE wrapper which rebinds its first
        // parameter to globalThis, clobbering any local declaration.
        banner:
          "globalThis.process = { env: { NODE_ENV: 'production' }, emit: function () {} };",
      },
    },
  },
  server: {
    fs: {
      allow: [__dirname, hostSrc, repoRoot, path.resolve(repoRoot, '..')],
    },
  },
});

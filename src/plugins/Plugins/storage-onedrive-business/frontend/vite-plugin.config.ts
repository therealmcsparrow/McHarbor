// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import path from 'path';
import fs from 'fs';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const slug = 'storage-onedrive-business';

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
      // @resources / @core / @modules are bundled in. React and
      // i18next are bundled in too — matches the working plugin
      // pattern (storage-onedrive-personal, email-smtp, comm-slack).
      // The host cannot expose them as globals because its main
      // bundle is an ES module, so a UMD plugin that lists them
      // as externals runs `t.createRoot(...)` on `undefined` and
      // silently mounts nothing.
      output: {
        // The scheduler chunk pulled in by react-dom references
        // `process.env.NODE_ENV` and `process.emit(...)` at module
        // top level. Without a polyfill the UMD factory throws
        // `process is not defined` before assigning `e.mount`, so
        // PluginFormOutlet sees an empty exports object and renders
        // the "plugin X did not export a mount() function" error.
        // Prepending a banner before the IIFE gives every bundle a
        // browser-safe `process` without rewriting every callsite.
        // Assigning to `process` directly works in principle, but the
        // UMD wrapper IIFE binds its first parameter to `process` and
        // immediately overwrites it with `globalThis`. That clobbers
        // a `var process = ...` banner before the factory body runs.
        // Hooking `globalThis.process` survives the IIFE parameter
        // rebinding because the wrapper only reassigns its *local*
        // parameter, not the global.
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

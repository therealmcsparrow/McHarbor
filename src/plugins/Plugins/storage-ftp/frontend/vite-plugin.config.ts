// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Vite config for the storage-ftp plugin frontend bundle. The
// build emits a UMD blob the host loads via the plugin-runtime
// loader. The bundle is consumed by the host's PluginPageHost
// mount flow; the host provides react / react-dom /
// react-i18next / i18next as externals so the bundle does not
// duplicate them.
//
// Two globals are pinned for the loader:
//   - McharborPlugin_storage_ftp   (loader looks for this name)
//   - storage-ftp.umd.cjs          (the file the host fetches)
//
// See .claude/skills/plugin/SKILL.md for the full UMD bundle
// contract — the loader expects Mount(el, ctx) → { cleanup, update }
// and looks up the plugin by its slug-keyed global.

import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import path from 'node:path';

const slug = 'storage-ftp';

export default defineConfig({
  plugins: [react()],
  define: {
    'process.env.NODE_ENV': JSON.stringify('production'),
  },
  build: {
    target: 'es2022',
    sourcemap: false,
    minify: 'esbuild',
    cssCodeSplit: false,
    outDir: path.resolve(__dirname),
    emptyOutDir: false,
    lib: {
      entry: path.resolve(__dirname, 'src/index.tsx'),
      name: `McharborPlugin_${slug.replace(/-/g, '_')}`,
      formats: ['umd'],
      fileName: () => `${slug}.umd.cjs`,
    },
    rollupOptions: {
      external: [
        'react',
        'react-dom',
        'react-dom/client',
        'react/jsx-runtime',
        'react-i18next',
        'i18next',
      ],
      output: {
        globals: {
          react: 'React',
          'react-dom': 'ReactDOM',
          'react-dom/client': 'ReactDOMClient',
          'react/jsx-runtime': 'jsxRuntime',
          'react-i18next': 'ReactI18next',
          i18next: 'i18next',
        },
        banner: '/* mcharbor-plugin-storage-ftp bundle */',
      },
    },
  },
  // We do NOT alias `@resources` to the host's component tree — the
  // host's `@resources` path does not resolve from a plugin frontend,
  // and bundling host components would duplicate them at runtime.
  // Plugin config forms build on plain elements plus the externals
  // above. (The alias that used to sit here pointed at
  // src/plugins/src/frontend, which does not exist.)
});

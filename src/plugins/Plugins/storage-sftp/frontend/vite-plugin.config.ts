// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Vite config for the storage-sftp plugin frontend.
//
// Build emits a single UMD bundle at
//   <repo>/src/plugins/Plugins/storage-sftp/frontend/storage-sftp.umd.cjs
// with a `window.McharborPlugin_storage_sftp` global. The
// host's `loader.go` resolves that bundle by convention and
// mounts it into the storage_location config_form / setup_guide
// slot the plugin declares in plugin.yaml.
//
// Gotchas (see mcharbor-plugin-frontend-fix):
//   - global name:      McharborPlugin_<slug_with_underscores>
//   - filename:         <slug>.umd.cjs   (not .umd.js)
//   - outDir:           path.resolve(__dirname) so the bundle
//                       lands at frontend/<slug>.umd.cjs,
//                       not frontend/dist/<slug>.umd.cjs.
//
// We do NOT alias `@resources` to the host's component tree —
// the host's `@resources` path does not resolve from a plugin
// build (verified 2026-08-10). The plugin's frontend uses
// native HTML + Tailwind utilities + `@tabler/icons-react`
// only, matching the working storage-nfs plugin's approach.

import path from 'node:path';
import { fileURLToPath } from 'node:url';
import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const slug = 'storage-sftp';

const sharedReactName = 'McharborSharedReact';
const sharedI18nName = 'McharborSharedI18n';

export default defineConfig({
  plugins: [
    react({
      jsxRuntime: 'automatic',
    }),
  ],

  build: {
    outDir: path.resolve(__dirname),
    emptyOutDir: false,
    sourcemap: false,
    minify: 'esbuild',
    cssCodeSplit: false,
    lib: {
      entry: path.resolve(__dirname, 'src/index.tsx'),
      name: `McharborPlugin_storage_sftp`,
      formats: ['umd'],
      fileName: () => `${slug}.umd.cjs`,
      banner: {
        umd: `var globalThis = globalThis || self; var process = { env: {} }; var ${sharedReactName} = (typeof window !== "undefined") ? window.McharborSharedReact : (typeof globalThis !== "undefined" ? globalThis.McharborSharedReact : undefined); var ${sharedI18nName} = (typeof window !== "undefined") ? window.McharborSharedI18n : (typeof globalThis !== "undefined" ? globalThis.McharborSharedI18n : undefined);`,
      },
    },
    rollupOptions: {
      output: {
        inlineDynamicImports: true,
        globals: {
          react: sharedReactName,
          'react-dom': `${sharedReactName}Dom`,
          'react-dom/client': `${sharedReactName}DomClient`,
          'react-i18next': sharedI18nName,
          i18next: `${sharedI18nName}Core`,
        },
      },
      external: [
        'react',
        'react-dom',
        'react-dom/client',
        'react-i18next',
        'i18next',
        '@tabler/icons-react',
      ],
    },
  },
});

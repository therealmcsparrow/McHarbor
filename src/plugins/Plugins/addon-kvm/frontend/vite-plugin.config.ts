// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import path from 'path';
import fs from 'fs';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const slug = 'addon-kvm';
const repoRoot = path.resolve(__dirname, '../../../');
const hostSrc = path.resolve(repoRoot, '..', 'frontend');

if (!fs.existsSync(hostSrc)) {
  throw new Error(`vite-plugin.config.ts: host frontend not found at ${hostSrc}.`);
}

/**
 * Strips noVNC's one top-level `await`.
 *
 * `lib/util/browser.js` ends with
 *
 *     exports.supportsWebCodecsH264Decode = ... = await _checkWebCodecsH264DecodeSupport();
 *
 * and top-level await cannot be expressed in UMD, so rollup refuses the bundle
 * outright (UNSUPPORTED_FEATURE). The await is not load-bearing: the flag has a
 * `false` initialiser and its only reader is `rfb.js`, which consults it while
 * negotiating an already-open connection — long after module evaluation. So the
 * probe is rewritten to settle on its own and write the same two bindings when
 * it does. `rfb.js` reads it off the live module namespace, so it observes the
 * later value; the only behavioural difference would be a connection opened
 * within the probe's first few milliseconds falling back to the software H.264
 * path, which is the same thing that happens on every browser without
 * WebCodecs.
 *
 * Paired with the `@novnc/novnc` pin in package.json: 1.7 dropped the
 * transpiled `lib/` tree in favour of `core/` behind an `exports` map whose
 * sources are untranspiled ESM with the same await, so the pin and this
 * transform are one decision, not two.
 */
function stripNoVNCTopLevelAwait(): import('vite').Plugin {
  const marker = 'await _checkWebCodecsH264DecodeSupport()';
  return {
    name: 'mcharbor:strip-novnc-tla',
    enforce: 'pre',
    transform(code, id) {
      if (!id.includes('@novnc/novnc') || !code.includes(marker)) return null;
      return {
        code: code.replace(
          /exports\.supportsWebCodecsH264Decode = supportsWebCodecsH264Decode = await _checkWebCodecsH264DecodeSupport\(\);/,
          [
            '_checkWebCodecsH264DecodeSupport().then(function (supported) {',
            '  exports.supportsWebCodecsH264Decode = supportsWebCodecsH264Decode = supported;',
            '}, function () {',
            '  exports.supportsWebCodecsH264Decode = supportsWebCodecsH264Decode = false;',
            '});',
          ].join('\n'),
        ),
        map: null,
      };
    },
  };
}

/**
 * Gives each spice-html5 channel its own console URL.
 *
 * spice-html5 opens every channel (display, inputs, cursor, playback…) on
 * `this.ws.url`, the main channel's URL. The console gateway's tickets are
 * single-use, so a second socket on the same URL is refused. The gateway
 * issues a batch of channel tickets for one session instead, and this rewrites
 * that one expression to ask the main connection for the next channel's URL
 * (`channel_uri`, set in `spice-session.ts`), falling back to the old
 * behaviour when it is not set.
 *
 * spice-html5 is LGPL-3.0: the package on disk is unmodified, and this
 * transform is the whole of the modification, in source, next to the pin.
 * A rewrite that finds nothing fails the build, so an upstream change cannot
 * silently put every channel back on one URL.
 */
function spiceChannelUrls(): import('vite').Plugin {
  const expression = 'uri: this.ws.url,';
  return {
    name: 'mcharbor:spice-channel-urls',
    enforce: 'pre',
    transform(code, id) {
      if (!id.replace(/\\/g, '/').includes('@spice-project/spice-html5/src/main.js')) return null;
      if (!code.includes(expression)) {
        throw new Error('spice-html5 main.js no longer opens channels on this.ws.url; revisit spiceChannelUrls().');
      }
      return {
        code: code.replace(
          expression,
          "uri: typeof this.channel_uri === 'function' ? this.channel_uri() : this.ws.url,",
        ),
        map: null,
      };
    },
  };
}

export default defineConfig({
  plugins: [stripNoVNCTopLevelAwait(), spiceChannelUrls(), react()],
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
        postBanner: "/*! Includes @spice-project/spice-html5 0.2.1 (LGPL-3.0-or-later); source and licence: see THIRD_PARTY_NOTICES.md in this plugin. */\n",
        banner: "globalThis.process = { env: { NODE_ENV: 'production' }, emit: function () {} };",
        inlineDynamicImports: true,
      },
    },
  },
  server: {
    fs: { allow: [__dirname, hostSrc, repoRoot, path.resolve(repoRoot, '..')] },
  },
});

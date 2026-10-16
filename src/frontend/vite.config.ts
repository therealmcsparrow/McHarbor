// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { defineConfig } from 'vite-plus';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';
import path from 'path';
import { createHash } from 'node:crypto';
import { build as viteBuild, type Plugin as VitePlugin } from 'vite';
import type { OutputBundle, OutputChunk } from 'vite/rolldown';
import { fmt, lint } from './lint/config';

const repoRoot = path.resolve(__dirname, '../..');
const frontendNodeModules = path.resolve(__dirname, 'node_modules');
const compatShim = (itemName: string) => path.resolve(__dirname, `./core/vendor/es-toolkit-compat/${itemName}.js`);

const SHELL_STATIC = [
  '/index.html',
  '/icon_McSparrow.svg',
  '/fonts/fonts.css',
  '/fonts/inter-tight-latin.woff2',
  '/fonts/bricolage-grotesque-latin.woff2',
];

function precacheList(bundle: OutputBundle): string[] {
  const chunks = Object.values(bundle).filter((item): item is OutputChunk => item.type === 'chunk');
  const byFile = new Map(chunks.map((chunk) => [chunk.fileName, chunk]));
  const files = new Set<string>();
  const visit = (chunk: OutputChunk | undefined) => {
    if (!chunk || files.has(chunk.fileName)) return;
    files.add(chunk.fileName);
    const css = (chunk as OutputChunk & { viteMetadata?: { importedCss?: Set<string> } }).viteMetadata?.importedCss;
    css?.forEach((file) => files.add(file));
    chunk.imports.forEach((file) => visit(byFile.get(file)));
  };
  chunks
    .filter(
      (chunk) => chunk.isEntry || chunk.moduleIds.some((id) => id.replace(/\\/g, '/').includes('/modules/oncall/')),
    )
    .forEach(visit);
  return [...SHELL_STATIC, ...[...files].sort().map((file) => `/${file}`)];
}

function serviceWorker(): VitePlugin {
  return {
    name: 'mcharbor-service-worker',
    apply: 'build',
    async writeBundle(options, bundle) {
      const files = precacheList(bundle);
      const version = createHash('sha256').update(files.join('\n')).digest('hex').slice(0, 12);
      await viteBuild({
        configFile: false,
        logLevel: 'warn',
        define: { __MCHARBOR_PRECACHE__: JSON.stringify({ version, files }) },
        build: {
          outDir: options.dir,
          emptyOutDir: false,
          copyPublicDir: false,
          sourcemap: false,
          lib: {
            entry: path.resolve(__dirname, 'core/pwa/sw.ts'),
            formats: ['iife'],
            name: 'mcharborServiceWorker',
            fileName: () => 'sw.js',
          },
        },
      });
    },
  };
}

export default defineConfig({
  plugins: [react(), tailwindcss(), serviceWorker()],
  lint,
  fmt,
  resolve: {
    alias: {
      '@': path.resolve(__dirname, '.'),
      '@core': path.resolve(__dirname, './core'),
      '@resources': path.resolve(__dirname, './resources'),
      '@modules': path.resolve(__dirname, './modules'),
      '@widgets': path.resolve(repoRoot, 'widgets'),
      '@nodes': path.resolve(repoRoot, 'nodes'),
      '@tabler/icons-react': path.resolve(frontendNodeModules, '@tabler/icons-react'),
      '@tanstack/react-query': path.resolve(frontendNodeModules, '@tanstack/react-query'),
      'es-toolkit/compat/get': compatShim('get'),
      'es-toolkit/compat/isPlainObject': compatShim('isPlainObject'),
      'es-toolkit/compat/last': compatShim('last'),
      'es-toolkit/compat/maxBy': compatShim('maxBy'),
      'es-toolkit/compat/minBy': compatShim('minBy'),
      'es-toolkit/compat/omit': compatShim('omit'),
      'es-toolkit/compat/range': compatShim('range'),
      'es-toolkit/compat/sortBy': compatShim('sortBy'),
      'es-toolkit/compat/sumBy': compatShim('sumBy'),
      'es-toolkit/compat/throttle': compatShim('throttle'),
      'es-toolkit/compat/uniqBy': compatShim('uniqBy'),
      react: path.resolve(frontendNodeModules, 'react'),
      'react/jsx-runtime': path.resolve(frontendNodeModules, 'react/jsx-runtime.js'),
      'react-dom': path.resolve(frontendNodeModules, 'react-dom'),
      'react-i18next': path.resolve(frontendNodeModules, 'react-i18next'),
      'react-router': path.resolve(frontendNodeModules, 'react-router'),
      recharts: path.resolve(frontendNodeModules, 'recharts'),
      sonner: path.resolve(frontendNodeModules, 'sonner'),
      zod: path.resolve(frontendNodeModules, 'zod'),
    },
    dedupe: ['react', 'react-dom', 'react-i18next'],
  },
  server: {
    watch: {
      ignored: ['**/.results/**', '**/coverage/**', '**/playwright-report/**', '**/test-results/**'],
    },
    fs: {
      allow: [repoRoot],
    },
    port: 8173,
    proxy: {
      '/api': {
        target: process.env.MCHARBOR_API_TARGET ?? 'https://localhost:8705',
        changeOrigin: true,
        secure: false,
        ws: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    sourcemap: false,
    rolldownOptions: {
      output: {
        manualChunks(id) {
          if (!id.includes('node_modules')) {
            return undefined;
          }

          if (id.includes('/react/') || id.includes('/react-dom/') || id.includes('/react-router/')) {
            return 'react-core';
          }

          if (id.includes('@tanstack/react-query') || id.includes('zustand')) {
            return 'state-core';
          }

          if (id.includes('@codemirror/lang-yaml') || id.includes('@lezer/yaml')) {
            return 'codemirror-yaml';
          }

          if (id.includes('@codemirror/lang-javascript') || id.includes('@lezer/javascript')) {
            return 'codemirror-javascript';
          }

          if (id.includes('@codemirror/') || id.includes('@lezer/')) {
            return 'codemirror-core';
          }

          if (id.includes('@xterm/addon-fit')) {
            return 'xterm-fit';
          }

          if (id.includes('@xterm/addon-web-links')) {
            return 'xterm-links';
          }

          if (id.includes('@xterm/xterm')) {
            return 'xterm-core';
          }

          return undefined;
        },
      },
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./test/setup.ts'],
    include: ['**/*.{test,spec}.{ts,tsx}'],
    exclude: ['**/node_modules/**', 'dist/**', 'tests/**'],
    coverage: {
      provider: 'v8',
      reporter: ['text', 'html', 'lcov'],
      reportsDirectory: './coverage',
      include: [
        'core/**/*.{ts,tsx}',
        'resources/**/*.{ts,tsx}',
        'modules/**/*.{ts,tsx}',
        'widgets/**/*.{ts,tsx}',
        'nodes/**/*.{ts,tsx}',
      ],
      exclude: [
        '**/*.d.ts',
        '**/*.test.{ts,tsx}',
        '**/*.spec.{ts,tsx}',
        '**/i18n/**',
        '**/locales/**',
        'dist/**',
        'coverage/**',
        'vite.config.ts',
      ],
    },
  },
});

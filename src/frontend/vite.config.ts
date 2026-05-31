// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';
import path from 'path';

const repoRoot = path.resolve(__dirname, '../..');
const frontendNodeModules = path.resolve(__dirname, 'node_modules');

export default defineConfig({
  plugins: [react(), tailwindcss()],
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
      'react': path.resolve(frontendNodeModules, 'react'),
      'react/jsx-runtime': path.resolve(frontendNodeModules, 'react/jsx-runtime.js'),
      'react-dom': path.resolve(frontendNodeModules, 'react-dom'),
      'react-i18next': path.resolve(frontendNodeModules, 'react-i18next'),
      'react-router': path.resolve(frontendNodeModules, 'react-router'),
      'recharts': path.resolve(frontendNodeModules, 'recharts'),
      'sonner': path.resolve(frontendNodeModules, 'sonner'),
      'zod': path.resolve(frontendNodeModules, 'zod'),
    },
    dedupe: ['react', 'react-dom', 'react-i18next'],
  },
  server: {
    fs: {
      allow: [repoRoot],
    },
    port: 8173,
    proxy: {
      '/api': {
        target: 'http://localhost:5474',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    sourcemap: false,
    rollupOptions: {
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

          if (
            id.includes('@codemirror/') ||
            id.includes('@lezer/')
          ) {
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
    coverage: {
      provider: 'v8',
      reporter: ['text', 'html', 'lcov'],
      reportsDirectory: './coverage',
      include: ['core/**/*.{ts,tsx}', 'resources/**/*.{ts,tsx}', 'modules/**/*.{ts,tsx}', 'widgets/**/*.{ts,tsx}', 'nodes/**/*.{ts,tsx}'],
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

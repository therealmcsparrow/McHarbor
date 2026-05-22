// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';
import path from 'path';

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, '.'),
      '@core': path.resolve(__dirname, './core'),
      '@resources': path.resolve(__dirname, './resources'),
      '@modules': path.resolve(__dirname, './modules'),
      '@widgets': path.resolve(__dirname, './widgets'),
      '@nodes': path.resolve(__dirname, './nodes'),
    },
  },
  server: {
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
});

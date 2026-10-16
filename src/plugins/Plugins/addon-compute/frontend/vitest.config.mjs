// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const hostSrc = path.resolve(here, '../../../../frontend');
const hostModules = path.join(hostSrc, 'node_modules');
const shared = [
  'react',
  'react-dom',
  'react-i18next',
  'i18next',
  '@tabler/icons-react',
  '@tanstack/react-query',
  '@tanstack/react-table',
  '@testing-library/react',
  '@testing-library/jest-dom',
  '@testing-library/user-event',
  'vitest',
];

export default {
  resolve: {
    alias: [
      { find: '@core', replacement: path.join(hostSrc, 'core') },
      { find: '@resources', replacement: path.join(hostSrc, 'resources') },
      { find: '@modules', replacement: path.join(hostSrc, 'modules') },
      { find: /^@\/(.*)$/, replacement: path.join(here, 'src', '$1') },
      ...shared.map((name) => ({
        find: new RegExp(`^${name.replace(/[/]/g, '\/')}(\/.*)?$`),
        replacement: `${path.join(hostModules, name)}$1`,
      })),
    ],
    dedupe: shared,
  },
  server: { fs: { allow: [here, hostSrc, path.resolve(hostSrc, '..')] } },
  test: {
    root: here,
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./test-setup.ts'],
    include: ['src/**/*.test.{ts,tsx}'],
  },
};

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import '@testing-library/jest-dom/vitest';
import { cleanup } from '@testing-library/react';
import { afterAll, afterEach, beforeAll } from 'vitest';
import { initTestI18n } from './i18n';
import { resetPermissions } from './permissions';
import { server } from './server';

beforeAll(async () => {
  await initTestI18n();
  server.listen({ onUnhandledFrame: 'error' });
});

afterEach(() => {
  cleanup();
  server.resetHandlers();
  resetPermissions();
  localStorage.clear();
});

afterAll(() => {
  server.close();
});

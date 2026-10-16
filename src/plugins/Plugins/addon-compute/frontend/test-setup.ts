// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import '@testing-library/jest-dom/vitest';
import { cleanup } from '@testing-library/react';
import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import { afterEach, beforeAll } from 'vitest';
import compute from '../i18n/en/compute.json';
import common from '../../../../frontend/core/i18n/locales/en/common.json';

beforeAll(async () => {
  if (i18n.isInitialized) return;
  await i18n.use(initReactI18next).init({
    lng: 'en',
    fallbackLng: 'en',
    defaultNS: 'compute',
    ns: ['compute', 'common'],
    resources: { en: { compute, common } },
    interpolation: { escapeValue: false },
  });
});

afterEach(() => {
  cleanup();
});

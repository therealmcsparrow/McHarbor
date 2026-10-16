// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import '@testing-library/jest-dom/vitest';
import { cleanup } from '@testing-library/react';
import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import { afterEach, beforeAll } from 'vitest';
import ct from '../i18n/en/ct.json';

beforeAll(async () => {
  if (i18n.isInitialized) return;
  await i18n.use(initReactI18next).init({
    lng: 'en',
    fallbackLng: 'en',
    defaultNS: 'ct',
    ns: ['ct'],
    resources: { en: { ct } },
    interpolation: { escapeValue: false },
  });
});

afterEach(() => {
  cleanup();
});

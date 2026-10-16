// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';

const bundles = import.meta.glob<Record<string, unknown>>('../core/i18n/locales/en/*.json', {
  import: 'default',
  eager: true,
});

const resources: Record<string, Record<string, unknown>> = {};
for (const [path, bundle] of Object.entries(bundles)) {
  const ns = path
    .split('/')
    .pop()
    ?.replace(/\.json$/, '');
  if (ns) resources[ns] = bundle;
}

export async function initTestI18n(): Promise<void> {
  if (i18n.isInitialized) return;
  await i18n.use(initReactI18next).init({
    lng: 'en',
    fallbackLng: 'en',
    defaultNS: 'common',
    ns: Object.keys(resources),
    resources: { en: resources },
    interpolation: { escapeValue: false },
    returnNull: false,
  });
}

export { i18n };

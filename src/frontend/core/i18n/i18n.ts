// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import BrowserLanguageDetector from 'i18next-browser-languagedetector';
import '@core/plugins/hostBridge';
import { buildNodeTranslations } from '@nodes/i18n-loader';
import { buildWidgetTranslations } from '../../modules/dashboard/widgets/i18n-loader';

import {
  detectInitialLanguage,
  getCachedDefaultLanguage,
  languageLabels,
  supportedLanguages,
  type SupportedLanguage,
} from './languages';

export { getCachedDefaultLanguage, languageLabels, supportedLanguages };
export type { SupportedLanguage };

const localeModules = import.meta.glob<Record<string, unknown>>('./locales/*/*.json', { import: 'default' });

const staticNamespaces = [
  'common',
  'auth',
  'containers',
  'images',
  'volumes',
  'networks',
  'stacks',
  'secrets',
  'environments',
  'settings',
  'dashboard',
  'terminal',
  'security',
  'docker',
  'host',
  'gitops',
  'abac',
  'ai',
  'plugins',
  'enterprise',
  'scim',
  'license',
  'database',
  'oncall',
  'reconciler',
  'instancebackup',
  'incidents',
  'support',
  'compliance',
  'compliancepacks',
  'envref',
  'audit',
  'access',
  'runbooks',
  'demo',
  'workflows',
] as const;

type StaticNamespace = (typeof staticNamespaces)[number];

/** Deep-merge source into target, mutating target in place. */
function deepMerge(target: Record<string, unknown>, source: Record<string, unknown>): Record<string, unknown> {
  for (const key of Object.keys(source)) {
    const sv = source[key];
    const tv = target[key];
    if (sv && typeof sv === 'object' && !Array.isArray(sv) && tv && typeof tv === 'object' && !Array.isArray(tv)) {
      deepMerge(tv as Record<string, unknown>, sv as Record<string, unknown>);
    } else {
      target[key] = sv;
    }
  }
  return target;
}

const loadedLanguages = new Set<SupportedLanguage>();

/**
 * Returns the language code currently active in i18next, normalized
 * to the short BCP-47 form ('en', 'nl', 'de', …). Falls back to 'en'
 * when the active language isn't in the supported set (e.g., browser
 * served a locale we don't ship translations for). Useful for plugin
 * runtime code that needs to know which locale to load from the
 * host's i18n bundle.
 */
export function resolveCurrentLanguage(): SupportedLanguage {
  const raw = (i18n.language ?? '').toLowerCase();
  const short = raw.substring(0, 2);
  return supportedLanguages.includes(short as SupportedLanguage) ? (short as SupportedLanguage) : 'en';
}

async function loadStaticNamespace(
  lang: SupportedLanguage,
  namespace: StaticNamespace,
): Promise<Record<string, unknown>> {
  const loader = localeModules[`./locales/${lang}/${namespace}.json`];
  if (!loader) {
    return {};
  }

  const resource = await loader();
  return (resource ?? {}) as Record<string, unknown>;
}

async function buildResourcesForLanguage(lang: SupportedLanguage) {
  const namespaceEntries = await Promise.all(
    staticNamespaces.map(async (namespace) => {
      const resource = await loadStaticNamespace(lang, namespace);
      return [namespace, resource] as const;
    }),
  );

  const resources = Object.fromEntries(namespaceEntries) as Record<StaticNamespace, Record<string, unknown>>;
  const widgetTranslations = await buildWidgetTranslations(lang);
  const nodeTranslations = await buildNodeTranslations(lang);

  return {
    ...resources,
    dashboard: deepMerge({ ...resources.dashboard }, widgetTranslations),
    nodes: nodeTranslations,
  };
}

export async function loadLanguageResources(lang: SupportedLanguage) {
  if (loadedLanguages.has(lang)) {
    return;
  }

  const resources = await buildResourcesForLanguage(lang);
  for (const [namespace, resource] of Object.entries(resources)) {
    i18n.addResourceBundle(lang, namespace, resource, true, true);
  }

  loadedLanguages.add(lang);
}

export async function changeAppLanguage(lang: SupportedLanguage) {
  await loadLanguageResources(lang);
  await i18n.changeLanguage(lang);
  document.documentElement.lang = lang;
}

export async function initializeI18n() {
  if (i18n.isInitialized) {
    return;
  }

  const initialLang = await detectInitialLanguage();

  await i18n
    .use(BrowserLanguageDetector)
    .use(initReactI18next)
    .init({
      lng: initialLang,
      fallbackLng: 'en',
      defaultNS: 'common',
      supportedLngs: supportedLanguages,
      interpolation: {
        escapeValue: false,
      },
      detection: {
        order: ['localStorage', 'navigator'],
        lookupLocalStorage: 'mcharbor-i18next-lang',
        caches: ['localStorage'],
      },
    });

  await loadLanguageResources('en');
  if (initialLang !== 'en') {
    await loadLanguageResources(initialLang);
  }

  await i18n.changeLanguage(initialLang);
  document.documentElement.lang = initialLang;

  publishHostI18n();
}

/**
 * Publishes the initialized i18next instance on `window.McharborHost`.
 *
 * Plugin UMD bundles compile their own copy of `react-i18next` (the
 * host ships an ES module, so it cannot hand them the library as a UMD
 * global). That private copy never saw `initReactI18next`, so a plain
 * `useTranslation(ns)` inside a plugin has no instance to resolve
 * against and echoes the key back. Exposing the host instance lets a
 * plugin wrap its tree in its own `<I18nextProvider i18n={…}>` and
 * resolve against the same store the loader merges plugin namespaces
 * into (see core/plugins/pluginI18n.ts).
 */
function publishHostI18n(): void {
  window.McharborHost = { ...window.McharborHost, i18n };
}

export default i18n;

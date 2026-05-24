// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import LanguageDetector from 'i18next-browser-languagedetector';
import { buildNodeTranslations } from '../../nodes/i18n-loader';
import { buildWidgetTranslations } from '../../modules/dashboard/widgets/i18n-loader';

const localeModules = import.meta.glob<Record<string, unknown>>('./locales/*/*.json', { import: 'default' });

const staticNamespaces = [
  'common',
  'auth',
  'containers',
  'images',
  'volumes',
  'networks',
  'stacks',
  'environments',
  'settings',
  'dashboard',
  'kubernetes',
  'terminal',
  'security',
  'docker',
] as const;

type StaticNamespace = (typeof staticNamespaces)[number];

/** Deep-merge source into target, mutating target in place. */
function deepMerge(
  target: Record<string, unknown>,
  source: Record<string, unknown>,
): Record<string, unknown> {
  for (const key of Object.keys(source)) {
    const sv = source[key];
    const tv = target[key];
    if (
      sv && typeof sv === 'object' && !Array.isArray(sv) &&
      tv && typeof tv === 'object' && !Array.isArray(tv)
    ) {
      deepMerge(tv as Record<string, unknown>, sv as Record<string, unknown>);
    } else {
      target[key] = sv;
    }
  }
  return target;
}

export const supportedLanguages = ['en', 'nl', 'de', 'es', 'fr', 'pt', 'zh'] as const;
export type SupportedLanguage = (typeof supportedLanguages)[number];

export const languageLabels: Record<SupportedLanguage, string> = {
  en: 'English',
  nl: 'Nederlands',
  de: 'Deutsch',
  es: 'Español',
  fr: 'Français',
  pt: 'Português',
  zh: '中文',
};

const loadedLanguages = new Set<SupportedLanguage>();

function normalizeLanguage(value: string | null | undefined): SupportedLanguage {
  const short = value?.substring(0, 2).toLowerCase();
  return supportedLanguages.includes(short as SupportedLanguage) ? (short as SupportedLanguage) : 'en';
}

function detectInitialLanguage(): SupportedLanguage {
  try {
    const stored = window.localStorage.getItem('mcharbor-i18next-lang');
    if (stored) {
      return normalizeLanguage(stored);
    }
  } catch {
    // Ignore localStorage access failures.
  }

  try {
    return normalizeLanguage(window.navigator.language);
  } catch {
    return 'en';
  }
}

async function loadStaticNamespace(lang: SupportedLanguage, namespace: StaticNamespace): Promise<Record<string, unknown>> {
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

  const initialLang = detectInitialLanguage();

  await i18n
    .use(LanguageDetector)
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
}

export default i18n;

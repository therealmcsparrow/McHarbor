// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/**
 * Auto-loads all widget i18n JSON files via Vite's import.meta.glob.
 * Each widget folder has i18n/{en,nl,de}.json — this builds a per-language
 * resource map that gets deep-merged into the "dashboard" i18next namespace.
 *
 * Path pattern: /widgets/<widget-folder>/i18n/<lang>.json
 */
const widgetI18nModules = import.meta.glob<Record<string, unknown>>('@widgets/*/i18n/*.json', { import: 'default' });

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

export async function buildWidgetTranslations(lang: string): Promise<Record<string, unknown>> {
  const result: Record<string, unknown> = {};

  for (const [path, loadTranslations] of Object.entries(widgetI18nModules)) {
    const fileLang = path.match(/[\\/]i18n[\\/]([^\\/]+)\.json$/)?.[1];
    if (!fileLang || fileLang !== lang) {
      continue;
    }

    const translations = await loadTranslations();
    deepMerge(result, translations as Record<string, unknown>);
  }

  return result;
}

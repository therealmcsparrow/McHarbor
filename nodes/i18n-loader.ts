// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/**
 * Auto-loads workflow node i18n JSON files via Vite's import.meta.glob.
 * Path pattern: /nodes/<node-folder>/i18n/<lang>.json
 */
const nodeI18nModules = import.meta.glob<Record<string, unknown>>('@nodes/*/i18n/*.json', { import: 'default' });

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

export async function buildNodeTranslations(lang: string): Promise<Record<string, unknown>> {
  const result: Record<string, unknown> = {};

  for (const [path, loadTranslations] of Object.entries(nodeI18nModules)) {
    const match = path.match(/[\\/]([^\\/]+)[\\/]i18n[\\/]([^\\/]+)\.json$/);
    const nodeKey = match?.[1];
    const fileLang = match?.[2];
    if (!nodeKey || !fileLang || fileLang !== lang) {
      continue;
    }

    const translations = await loadTranslations();
    const existing = result[nodeKey];
    const target = existing && typeof existing === 'object' ? (existing as Record<string, unknown>) : {};
    result[nodeKey] = deepMerge(target, translations as Record<string, unknown>);
  }

  return result;
}

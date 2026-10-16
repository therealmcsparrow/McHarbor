// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { create } from 'zustand';
import type { WidgetDefinition } from '../widgets/registry';
import { DOWNLOADED_PREFIX, toDownloadedDefinition } from '../widgets/downloaded-loader';

type WidgetRegistryState = {
  definitions: WidgetDefinition[];
  definitionMap: Record<string, WidgetDefinition>;
  setDefinitions: (defs: WidgetDefinition[]) => void;
  mergeDownloaded: (downloaded: WidgetDefinitionWithI18n[]) => void;
};

/** API response shape for downloaded widgets — definition metadata + translations. */
export type WidgetDefinitionFromAPI = {
  key: string;
  label: string;
  category: string;
  description: string;
  icon: string;
  source: string;
  component: string;
  enabled: boolean;
  defaultSize: { w: number; h: number };
  minSize: { w: number; h: number };
  version?: string;
  verified?: boolean;
};

export type WidgetDefinitionWithI18n = WidgetDefinitionFromAPI & {
  translations?: Record<string, Record<string, unknown>>;
};

// One definition per installed version, so a re-sync keeps the same lazy
// component and placed tiles do not remount.
const downloadedCache = new Map<string, WidgetDefinition | null>();

function cachedDownloadedDefinition(dl: WidgetDefinitionWithI18n): WidgetDefinition | null {
  const cacheKey = `${dl.key}@${dl.version ?? ''}:${dl.verified === true}`;
  if (!downloadedCache.has(cacheKey)) downloadedCache.set(cacheKey, toDownloadedDefinition(dl));
  return downloadedCache.get(cacheKey) ?? null;
}

export const useWidgetRegistryStore = create<WidgetRegistryState>()((set, get) => ({
  definitions: [],
  definitionMap: {},

  setDefinitions: (defs) => {
    const definitionMap: Record<string, WidgetDefinition> = {};
    for (const d of defs) {
      definitionMap[d.id] = d;
    }
    set({ definitions: defs, definitionMap });
  },

  mergeDownloaded: (downloaded) => {
    const incoming: WidgetDefinition[] = [];
    for (const dl of downloaded) {
      const def = cachedDownloadedDefinition(dl);
      if (def) incoming.push(def);
    }
    // Replace the whole downloaded set so a removed or no-longer-verified
    // widget drops out; built-in and plugin definitions are left untouched.
    const kept = get().definitions.filter((d) => !String(d.id).startsWith(DOWNLOADED_PREFIX));
    const definitions = [...kept, ...incoming];
    const definitionMap: Record<string, WidgetDefinition> = {};
    for (const d of definitions) definitionMap[d.id] = d;
    set({ definitions, definitionMap });

    // Inject i18n translations for downloaded widgets
    if (typeof window !== 'undefined') {
      void import('@core/i18n/i18n').then(({ default: i18n }) => {
        for (const dl of downloaded) {
          // A catalog entry without an `en` bundle still names itself.
          const translations =
            dl.source === 'downloaded' && !dl.translations?.en
              ? { ...dl.translations, en: { label: dl.label, description: dl.description } }
              : dl.translations;
          if (!translations) continue;
          for (const [lang, trans] of Object.entries(translations)) {
            i18n.addResourceBundle(lang, 'dashboard', { widgets: { [dl.key]: trans } }, true, false);
          }
        }
      });
    }
  },
}));

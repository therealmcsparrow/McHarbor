// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { create } from 'zustand';
import type { WidgetDefinition, WidgetTypeId } from '../widgets/registry';

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
};

export type WidgetDefinitionWithI18n = WidgetDefinitionFromAPI & {
  translations?: Record<string, Record<string, unknown>>;
};

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
    const { definitions, definitionMap } = get();
    const existingKeys = new Set(definitions.map((d) => d.id));
    const newDefs: WidgetDefinition[] = [];

    for (const dl of downloaded) {
      if (existingKeys.has(dl.key as WidgetTypeId)) continue;

      // Stub: downloaded widget components are not yet supported.
      // When the store supports downloadable widget components, map
      // dl.component to an actual React component here.
    }

    if (newDefs.length > 0) {
      const merged = [...definitions, ...newDefs];
      const mergedMap = { ...definitionMap };
      for (const d of newDefs) {
        mergedMap[d.id] = d;
      }
      set({ definitions: merged, definitionMap: mergedMap });
    }

    // Inject i18n translations for downloaded widgets
    if (typeof window !== 'undefined') {
      import('@core/i18n/i18n').then(({ default: i18n }) => {
        for (const dl of downloaded) {
          if (!dl.translations) continue;
          for (const [lang, trans] of Object.entries(dl.translations)) {
            i18n.addResourceBundle(lang, 'dashboard', { widgets: { [dl.key]: trans } }, true, false);
          }
        }
      });
    }
  },
}));

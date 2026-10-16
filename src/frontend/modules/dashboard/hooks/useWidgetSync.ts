// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect } from 'react';
import { useWidgetCatalogDefinitions, type WidgetCatalogDefinition } from '@resources/hooks/useWidgetCatalog';
import { BUILTIN_WIDGET_DEFINITIONS } from '../widgets/catalog';
import { useWidgetRegistryStore } from '../stores/widget-registry';
import { useDashboardLayoutStore } from '../stores/dashboard-layout';
import { getWidgetDefinitionMap } from '../widgets/registry';

/**
 * Fetches widget definitions from the backend API and filters the dashboard
 * registry down to currently enabled built-in, plugin and downloaded widgets.
 */
export function useWidgetSync() {
  const setDefinitions = useWidgetRegistryStore((s) => s.setDefinitions);
  const mergeDownloaded = useWidgetRegistryStore((s) => s.mergeDownloaded);
  const pruneUnavailable = useDashboardLayoutStore((s) => s.pruneUnavailable);
  const query = useWidgetCatalogDefinitions();

  useEffect(() => {
    if (!query.data) return;
    const definitionMap = getWidgetDefinitionMap();

    const enabledLookup = new Map(query.data.map((definition) => [definition.key, definition.enabled !== false]));

    const enabledBuiltins = BUILTIN_WIDGET_DEFINITIONS.filter(
      (definition) => enabledLookup.get(definition.id) !== false,
    );

    const enabledDefinitions = enabledBuiltins.flatMap((definition) => {
      const runtimeDefinition = definitionMap[definition.id];
      if (!runtimeDefinition) {
        return [];
      }

      return [
        {
          ...definition,
          component: runtimeDefinition.component,
        },
      ];
    });

    // Plugin widgets are registered by PluginWidgetBridge, not by this
    // catalog; rebuilding from the built-in list alone would drop them and
    // then prune every placed instance. Their availability override still
    // comes from the catalog, so a widget disabled in the Store drops out
    // here too.
    const pluginDefinitions = useWidgetRegistryStore
      .getState()
      .definitions.filter(
        (definition) =>
          String(definition.id).startsWith('plugin:') && enabledLookup.get(String(definition.id)) !== false,
      );
    const nextDefinitions = [...enabledDefinitions, ...pluginDefinitions];
    setDefinitions(nextDefinitions);

    // Downloaded widgets come from the same catalog response; the store only
    // registers the ones the server reports verified.
    mergeDownloaded(
      query.data.filter((definition) => definition.source === 'downloaded' && definition.enabled !== false),
    );
    pruneUnavailable(new Set(useWidgetRegistryStore.getState().definitions.map((definition) => definition.id)));
  }, [query.data, pruneUnavailable, setDefinitions, mergeDownloaded]);

  return query;
}

export type WidgetDefinitionWithI18n = WidgetCatalogDefinition;

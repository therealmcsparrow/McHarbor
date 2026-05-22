// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect } from 'react';
import {
  useWidgetCatalogDefinitions,
  type WidgetCatalogDefinition,
} from '@resources/hooks/useWidgetCatalog';
import { BUILTIN_WIDGET_DEFINITIONS } from '../widgets/catalog';
import { useWidgetRegistryStore } from '../stores/widget-registry';
import { useDashboardLayoutStore } from '../stores/dashboard-layout';
import { getWidgetDefinitionMap } from '../widgets/registry';

/**
 * Fetches widget definitions from the backend API and filters the dashboard
 * registry down to currently enabled built-in widgets.
 */
export function useWidgetSync() {
  const setDefinitions = useWidgetRegistryStore((s) => s.setDefinitions);
  const pruneUnavailable = useDashboardLayoutStore((s) => s.pruneUnavailable);
  const query = useWidgetCatalogDefinitions();

  useEffect(() => {
    if (!query.data) return;
    const definitionMap = getWidgetDefinitionMap();

    const enabledLookup = new Map(
      query.data.map((definition) => [definition.key, definition.enabled !== false]),
    );

    const enabledBuiltins = BUILTIN_WIDGET_DEFINITIONS.filter(
      (definition) => enabledLookup.get(definition.id) !== false,
    );

    const enabledDefinitions = enabledBuiltins.flatMap((definition) => {
      const runtimeDefinition = definitionMap[definition.id];
      if (!runtimeDefinition) {
        return [];
      }

      return [{
        ...definition,
        component: runtimeDefinition.component,
      }];
    });

    setDefinitions(enabledDefinitions);
    pruneUnavailable(new Set(enabledDefinitions.map((definition) => definition.id)));
  }, [query.data, pruneUnavailable, setDefinitions]);

  return query;
}

export type WidgetDefinitionWithI18n = WidgetCatalogDefinition;

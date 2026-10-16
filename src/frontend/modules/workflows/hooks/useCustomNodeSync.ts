// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect } from 'react';
import { useQuery } from '@tanstack/react-query';
import { usePermission } from '@core/auth/usePermission';
import { PERM } from '@core/auth/permissionKeys';
import { api } from '@core/api/client';
import i18n from '@core/i18n/i18n';
import { useCustomNodeRegistry } from '../stores/custom-node-registry';
import type { NodeDefinition } from '../types';

export type CustomNodeWithCode = {
  key: string;
  label: string;
  category: 'trigger' | 'action' | 'logic' | 'utility' | 'integration';
  description: string;
  icon: string;
  source: 'custom';
  configSchema: NodeDefinition['configSchema'];
  inputPorts: string[];
  outputPorts: string[];
  code: string;
  translations: Record<string, Record<string, unknown>>;
};

/**
 * Fetches custom node definitions from the backend API and merges them
 * into the workflow node registry so they appear in the NodePalette
 * alongside built-in nodes.
 */
export function useCustomNodeSync() {
  const setCustomNodes = useCustomNodeRegistry((s) => s.setCustomNodes);
  // custom_nodes.view gates the list route; the palette shows built-ins only
  // without it, rather than logging a 403 on every canvas open.
  const allowed = usePermission(PERM.CUSTOM_NODES_VIEW);

  const query = useQuery({
    enabled: allowed,
    queryKey: ['custom-nodes'],
    queryFn: () => api.get<CustomNodeWithCode[]>('/custom-nodes').then((r) => r.data ?? []),
    staleTime: 5 * 60_000,
  });

  useEffect(() => {
    if (!query.data) return;

    // Inject translations into i18next 'nodes' namespace
    for (const node of query.data) {
      if (node.translations) {
        for (const [lang, trans] of Object.entries(node.translations)) {
          i18n.addResourceBundle(lang, 'nodes', { [node.key]: trans }, true, true);
        }
      }
    }

    // Convert API response to NodeDefinition[] and store in registry
    const definitions: NodeDefinition[] = query.data.map((node) => ({
      key: node.key,
      label: node.label,
      category: node.category,
      description: node.description,
      icon: node.icon,
      configSchema: node.configSchema,
      inputPorts: node.inputPorts,
      outputPorts: node.outputPorts,
    }));

    // Plugin-contributed nodes share this registry (usePluginNodes)
    // and are keyed `plugin:<slug>:<key>`; keep them when the custom
    // list is replaced, or a palette mount wipes them off the canvas
    // picker until the plugin bridge notices and re-registers.
    const pluginNodes = useCustomNodeRegistry.getState().customNodes.filter((n) => n.key.startsWith('plugin:'));
    setCustomNodes([...definitions, ...pluginNodes]);
  }, [query.data, setCustomNodes]);

  return query;
}

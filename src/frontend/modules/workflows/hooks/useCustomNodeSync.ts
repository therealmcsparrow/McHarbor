// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect } from 'react';
import { useQuery } from '@tanstack/react-query';
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

  const query = useQuery({
    queryKey: ['custom-nodes'],
    queryFn: () =>
      api.get<CustomNodeWithCode[]>('/custom-nodes').then((r) => r.data ?? []),
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

    setCustomNodes(definitions);
  }, [query.data, setCustomNodes]);

  return query;
}

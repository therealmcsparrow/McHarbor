// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { usePluginLoader } from '@core/plugins/loader';
import { useCustomNodeRegistry } from '../stores/custom-node-registry';
import { findNodeDefinition } from '../nodes/resolve';
import type { NodeDefinition } from '../types';

/**
 * useNodeDefinition is the reactive form for components: it re-renders
 * when the custom-node registry changes, so a plugin node dropped
 * before its bundle finished loading picks up its definition.
 *
 * The non-reactive twin, resolveNodeDefinition, stays in nodes/resolve.ts —
 * it is callable outside React and is not a hook.
 */
export function useNodeDefinition(action: string | undefined): NodeDefinition | undefined {
  const customNodes = useCustomNodeRegistry((s) => s.customNodes);
  const capabilities = usePluginLoader((s) => s.capabilities);
  const installed = usePluginLoader((s) => s.installed);
  return useMemo(() => {
    if (!action) return undefined;
    return findNodeDefinition(action, customNodes, capabilities, installed);
  }, [action, customNodes, capabilities, installed]);
}

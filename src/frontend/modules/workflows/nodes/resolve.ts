// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { NODE_DEFINITION_MAP } from '@resources/nodes/registry';
import { usePluginLoader, type PluginCapability, type PluginStatus } from '@core/plugins/loader';
import { CHANNEL_ALIAS_NODE, resolveChannelAlias, resolveNodeAliasAction } from '@core/plugins/capabilities';
import { useCustomNodeRegistry } from '../stores/custom-node-registry';
import type { NodeDefinition } from '../types';

/**
 * resolveNodeDefinition finds a node definition by its canvas action,
 * looking at the built-in catalogue first and then at the custom-node
 * registry, which holds goja custom nodes and plugin-contributed
 * nodes (`plugin:<slug>:<key>`). Safe to call outside React (drop
 * handlers, geometry helpers): it reads the store synchronously.
 *
 * The reactive twin for components is useNodeDefinition in
 * hooks/useNodeDefinition.ts.
 */
export function resolveNodeDefinition(action: string): NodeDefinition | undefined {
  const { capabilities, installed } = usePluginLoader.getState();
  return findNodeDefinition(action, useCustomNodeRegistry.getState().customNodes, capabilities, installed);
}

/**
 * The lookup both resolvers share: built-in first, then the custom-node registry, then
 * a plugin's claim on a bare action a canvas saved before the node moved into it: its own
 * node, or for a comm plugin the generic send node on the channel type it owns.
 */
export function findNodeDefinition(
  action: string,
  customNodes: NodeDefinition[],
  capabilities: PluginCapability[],
  installed: PluginStatus[],
): NodeDefinition | undefined {
  const builtin = NODE_DEFINITION_MAP[action];
  if (builtin) return builtin;
  const custom = customNodes.find((d) => d.key === action);
  if (custom) return custom;
  const claimed = resolveNodeAliasAction(action, capabilities, installed);
  if (claimed) return customNodes.find((d) => d.key === claimed);
  return resolveChannelAlias(action, capabilities, installed) ? NODE_DEFINITION_MAP[CHANNEL_ALIAS_NODE] : undefined;
}

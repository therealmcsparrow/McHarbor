// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, type ComponentType } from 'react';
import { usePluginLoader } from '@core/plugins/loader';
import { useCustomNodeRegistry } from '../stores/custom-node-registry';
import type { NodeDefinition } from '../types';
import { loadPluginComponent } from '@core/plugins/pluginComponent';

/**
 * usePluginNodes bridges the plugin runtime loader into the workflow
 * canvas's custom-node registry. Each plugin can contribute a node
 * by listing it in its manifest's `contributes.nodes` array; the
 * loader exposes the metadata via `/api/plugins/loader/nodes`.
 *
 * For each descriptor we:
 *   1. resolve the React component by lazy-loading the plugin's
 *      UMD bundle and reading `nodes.<key>` off the bundle global;
 *   2. attach the component to the NodeDefinition so the canvas
 *      can render it directly;
 *   3. register the entry under `plugin:<slug>:<key>` so the
 *      canvas's node picker surfaces it alongside built-ins.
 *
 * Plugins that don't export a named component are silently skipped
 * — the canvas still renders with the built-in catalogue.
 *
 * See dependency-host-plugin.md §6.2.
 */
export function usePluginNodes() {
  const nodes = usePluginLoader((s) => s.nodes);
  const customNodes = useCustomNodeRegistry((s) => s.customNodes);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      const built: NodeDefinition[] = [];
      for (const n of nodes) {
        const key = `plugin:${n.plugin_slug}:${n.key}`;
        let component: ComponentType<unknown> | undefined;
        try {
          const c = await loadPluginComponent(n.plugin_slug, 'nodes', n.key);
          if (!cancelled) component = (c as ComponentType<unknown>) ?? undefined;
        } catch {
          // Bundle load failure — fall through with no component;
          // the canvas will render its default body for the node.
        }
        built.push({
          key,
          label: n.display_name || key,
          category: n.category || 'plugin',
          description: `Plugin node from ${n.plugin_slug}`,
          icon: n.icon || 'puzzle',
          configSchema: n.config_schema ?? [],
          inputPorts: n.input_ports ?? [],
          outputPorts: n.output_ports ?? [],
          permission: n.requires_permission || undefined,
          component,
        });
      }
      if (cancelled) return;
      // Compare only the plugin-owned entries: the goja custom nodes
      // live in the same registry (useCustomNodeSync) and must not
      // count as a difference, or the two bridges re-register each
      // other forever.
      const existing = new Set(customNodes.filter((n) => n.key.startsWith('plugin:')).map((n) => n.key));
      const incoming = new Set(built.map((n) => n.key));
      const same = existing.size === incoming.size && [...existing].every((id) => incoming.has(id));
      if (!same) {
        const nonPlugin = customNodes.filter((n) => !n.key.startsWith('plugin:'));
        useCustomNodeRegistry.getState().setCustomNodes([...nonPlugin, ...built]);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [nodes, customNodes]);
}

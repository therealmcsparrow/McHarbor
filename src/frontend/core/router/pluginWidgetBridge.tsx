// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, type ComponentType } from 'react';
import i18n from '@core/i18n/i18n';
import { toastError } from '@resources/utils/toast';
import { useWidgetRegistryStore } from '@modules/dashboard/stores/widget-registry';
import type { WidgetDefinition } from '@modules/dashboard/widgets/registry';
import type { WidgetProps } from '@modules/dashboard/widgets/catalog';
import { resolvePluginIcon } from '@modules/appstore/components/plugin-icons';
import { usePluginWidgets, type PluginWidget } from '@core/plugins/loader';
import { loadPluginComponent } from '@core/plugins/pluginComponent';
import { parsePluginWidgetSize } from '@core/plugins/bridge-widgets';
import { createPluginWidgetComponent, isPluginWidgetEntry } from '@core/plugins/PluginWidgetMount';

/**
 * PluginWidgetBridge — subscribes to the plugin loader's widget
 * list and registers each entry as a fully-fledged
 * WidgetDefinition in the host's widget-registry store.
 *
 * Each plugin widget descriptor from `/api/plugins/loader/widgets`
 * becomes a WidgetDefinition whose `id` is `plugin:<slug>:<key>`.
 * The plugin's UMD bundle exports `widgets[key]` as a mount entry
 * (`{ mount, unmount?, update? }`) which `createPluginWidgetComponent`
 * wraps in a host component; a bare React component export is
 * accepted for backwards compatibility but only works for hook-free
 * components, since the bundle carries its own React.
 *
 * Label, description and icon come from the descriptor: `title` and
 * `settings.description` are i18n keys in `ns:path` form (a plain
 * string still displays, i18next echoes an unknown key), `settings.icon`
 * is a Tabler component name resolved the same way sidebar entries are.
 *
 * A plugin that does not export the named entry is skipped quietly, and one
 * that fails to load its bundle is skipped with a toast — the dashboard stays
 * functional with the built-in widgets only.
 *
 * See dependency-host-plugin.md §6.2.
 */
/** Widget ids already registered (or in flight) — survives effect re-runs. */
const registered = new Set<string>();

function register(id: string, meta: PluginWidget, component: ComponentType<WidgetProps>): void {
  // New arrays, not in-place pushes: Zustand compares by reference, and a
  // store set with the same array leaves the picker and grid unrendered.
  const state = useWidgetRegistryStore.getState();
  if (state.definitionMap[id]) return;
  const def = toDefinition(id, meta, component);
  useWidgetRegistryStore.setState({
    definitions: [...state.definitions, def],
    definitionMap: { ...state.definitionMap, [id]: def },
  });
}

export function PluginWidgetBridge(): null {
  const widgets = usePluginWidgets();

  useEffect(() => {
    if (widgets.length === 0) return;
    for (const w of widgets) {
      const id = `plugin:${w.plugin_slug}:${w.key}`;
      if (registered.has(id)) continue;
      registered.add(id);
      // Each widget registers the moment its bundle resolves. A single
      // batched registration used to be abandoned whenever this effect
      // re-ran mid-load, which dropped whole plugins from the picker.
      loadPluginComponent(w.plugin_slug, 'widgets', w.key)
        .then((exported) => {
          if (!exported) {
            registered.delete(id);
            console.warn(`plugin ${w.plugin_slug} exports no widgets.${w.key}; skipping`);
            return;
          }
          const component = isPluginWidgetEntry(exported)
            ? createPluginWidgetComponent(w.plugin_slug, w.key, exported)
            : (exported as ComponentType<WidgetProps>);
          register(id, w, component);
        })
        .catch((err: unknown) => {
          registered.delete(id);
          console.error(`plugin ${w.plugin_slug} widget ${w.key} failed to load`, err);
          toastError(err, i18n.t('errors.widget_load_failed', { ns: 'plugins', slug: w.plugin_slug, widget: w.key }));
        });
    }
  }, [widgets]);

  return null;
}

function toDefinition(id: string, meta: PluginWidget, component: ComponentType<WidgetProps>): WidgetDefinition {
  const settings = meta.settings ?? {};
  const defaultSize = parsePluginWidgetSize(meta.default_size, { w: 4, h: 3 });
  const minSize = {
    w: Math.min(defaultSize.w, Math.max(1, settings.min_width ?? 2)),
    h: Math.min(defaultSize.h, Math.max(1, settings.min_height ?? 2)),
  };
  return {
    id: id as WidgetDefinition['id'],
    labelKey: meta.title,
    descriptionKey: settings.description ?? meta.title,
    category: 'plugin',
    icon: resolvePluginIcon(settings.icon),
    defaultSize,
    minSize,
    component,
  };
}

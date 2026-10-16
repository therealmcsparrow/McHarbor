// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { usePluginInstalled } from '@core/plugins/loader';
import { useAIProviderAdapters } from './useAIProviderAdapters';

// ProviderType is whatever the runtime adapter catalog reports
// (e.g. "openai", "anthropic", "ollama", "minimax", "mistral").
// The picker no longer carries a hardcoded type list — it derives
// the union of (a) loaded adapters and (b) installed-but-not-loaded
// AI plugins so newly added plugins appear automatically.
export type ProviderType = string;

export type PickerEntry = {
  type: ProviderType;
  /** Display name shown on the picker tile. Falls back to the
   *  ProviderType if the spec didn't carry a DisplayName. */
  displayName: string;
  /** Slug of the plugin that owns this ProviderType (used for the
   *  disabled-locked tile copy + the "Open Plugins" deep-link). */
  slug: string;
  /** Asset filename of the plugin's brand icon, resolved through
   *  /api/plugins/loader/icon. Every plugin ships a single
   *  "plugin.svg", which is what the disabled sweep assumes for
   *  entries recovered from installedPlugins (no spec to read). */
  icon: string;
  /** Plugin installed on disk (whether loaded or not). */
  installed: boolean;
  /** Adapter spec is registered with the runtime (plugin loaded,
   *  enabled, and contributed an AIAdapterSpec). */
  enabled: boolean;
};

/** Derive a friendly display name from a plugin slug for tiles that
 *  have no loaded spec to read DisplayName from. Strips the
 *  provider-family prefix and title-cases the remainder. */
function displayNameFromSlug(slug: string): string {
  const stripped = slug.replace(/^ai-provider-/, '');
  if (!stripped) return slug;
  return stripped.charAt(0).toUpperCase() + stripped.slice(1);
}

/**
 * 3-bucket picker driven by the runtime adapter catalog plus the
 * installed-plugin surface.
 *
 *   enabled  → adapter registered with the runtime. The plugin
 *              is loaded and contributed an AIAdapterSpec; the
 *              admin can immediately create a connection.
 *   disabled → an AI plugin (slug starts with `ai-provider-`) is
 *              installed on disk but its adapter is NOT registered.
 *              Most often the plugin is disabled in
 *              /plugins/status; route the admin there.
 *   hidden   → no plugin of this family is installed at all; the
 *              admin needs to install one from the App Store.
 *
 * Walks the adapter catalog first so newly cataloged plugins appear
 * automatically, then sweeps installedPlugins for any AI plugin that
 * didn't make it into the adapter registry.
 */
export function useAIProviderPicker() {
  const adaptersQuery = useAIProviderAdapters();
  const aiAdapters = adaptersQuery.data;
  const installedPlugins = usePluginInstalled();

  const pickerEntries = useMemo<PickerEntry[]>(() => {
    const out: PickerEntry[] = [];
    const seen = new Set<string>();

    if (aiAdapters) {
      for (const [type, spec] of Object.entries(aiAdapters)) {
        seen.add(type);
        out.push({
          type,
          displayName: spec.DisplayName || type,
          slug: spec.Slug || '',
          icon: spec.Icon ?? '',
          installed: true,
          enabled: true,
        });
      }
    }

    if (installedPlugins.length > 0) {
      for (const p of installedPlugins) {
        if (!p.slug.startsWith('ai-provider-')) continue;
        // Recover the ProviderType from the slug when possible. The
        // catalog convention is `ai-provider-<type>`. If we can't
        // derive it, skip — without a ProviderType we can't create
        // a connection row.
        const inferredType = p.slug.replace(/^ai-provider-/, '');
        if (!inferredType || seen.has(inferredType)) continue;
        seen.add(inferredType);
        out.push({
          type: inferredType,
          displayName: displayNameFromSlug(p.slug),
          slug: p.slug,
          // No registered spec to read Icon from — every plugin
          // ships its brand icon as icons/plugin.svg, so name it
          // directly rather than leaving the tile iconless.
          icon: 'plugin.svg',
          installed: true,
          enabled: false,
        });
      }
    }

    // Stable, friendly order: alphabetical by display name.
    out.sort((a, b) => a.displayName.localeCompare(b.displayName));
    return out;
  }, [aiAdapters, installedPlugins]);

  return {
    adaptersQuery,
    installedPlugins,
    pickerEntries,
    enabledTypes: pickerEntries.filter((e) => e.enabled),
    disabledTypes: pickerEntries.filter((e) => !e.enabled),
    hasDisabledPlugin: installedPlugins.some((p) => p.slug.startsWith('ai-provider-') && !p.enabled),
    hasAnyAIPluginInstalled: pickerEntries.length > 0,
  };
}

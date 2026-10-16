// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect } from 'react';
import i18n from 'i18next';
import { usePluginLoader } from '@core/plugins/loader';

/**
 * pluginI18n exposes the deep-merged plugin namespaces for the
 * current active language. The loader fetches them once at boot
 * via `/api/plugins/loader/i18n?lang=<lang>` and stores them in
 * Zustand. This helper avoids the temptation to call addResource
 * directly from the sidebar / page components — the merge is
 * applied once in core/i18n/i18n.ts via applyPluginResources().
 */
export function getPluginNamespaces(): Record<string, Record<string, string>> {
  const loaded = usePluginLoader.getState().i18n;
  return loaded?.namespaces ?? {};
}

/**
 * applyPluginResources merges the plugin i18n namespaces into the
 * current i18next instance. Called from core/i18n/i18n.ts whenever
 * the active language changes or the plugin loader refreshes.
 */
export function applyPluginResources(
  i18nInstance: {
    addResourceBundle: (lang: string, ns: string, resource: unknown, deep: boolean, overwrite: boolean) => void;
  },
  lang: string,
): void {
  const namespaces = getPluginNamespaces();
  for (const [ns, resource] of Object.entries(namespaces)) {
    i18nInstance.addResourceBundle(lang, ns, resource, true, true);
  }
}

/**
 * usePluginI18nEffect re-runs applyPluginResources when the loader
 * resolves its i18n payload. Mounted by PluginBridges inside the
 * AppLayout so the merge happens after auth.
 *
 * Also subscribes to i18next's languageChanged event so the loader
 * re-fetches the plugin i18n bundle for the new language and the
 * sidebar / page labels re-render in the new locale without the user
 * having to reload the page. The server-side fallback (see
 * LoadI18n in plugin_runtime/service.go) means a plugin that only
 * ships English still renders correctly in any locale.
 */
export function usePluginI18nEffect() {
  const pluginI18n = usePluginLoader((s) => s.i18n);
  const refreshI18n = usePluginLoader((s) => s.refreshI18n);
  useEffect(() => {
    if (!pluginI18n) return;
    const lang = (i18n.language || 'en').substring(0, 2);
    applyPluginResources(i18n, lang);
  }, [pluginI18n]);
  useEffect(() => {
    const handler = () => {
      void refreshI18n();
    };
    i18n.on('languageChanged', handler);
    return () => {
      i18n.off('languageChanged', handler);
    };
  }, [refreshI18n]);
}

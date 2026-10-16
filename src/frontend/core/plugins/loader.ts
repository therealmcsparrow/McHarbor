// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { create } from 'zustand';
import { api } from '@core/api/client';
import i18n, { resolveCurrentLanguage } from '@core/i18n/i18n';
import { toastError } from '@resources/utils/toast';
import { pluginThemeAssetURL } from '@core/plugins/asset-url';
import { registerPluginThemes, type ThemeManifestEntry } from '../../src/themes';
import type {
  PluginRoute,
  PluginMenuItem,
  PluginWidget,
  PluginTheme,
  PluginNode,
  PluginStatus,
  PluginI18n,
  PluginCapability,
  PluginTabItem,
  PluginContextMenuItem,
  PluginTopbarItem,
  PluginFormItem,
  PluginLoaderState,
} from './loaderTypes';
export type {
  PluginRoute,
  PluginMenuItem,
  PluginWidgetSettings,
  PluginWidget,
  PluginTheme,
  PluginNode,
  PluginStatus,
  PluginI18n,
  PluginLoaderStatus,
  PluginCapability,
  PluginTabItem,
  PluginContextMenuItem,
  PluginTopbarItem,
  PluginFormItem,
  PluginFormMount,
  PluginFormContext,
} from './loaderTypes';
export type { PluginPickerList, PluginPickerSourceRef } from './pickerTypes';

/**
 * PluginLoader fetches the runtime plugin contribution tables from
 * `/api/plugins/loader/*` once at boot and caches them in a Zustand
 * store. App.tsx awaits `pluginLoader.init()` before mounting
 * `<RouterProvider>`; sidebar/widget/node registries read via the
 * `usePlugin*` hooks below.
 *
 * Mirrors the widget-registry pattern in
 * src/frontend/modules/dashboard/stores/widget-registry.ts.
 */

export const usePluginLoader = create<PluginLoaderState>()((set, get) => ({
  status: 'idle',
  routes: [],
  menu: [],
  widgets: [],
  themes: [],
  nodes: [],
  tabs: [],
  contextMenus: [],
  topbar: [],
  forms: [],
  capabilities: [],
  installed: [],
  i18n: null,
  init: async (opts) => {
    const force = opts?.force === true;
    if (!force && get().status === 'ready') return;
    set({ status: 'loading' });
    try {
      // Resolve the language through the centralized helper. Reading
      // `localStorage.getItem('mcharbor-language')` directly is unsafe:
      // the Zustand persist middleware serializes the value as JSON
      // (`{"state":{"language":"en"},"version":0}`) and dumping that
      // into a URL produced `?lang={"state` in the wild.
      const cleanLang = resolveCurrentLanguage();
      const [
        statusRes,
        routesRes,
        menuRes,
        widgetsRes,
        themesRes,
        nodesRes,
        tabsRes,
        contextRes,
        topbarRes,
        formsRes,
        capabilitiesRes,
        i18nRes,
      ] = await Promise.all([
        api.get<PluginStatus[]>('/plugins/loader/status'),
        api.get<PluginRoute[]>('/plugins/loader/routes'),
        api.get<PluginMenuItem[]>('/plugins/loader/menu'),
        api.get<PluginWidget[]>('/plugins/loader/widgets'),
        api.get<PluginTheme[]>('/plugins/loader/themes'),
        api.get<PluginNode[]>('/plugins/loader/nodes'),
        api.get<PluginTabItem[]>('/plugins/loader/tabs'),
        api.get<PluginContextMenuItem[]>('/plugins/loader/context-menus'),
        api.get<PluginTopbarItem[]>('/plugins/loader/topbar'),
        api.get<PluginFormItem[]>('/plugins/loader/forms'),
        api.get<PluginCapability[]>('/plugins/loader/capabilities'),
        api.get<PluginI18n>(`/plugins/loader/i18n?lang=${cleanLang}`),
      ]);
      // Treat any individual failure as "no contributions from this
      // surface". The plugin loader endpoints may be absent (the host
      // app may not have the plugins module mounted) and the host UI
      // must continue to render in that case.
      const themes = themesRes.data ?? [];
      registerPluginThemes(toThemeManifestEntries(themes));
      set({
        status: 'ready',
        installed: statusRes.data ?? [],
        routes: routesRes.data ?? [],
        menu: menuRes.data ?? [],
        widgets: widgetsRes.data ?? [],
        themes,
        nodes: nodesRes.data ?? [],
        tabs: tabsRes.data ?? [],
        contextMenus: contextRes.data ?? [],
        topbar: topbarRes.data ?? [],
        forms: formsRes.data ?? [],
        capabilities: capabilitiesRes.data ?? [],
        i18n: i18nRes.data ?? null,
      });
    } catch (e) {
      // Phase A: failing the loader must not abort the app boot.
      // Mark as error and continue; the admin UI can call refresh()
      // to retry after the user investigates.
      // eslint-disable-next-line no-console
      console.warn('plugin loader: init failed', e);
      toastError(e, i18n.t('errors.load_failed', { ns: 'plugins' }));
      set({ status: 'error' });
    }
  },
  refresh: (opts) => get().init({ force: opts?.force ?? true }),
  refreshI18n: async () => {
    // Re-fetch only the i18n bundle for the active language. Used
    // by the plugin i18n effect when the user switches language
    // so the sidebar / labels re-render in the new locale without
    // rebuilding the rest of the contribution tables.
    const cleanLang = resolveCurrentLanguage();
    try {
      const i18nRes = await api.get<PluginI18n>(`/plugins/loader/i18n?lang=${cleanLang}`);
      set({ i18n: i18nRes.data ?? null });
    } catch (e) {
      // eslint-disable-next-line no-console
      console.warn('plugin loader: refreshI18n failed', e);
    }
  },
}));

export const usePluginMenu = () => usePluginLoader((s) => s.menu);
export const usePluginWidgets = () => usePluginLoader((s) => s.widgets);
export const usePluginThemes = () => usePluginLoader((s) => s.themes);
export const usePluginNodes = () => usePluginLoader((s) => s.nodes);
export const usePluginTopbar = () => usePluginLoader((s) => s.topbar);
export const usePluginForms = () => usePluginLoader((s) => s.forms);
export const usePluginInstalled = () => usePluginLoader((s) => s.installed);

function toThemeManifestEntries(themes: PluginTheme[]): ThemeManifestEntry[] {
  return themes.map((theme) => ({
    id: `plugin:${theme.plugin_slug}:${theme.id}`,
    name: theme.name,
    description: theme.description,
    cssPath: pluginThemeAssetURL(theme.plugin_slug, theme.css_path),
    swatchHue: theme.swatch_hue,
    defaultVariant: theme.default_variant === 'light' ? 'light' : 'dark',
    hasDark: theme.has_dark,
    hasLight: theme.has_light,
  }));
}

export const pluginLoader = {
  init: () => usePluginLoader.getState().init(),
  refresh: (opts?: { force?: boolean }) => usePluginLoader.getState().refresh(opts),
  status: () => usePluginLoader.getState().status,
  routes: () => usePluginLoader.getState().routes,
  menu: () => usePluginLoader.getState().menu,
  widgets: () => usePluginLoader.getState().widgets,
  themes: () => usePluginLoader.getState().themes,
  nodes: () => usePluginLoader.getState().nodes,
  installed: () => usePluginLoader.getState().installed,
};

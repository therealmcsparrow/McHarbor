// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';

import { api } from '@core/api/client';
import type { InstalledPlugin } from '../types/plugins';
import { PLUGIN_QUERY_KEY } from './pluginQueryKey';

// Shape returned by UninstallConfirmDialog. Each count corresponds to
// a contribution table in `plugin_*` rows. `abac_rules` /
// `data_tables` / `migrations` / `config_keys` are not yet exposed
// through loader endpoints, so they stay at 0 until the host adds
// matching loader routes.
export type UninstallPreview = {
  drop_targets: {
    routes: number;
    menu: number;
    topbar: number;
    context_menus: number;
    tabs: number;
    widgets: number;
    nodes: number;
    abac_rules: number;
    data_tables: number;
    migrations: number;
    config_keys: number;
  };
  permissions: {
    referenced_by_roles: number;
    claimed: number;
    // Names of the roles that will lose their grants on uninstall.
    roles: string[];
  };
};

// Loader endpoints return arrays of objects that share a
// `plugin_slug` field. `PluginContribution` captures the shared shape
// for the filter pass.
type PluginContribution = { plugin_slug?: string };

function countWhere<T extends PluginContribution>(list: T[] | undefined, slug: string): number {
  if (!list) return 0;
  return list.reduce((n, it) => (it.plugin_slug === slug ? n + 1 : n), 0);
}

/**
 * useUninstallPreview computes the impact of uninstalling a plugin by
 * fanning out to the seven `/api/plugins/loader/*` endpoints (which
 * already enumerate every contribution table) and filtering each
 * list down to the target slug.
 *
 * Permissions claimed = the sum of contribution entries the plugin
 * owns (cheap proxy until a dedicated `/loader/permissions` endpoint
 * is added). Permissions referenced by roles is read from the
 * installed-plugin row, which the backend already returns with the
 * `permissions_referenced` count joined against the RBAC table.
 *
 * Returns the same preview shape UninstallConfirmDialog consumes, so
 * the dialog renders real numbers instead of the all-zeros stub.
 */
export function useUninstallPreview(slug: string | null) {
  const routes = useQuery({
    queryKey: ['plugins', 'loader', 'routes'],
    queryFn: () => api.get<PluginContribution[]>('/plugins/loader/routes').then((r) => r.data ?? []),
    enabled: !!slug,
    staleTime: 30_000,
  });
  const menu = useQuery({
    queryKey: ['plugins', 'loader', 'menu'],
    queryFn: () => api.get<PluginContribution[]>('/plugins/loader/menu').then((r) => r.data ?? []),
    enabled: !!slug,
    staleTime: 30_000,
  });
  const topbar = useQuery({
    queryKey: ['plugins', 'loader', 'topbar'],
    queryFn: () => api.get<PluginContribution[]>('/plugins/loader/topbar').then((r) => r.data ?? []),
    enabled: !!slug,
    staleTime: 30_000,
  });
  const contextMenus = useQuery({
    queryKey: ['plugins', 'loader', 'context-menus'],
    queryFn: () => api.get<PluginContribution[]>('/plugins/loader/context-menus').then((r) => r.data ?? []),
    enabled: !!slug,
    staleTime: 30_000,
  });
  const tabs = useQuery({
    queryKey: ['plugins', 'loader', 'tabs'],
    queryFn: () => api.get<PluginContribution[]>('/plugins/loader/tabs').then((r) => r.data ?? []),
    enabled: !!slug,
    staleTime: 30_000,
  });
  const widgets = useQuery({
    queryKey: ['plugins', 'loader', 'widgets'],
    queryFn: () => api.get<PluginContribution[]>('/plugins/loader/widgets').then((r) => r.data ?? []),
    enabled: !!slug,
    staleTime: 30_000,
  });
  const nodes = useQuery({
    queryKey: ['plugins', 'loader', 'nodes'],
    queryFn: () => api.get<PluginContribution[]>('/plugins/loader/nodes').then((r) => r.data ?? []),
    enabled: !!slug,
    staleTime: 30_000,
  });

  // Installed list provides permissions_referenced per slug. We
  // can't use useInstalledPlugins() directly because it lives in the
  // same module — but we can hit the same endpoint via `api.get` to
  // avoid a circular import. The InstalledPlugin type includes a
  // permissive count field; cast through unknown so we don't
  // over-specify the shape.
  const installed = useQuery({
    queryKey: [PLUGIN_QUERY_KEY, 'installed'],
    queryFn: () => api.get<InstalledPlugin[]>('/appstore/plugins/installed').then((r) => r.data ?? []),
    enabled: !!slug,
    staleTime: 30_000,
  });

  const target = slug ?? '';
  const allQueries = [routes, menu, topbar, contextMenus, tabs, widgets, nodes, installed];
  const isLoading = allQueries.some((q) => q.isLoading);
  const isError = allQueries.some((q) => q.isError);
  const error = allQueries.find((q) => q.isError)?.error ?? null;

  const routesCount = countWhere(routes.data, target);
  const menuCount = countWhere(menu.data, target);
  const topbarCount = countWhere(topbar.data, target);
  const contextCount = countWhere(contextMenus.data, target);
  const tabsCount = countWhere(tabs.data, target);
  const widgetsCount = countWhere(widgets.data, target);
  const nodesCount = countWhere(nodes.data, target);

  const claimed = routesCount + menuCount + topbarCount + contextCount + tabsCount + widgetsCount + nodesCount;

  const referencedRow = (installed.data ?? []).find((p) => p.slug === target) as
    | (InstalledPlugin & { permissions_claimed?: number })
    | undefined;

  const preview: UninstallPreview = {
    drop_targets: {
      routes: routesCount,
      menu: menuCount,
      topbar: topbarCount,
      context_menus: contextCount,
      tabs: tabsCount,
      widgets: widgetsCount,
      nodes: nodesCount,
      // Not yet exposed by the loader; the dialog renders these as
      // 0 when the contribution table doesn't have a matching entry.
      abac_rules: 0,
      data_tables: 0,
      migrations: 0,
      config_keys: 0,
    },
    permissions: {
      referenced_by_roles: referencedRow?.permissions_referenced ?? 0,
      claimed: referencedRow?.permissions_claimed ?? claimed,
      roles: referencedRow?.roles_referencing ?? [],
    },
  };

  return {
    data: preview,
    isLoading,
    isError,
    error,
    refetch: async () => {
      await Promise.all(allQueries.map((q) => q.refetch()));
      return {
        data: preview,
        isLoading: false,
        isError: false,
        error: null,
      } as never;
    },
  };
}

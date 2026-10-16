// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';

import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { pluginLoader } from '@core/plugins/loader';
import type { InstalledPlugin, PluginEntry } from '../types/plugins';
import { PLUGIN_QUERY_KEY } from './pluginQueryKey';

export { PLUGIN_QUERY_KEY };
export { useUninstallPreview, type UninstallPreview } from './useUninstallPreview';

// --- Catalog --------------------------------------------------------

// usePluginCatalog returns the browsable list of installable
// plugins served from the local catalog directory mounted into the
// container. The catalog is a pure read; an empty result renders
// the "no catalog configured" state in the UI (not an error).
export function usePluginCatalog() {
  return useQuery({
    queryKey: [PLUGIN_QUERY_KEY, 'catalog'],
    queryFn: () => api.get<PluginEntry[]>('/appstore/plugins/catalog').then((r) => r.data ?? []),
    staleTime: 30_000,
  });
}

// --- Installed ------------------------------------------------------

export function useInstalledPlugins() {
  return useQuery({
    queryKey: [PLUGIN_QUERY_KEY, 'installed'],
    queryFn: () => api.get<InstalledPlugin[]>('/appstore/plugins/installed').then((r) => r.data ?? []),
  });
}

// --- Mutations ------------------------------------------------------

function refreshAll(qc: ReturnType<typeof useQueryClient>) {
  void qc.invalidateQueries({ queryKey: [PLUGIN_QUERY_KEY] });
  // Re-scan the loader so newly enabled plugins contribute routes
  // / menu / widgets immediately.
  void pluginLoader.refresh();
}

export type ReconcileResult = {
  slug: string;
  version: string;
  action: 'kept' | 'installed' | 'updated';
};

/**
 * useReconcilePlugin pins one plugin to an exact catalog version: the host
 * keeps, installs or moves it so the record, files and binary agree, with
 * role grants intact. `enabled` must be sent, because the server defaults an
 * omitted flag to true and would re-enable a plugin the operator turned off.
 */
export function useReconcilePlugin() {
  const { t } = useTranslation('plugins');
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ slug, version, enabled }: { slug: string; version: string; enabled: boolean }) =>
      assertSuccess(
        await api.post<ReconcileResult>(`/appstore/plugins/${encodeURIComponent(slug)}/reconcile`, {
          version,
          enabled,
        }),
      ),
    meta: {
      success: (data: unknown) => {
        const result = data as ReconcileResult;
        const outcome =
          result.action === 'installed'
            ? t('repair.reconciledInstalled')
            : result.action === 'updated'
              ? t('repair.reconciledUpdated')
              : t('repair.reconciledKept');
        return t('repair.reconciled', {
          slug: result.slug,
          version: result.version,
          outcome,
        });
      },
    },
    onSuccess: () => refreshAll(qc),
  });
}

export function useUninstallPlugin() {
  const { t } = useTranslation('plugins');
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ slug, purgeData }: { slug: string; purgeData: boolean }) =>
      api
        .post<{ roles_stripped?: string[] }>('/appstore/plugins/uninstall', {
          slug,
          purge_data: purgeData,
        })
        .then((r) => {
          // The api client resolves on every non-401 status, so a
          // failed response would otherwise land in onSuccess and
          // toast "uninstalled" over a plugin that is still there.
          if (!r.success) throw new Error(r.error ?? t('toast.uninstall_failed'));
          return r.data?.roles_stripped ?? [];
        }),
    meta: {
      // Uninstall strips the plugin's keys from any role that still
      // granted them. Say which, so an admin who kept a custom role
      // pointed at the plugin learns it changed from the toast rather
      // than from a user who lost access.
      success: (roles: string[]) =>
        roles.length > 0 ? t('toast.uninstalled_roles_stripped', { roles: roles.join(', ') }) : t('toast.uninstalled'),
    },
    onSuccess: () => {
      refreshAll(qc);
      void qc.invalidateQueries({ queryKey: ['roles'] });
    },
  });
}

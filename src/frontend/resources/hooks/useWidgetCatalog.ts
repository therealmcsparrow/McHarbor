// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { usePermission } from '@core/auth/usePermission';
import { PERM } from '@core/auth/permissionKeys';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export type WidgetCatalogDefinition = {
  key: string;
  label: string;
  category: string;
  description: string;
  icon: string;
  source: string;
  component: string;
  enabled: boolean;
  defaultSize: { w: number; h: number };
  minSize: { w: number; h: number };
  version?: string;
  /** Set by the server only when a downloaded widget's stored bundle still verifies against a trust key. */
  verified?: boolean;
  translations?: Record<string, Record<string, unknown>>;
};

export type CatalogWidget = {
  key: string;
  version: string;
  label: string;
  description: string;
  icon: string;
  defaultSize: { w: number; h: number };
  signed: boolean;
  installed: boolean;
  installedVersion?: string;
};

export function useWidgetCatalogDefinitions() {
  // widgets.view gates the catalog route; without it the query is a 403 the
  // dashboard cannot act on.
  const allowed = usePermission(PERM.WIDGETS_VIEW);
  return useQuery({
    enabled: allowed,
    queryKey: ['widgets', 'definitions'],
    queryFn: () => api.get<WidgetCatalogDefinition[]>('/widgets/definitions').then(assertSuccess),
    staleTime: 5 * 60_000,
  });
}

export function useUpdateWidgetCatalogEnabled() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ key, enabled }: { key: string; enabled: boolean }) =>
      api.put<{ key: string; enabled: boolean }>(`/widgets/${key}`, { enabled }).then(assertSuccess),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['widgets', 'definitions'] });
    },
  });
}

export function useWidgetCatalogEntries() {
  const allowed = usePermission(PERM.WIDGETS_VIEW);
  return useQuery({
    enabled: allowed,
    queryKey: ['widgets', 'catalog'],
    queryFn: () => api.get<CatalogWidget[]>('/widgets/catalog').then(assertSuccess),
  });
}

export function useInstallCatalogWidget() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (key: string) =>
      api.post<WidgetCatalogDefinition>(`/widgets/catalog/${encodeURIComponent(key)}/install`).then(assertSuccess),
    meta: { error: 'widgets.install' },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['widgets'] });
    },
  });
}

export function useRemoveDownloadedWidget() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (key: string) => api.del<unknown>(`/widgets/${encodeURIComponent(key)}`).then(assertSuccess),
    meta: { error: 'widgets.remove' },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['widgets'] });
    },
  });
}

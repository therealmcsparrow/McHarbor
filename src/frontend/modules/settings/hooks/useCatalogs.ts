// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

// Catalog mirrors the Go struct in modules/catalogs/model.go.
export interface Catalog {
  id: string;
  name: string;
  url: string;
  kind: 'docker_app' | 'plugin' | 'theme' | 'vm_image';
  enabled: boolean;
  readonly: boolean;
  origin?: string;
  created_at: string;
  updated_at: string;
}

export const CATALOG_KINDS = ['docker_app', 'plugin', 'theme', 'vm_image'] as const;
export type CatalogKind = (typeof CATALOG_KINDS)[number];

// Widgets and workflow nodes no longer have a catalog kind of their own:
// they ship as plugin bundles (`widget-xx`, `nodes-xx`) and arrive through
// the plugin catalog. Toggling an individual widget or node on and off
// still lives in the App Store, untouched by this list.
export const RENDERABLE_CATALOG_KINDS = ['docker_app', 'plugin', 'vm_image'] as const satisfies readonly CatalogKind[];
export type VisibleCatalogKind = (typeof RENDERABLE_CATALOG_KINDS)[number];

const ALWAYS_VISIBLE_KINDS = ['docker_app', 'plugin'] as const satisfies readonly VisibleCatalogKind[];

export function visibleCatalogKinds(catalogs: Catalog[] | undefined): VisibleCatalogKind[] {
  const present = new Set((catalogs ?? []).map((c) => c.kind));
  return RENDERABLE_CATALOG_KINDS.filter(
    (kind) => (ALWAYS_VISIBLE_KINDS as readonly string[]).includes(kind) || present.has(kind),
  );
}

export function useCatalogs() {
  return useQuery({
    queryKey: ['settings', 'catalogs'],
    queryFn: () => api.get<Catalog[]>('/catalogs').then((r) => r.data ?? []),
  });
}

export function useCreateCatalog() {
  const { t } = useTranslation('settings');
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (req: { name: string; url: string; kind: VisibleCatalogKind }) =>
      api.post<Catalog>('/catalogs', req).then((res) => {
        // The URL may belong to a built-in row the operator has disabled, so name it:
        // "already configured" alone sends them looking for a catalog they cannot see.
        const data = res.data as { reason?: unknown; name?: unknown } | undefined;
        if (!res.success && data?.reason === 'duplicate_url') {
          throw new Error(t('catalogs.errors.duplicateUrl', { name: typeof data.name === 'string' ? data.name : '' }));
        }
        return assertSuccess(res);
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['settings', 'catalogs'] });
      void queryClient.invalidateQueries({ queryKey: ['app-store'] });
      toast.success(t('catalogs.toast.created'));
    },
    meta: { error: (err: Error) => t('catalogs.toast.createFailed', { error: err.message }) },
  });
}

export function useUpdateCatalog() {
  const { t } = useTranslation('settings');
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, patch }: { id: string; patch: { name?: string; url?: string; enabled?: boolean } }) =>
      api.put<Catalog>(`/catalogs/${id}`, patch).then(assertSuccess),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['settings', 'catalogs'] });
      void queryClient.invalidateQueries({ queryKey: ['app-store'] });
      toast.success(t('catalogs.toast.updated'));
    },
    meta: { error: (err: Error) => t('catalogs.toast.updateFailed', { error: err.message }) },
  });
}

export function useDeleteCatalog() {
  const { t } = useTranslation('settings');
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.del<void>(`/catalogs/${id}`).then(assertSuccess),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['settings', 'catalogs'] });
      void queryClient.invalidateQueries({ queryKey: ['app-store'] });
      toast.success(t('catalogs.toast.deleted'));
    },
    meta: { error: (err: Error) => t('catalogs.toast.deleteFailed', { error: err.message }) },
  });
}

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import type { PaginatedData } from '@core/api/client';
import type {
  AppTemplate,
  CategoryCount,
  InstallRequest,
  InstallResult,
  InstalledApp,
  SyncStatus,
} from '../types';

export function useAppCatalog(category?: string, search?: string, page = 1, perPage = 50) {
  const params: Record<string, string> = {
    page: String(page),
    per_page: String(perPage),
  };
  if (category) params.category = category;
  if (search) params.search = search;

  return useQuery({
    queryKey: ['app-store', 'catalog', category, search, page],
    queryFn: () =>
      api
        .get<PaginatedData<AppTemplate>>('/app-store', params)
        .then((r) => r.data ?? { items: [], total: 0, page: 1, per_page: perPage, total_pages: 0 }),
  });
}

export function useAppDetail(slug: string) {
  return useQuery({
    queryKey: ['app-store', 'detail', slug],
    queryFn: () =>
      api.get<AppTemplate>(`/app-store/${slug}`).then((r) => r.data),
    enabled: !!slug,
  });
}

export function useAppCategories() {
  return useQuery({
    queryKey: ['app-store', 'categories'],
    queryFn: () =>
      api
        .get<CategoryCount[]>('/app-store/categories')
        .then((r) => r.data ?? []),
  });
}

export function useInstallApp() {
  const { t } = useTranslation('common');
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (req: InstallRequest) =>
      api.post<InstallResult>('/app-store/install', req).then((r) => {
        if (!r.success) throw new Error(r.error ?? 'Install failed');
        return r.data as InstallResult;
      }),
    meta: { success: t('appStore.mutationInstalled') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['app-store'] });
      queryClient.invalidateQueries({ queryKey: ['stacks'] });
    },
  });
}

export function useSyncCatalog() {
  const { t } = useTranslation('common');
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () =>
      api.post('/app-store/sync').then((r) => {
        if (!r.success) throw new Error(r.error ?? 'Sync failed');
        return r.data;
      }),
    meta: { success: t('appStore.mutationSynced') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['app-store'] });
    },
  });
}

export function useSyncStatus() {
  return useQuery({
    queryKey: ['app-store', 'sync-status'],
    queryFn: () =>
      api
        .get<SyncStatus>('/app-store/sync/status')
        .then((r) => r.data),
  });
}

export function useInstalledApps() {
  return useQuery({
    queryKey: ['app-store', 'installed'],
    queryFn: () =>
      api
        .get<InstalledApp[]>('/app-store/installed')
        .then((r) => r.data ?? []),
  });
}


// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useInfiniteQuery, useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { PaginatedData } from '@core/api/client';
import type { AppTemplate, CategoryCount, SyncStatus } from '../types';

// The catalog runs to well over a thousand entries, so it is fetched page by
// page as the grid scrolls; a single page used to be all "All" ever showed.
export function useAppCatalog(category?: string, search?: string, perPage = 60) {
  return useInfiniteQuery({
    queryKey: ['app-store', 'catalog', category, search, perPage],
    queryFn: async ({ pageParam }) => {
      const params: Record<string, string> = {
        page: String(pageParam),
        per_page: String(perPage),
      };
      if (category) params.category = category;
      if (search) params.search = search;
      const res = await api.get<PaginatedData<AppTemplate>>('/app-store', params);
      return assertSuccess(res) ?? { items: [], total: 0, page: pageParam, per_page: perPage, total_pages: 0 };
    },
    initialPageParam: 1,
    getNextPageParam: (lastPage) => (lastPage.page < lastPage.total_pages ? lastPage.page + 1 : undefined),
  });
}

export function useAppCategories() {
  return useQuery({
    queryKey: ['app-store', 'categories'],
    queryFn: () => api.get<CategoryCount[]>('/app-store/categories').then((r) => assertSuccess(r) ?? []),
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
      void queryClient.invalidateQueries({ queryKey: ['app-store'] });
    },
  });
}

export function useSyncStatus() {
  return useQuery({
    queryKey: ['app-store', 'sync-status'],
    queryFn: () => api.get<SyncStatus>('/app-store/sync/status').then(assertSuccess),
  });
}

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { LayoutPayload } from './table-layout';

export type SavedFilter = {
  id: string;
  tableId: string;
  name: string;
  state: Record<string, unknown>;
  isDefault: boolean;
};

export type TableViewData = {
  layout: { id: string; payload: LayoutPayload } | null;
  filters: SavedFilter[];
};

export const tableViewKey = (tableId: string) => ['table-views', tableId] as const;

const base = (tableId: string) => `/table-views/${encodeURIComponent(tableId)}`;

export function useTableViewQuery(tableId: string) {
  return useQuery({
    queryKey: tableViewKey(tableId),
    queryFn: () => api.get<TableViewData>(base(tableId)).then(assertSuccess),
    staleTime: Number.POSITIVE_INFINITY,
    retry: 1,
  });
}

export function useSaveLayout(tableId: string) {
  const { t } = useTranslation('common');
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (payload: LayoutPayload) =>
      api.put<{ id: string; payload: LayoutPayload }>(`${base(tableId)}/layout`, payload).then(assertSuccess),
    meta: { error: t('tableViews.errors.saveLayout') },
    onMutate: async (payload) => {
      await queryClient.cancelQueries({ queryKey: tableViewKey(tableId) });
      const previous = queryClient.getQueryData<TableViewData>(tableViewKey(tableId));
      if (previous) {
        queryClient.setQueryData<TableViewData>(tableViewKey(tableId), {
          ...previous,
          layout: { id: previous.layout?.id ?? '', payload },
        });
      }
      return { previous };
    },
    onSuccess: (data) => {
      queryClient.setQueryData<TableViewData>(tableViewKey(tableId), (prev) =>
        prev ? { ...prev, layout: { id: data.id, payload: data.payload } } : prev,
      );
    },
    onError: (_error, _payload, context) => {
      if (context?.previous) queryClient.setQueryData(tableViewKey(tableId), context.previous);
    },
  });
}

export function useResetLayout(tableId: string) {
  const { t } = useTranslation('common');
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => api.del(`${base(tableId)}/layout`).then(assertSuccess),
    meta: { error: t('tableViews.errors.resetLayout') },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: tableViewKey(tableId) }),
  });
}

export function useCreateFilter(tableId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: { name: string; state: Record<string, unknown>; isDefault: boolean }) =>
      api.post<SavedFilter>(`${base(tableId)}/filters`, input).then(assertSuccess),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: tableViewKey(tableId) }),
  });
}

export function useUpdateFilter(tableId: string) {
  const { t } = useTranslation('common');
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      ...input
    }: {
      id: string;
      name?: string;
      state?: Record<string, unknown>;
      isDefault?: boolean;
    }) => api.put<SavedFilter>(`${base(tableId)}/filters/${encodeURIComponent(id)}`, input).then(assertSuccess),
    meta: { error: t('tableViews.errors.saveFilter') },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: tableViewKey(tableId) }),
  });
}

export function useDeleteFilter(tableId: string) {
  const { t } = useTranslation('common');
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.del(`${base(tableId)}/filters/${encodeURIComponent(id)}`).then(assertSuccess),
    meta: { success: t('tableViews.toast.deleted'), error: t('tableViews.errors.deleteFilter') },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: tableViewKey(tableId) }),
  });
}

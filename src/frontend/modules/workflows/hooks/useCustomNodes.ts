// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { CustomNodeWithCode } from './useCustomNodeSync';

export function useCustomNodes() {
  return useQuery({
    queryKey: ['custom-nodes'],
    queryFn: () =>
      api.get<CustomNodeWithCode[]>('/custom-nodes').then((r) => r.data ?? []),
  });
}

export function useCustomNode(key: string) {
  return useQuery({
    queryKey: ['custom-nodes', key],
    queryFn: () =>
      api.get<CustomNodeWithCode>(`/custom-nodes/${key}`).then((r) => r.data),
    enabled: !!key,
  });
}

export function useCreateCustomNode() {
  const { t } = useTranslation('common');
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (data: Omit<CustomNodeWithCode, 'source'>) =>
      api.post<CustomNodeWithCode>('/custom-nodes', data).then(assertSuccess),
    meta: { success: t('customNodes.toast.created') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['custom-nodes'] });
    },
  });
}

export function useUpdateCustomNode(key: string) {
  const { t } = useTranslation('common');
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (data: Partial<Omit<CustomNodeWithCode, 'source'>>) =>
      api.put<CustomNodeWithCode>(`/custom-nodes/${key}`, data).then(assertSuccess),
    meta: { success: t('customNodes.toast.updated') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['custom-nodes'] });
    },
  });
}

export function useDeleteCustomNode() {
  const { t } = useTranslation('common');
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (nodeKey: string) =>
      api.del(`/custom-nodes/${nodeKey}`).then(assertSuccess),
    meta: { success: t('customNodes.toast.deleted') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['custom-nodes'] });
    },
  });
}

export function useTestCustomNode() {
  return useMutation({
    mutationFn: (data: {
      code: string;
      config: Record<string, unknown>;
      msg: Record<string, unknown>;
    }) => api.post<{ result: unknown; logs: string[] }>('/custom-nodes/test', data).then(assertSuccess),
  });
}

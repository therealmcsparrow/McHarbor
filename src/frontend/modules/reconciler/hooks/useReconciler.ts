// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api, type PaginatedData } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { useEnvironmentStore } from '@resources/stores/environment';

export type DesiredState = {
  id: string;
  name: string;
  description: string;
  containerName: string;
  imageRef: string;
  desiredStatus: string;
  restartPolicy: string;
  lastReconcile: string | null;
  driftDetected: boolean;
  createdAt: string;
};

export function useDesiredStates() {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['reconciler', envId],
    queryFn: () =>
      api
        .get<PaginatedData<DesiredState>>('/reconciler', {
          per_page: '100',
          ...(envId ? { env: envId } : {}),
        })
        .then((r) => r.data?.items ?? []),
    refetchInterval: 15_000,
  });
}

export function useReconcile(t: (key: string) => string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.post(`/reconciler/${id}/reconcile`).then(assertSuccess),
    meta: { success: t('reconciler.mutationReconciled') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['reconciler'] });
    },
  });
}

export function useCheckDrift(t: (key: string) => string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.get(`/reconciler/${id}/drift`).then(assertSuccess),
    meta: { success: t('reconciler.mutationDriftChecked') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['reconciler'] });
    },
  });
}

export function useCreateDesiredState(t: (key: string) => string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (data: { name: string; containerName: string; imageRef: string; desiredStatus: string }) =>
      api.post('/reconciler', data).then(assertSuccess),
    meta: { success: t('reconciler.mutationCreated') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['reconciler'] });
    },
  });
}

export function useDeleteDesiredState(t: (key: string) => string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.del(`/reconciler/${id}`).then(assertSuccess),
    meta: { success: t('reconciler.mutationDeleted') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['reconciler'] });
    },
  });
}

export function deriveStatus(s: DesiredState): string {
  if (s.driftDetected) return 'drifted';
  if (s.lastReconcile) return 'synced';
  return 'pending';
}

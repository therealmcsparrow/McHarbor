// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api, type PaginatedData } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { useEnvironmentStore } from '@resources/stores/environment';
import { refusalOf } from './refusal';

export type DriftClass = 'configuration' | 'state' | 'existence' | 'image';

export type DriftDiff = {
  class: DriftClass;
  field: string;
  expected: string;
  actual: string;
  firstSeenAt?: string;
};

export type DriftReport = {
  desiredStateId: string;
  hasDrift: boolean;
  diffs: DriftDiff[];
  checkedAt: string;
};

export type DesiredStateKind = 'container' | 'stack' | 'environment';

export type ReconcileMode = 'observe' | 'propose' | 'enforce';

export type DesiredState = {
  id: string;
  name: string;
  description: string;
  kind: DesiredStateKind | (string & {});
  stackName: string;
  lastReport?: DriftReport;
  source?: string;
  subjectId?: string;
  spec?: Record<string, string>;
  containerName: string;
  imageRef: string;
  desiredStatus: string;
  restartPolicy: string;
  lastReconcile: string | null;
  driftDetected: boolean;
  createdAt: string;
  envId?: string;
  updatedAt?: string;
  mode: ReconcileMode;
  sourceId?: string;
  sourcePath?: string;
  lastEnforcedAt?: string;
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
      void queryClient.invalidateQueries({ queryKey: ['reconciler'] });
    },
  });
}

export function useCheckDrift(t: (key: string) => string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.get(`/reconciler/${id}/drift`).then(assertSuccess),
    meta: { success: t('reconciler.mutationDriftChecked') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['reconciler'] });
    },
  });
}

export type CreateDesiredStateInput = {
  name: string;
  kind: DesiredStateKind | (string & {});
  containerName: string;
  imageRef: string;
  stackName: string;
  desiredStatus: string;
  mode: Exclude<ReconcileMode, 'enforce'>;
  source?: string;
  subjectId?: string;
  spec?: Record<string, string>;
  envId?: string;
};

export function useCreateDesiredState(t: (key: string) => string) {
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);
  return useMutation({
    mutationFn: (data: CreateDesiredStateInput) =>
      api.post('/reconciler', { ...data, envId: data.envId ?? envId }).then(assertSuccess),
    meta: { success: t('reconciler.mutationCreated') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['reconciler'] });
    },
  });
}

export function useDeleteDesiredState(t: (key: string) => string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.del(`/reconciler/${id}`).then(assertSuccess),
    meta: { success: t('reconciler.mutationDeleted') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['reconciler'] });
    },
  });
}

export function deriveStatus(s: DesiredState): string {
  if (s.driftDetected) return 'drifted';
  if (s.lastReconcile) return 'synced';
  return 'pending';
}

export function useSetMode() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, mode }: { id: string; mode: Exclude<ReconcileMode, 'enforce'> }) =>
      api.put<DesiredState>(`/reconciler/${encodeURIComponent(id)}`, { mode }).then(refusalOf),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['reconciler'] }),
  });
}

export function useEnforce() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.post<DesiredState>(`/reconciler/${encodeURIComponent(id)}/enforce`).then(refusalOf),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['reconciler'] }),
  });
}

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { ContainerBackupPlan, ContainerBackupRun, ContainerBackupInput } from './types';
import { envQuery } from './shared';
import { containerBackupKeys } from './queryKeys';

export function useRunContainerBackup(containerId: string) {
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);
  const { t } = useTranslation('containers');

  return useMutation({
    mutationFn: (body: ContainerBackupInput) =>
      api
        .post<ContainerBackupRun>(`/containers/${containerId}/backups/run${envQuery(envId)}`, body)
        .then(assertSuccess),
    meta: { success: () => t('backups.toast.started') },
    onMutate: async () => {
      const queryKey = containerBackupKeys.runs(envId, containerId);
      const optimisticRunId = `pending-${Date.now()}`;
      await queryClient.cancelQueries({ queryKey });
      queryClient.setQueryData<ContainerBackupRun[]>(queryKey, (current = []) => [
        {
          id: optimisticRunId,
          planId: '',
          operation: 'backup',
          environmentId: envId ?? '',
          containerId,
          status: 'running',
          archiveSize: 0,
          progressStage: 'queued',
          destinations: [],
          startedAt: new Date().toISOString(),
          durationMs: 0,
          createdAt: new Date().toISOString(),
          updatedAt: new Date().toISOString(),
        },
        ...current.filter((run) => run.id !== optimisticRunId),
      ]);
      return { optimisticRunId, queryKey };
    },
    onSuccess: (run, _variables, context) => {
      queryClient.setQueryData<ContainerBackupRun[]>(context.queryKey, (current = []) => [
        run,
        ...current.filter((item) => item.id !== context.optimisticRunId && item.id !== run.id),
      ]);
      void queryClient.invalidateQueries({ queryKey: containerBackupKeys.runsAll });
    },
    onError: (_error, _variables, context) => {
      if (!context) return;
      queryClient.setQueryData<ContainerBackupRun[]>(context.queryKey, (current = []) =>
        current.filter((run) => run.id !== context.optimisticRunId),
      );
      void queryClient.invalidateQueries({ queryKey: containerBackupKeys.runsAll });
    },
  });
}

export function useCreateContainerBackupPlan(containerId: string) {
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);
  const { t } = useTranslation('containers');

  return useMutation({
    mutationFn: (body: ContainerBackupInput) =>
      api
        .post<ContainerBackupPlan>(`/container-backups${envQuery(envId)}`, {
          ...body,
          containerId,
        })
        .then(assertSuccess),
    meta: { success: () => t('backups.toast.scheduleCreated') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: containerBackupKeys.plansAll });
    },
  });
}

export function useUpdateContainerBackupPlan() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('containers');

  return useMutation({
    mutationFn: ({ id, body }: { id: string; body: ContainerBackupInput }) =>
      api.put<ContainerBackupPlan>(`/container-backups/${encodeURIComponent(id)}`, body).then(assertSuccess),
    meta: { success: () => t('backups.toast.scheduleUpdated') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: containerBackupKeys.plansAll });
    },
  });
}

export function useDeleteContainerBackupPlan() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('containers');

  return useMutation({
    mutationFn: (id: string) => api.del(`/container-backups/${id}`).then(assertSuccess),
    meta: { success: () => t('backups.toast.scheduleDeleted') },
    onMutate: async (id) => {
      // Optimistic removal: drop the plan from every cached query
      // so the table row disappears immediately. The rollback
      // restores the previous snapshot if the delete fails.
      await queryClient.cancelQueries({ queryKey: containerBackupKeys.plansAll });
      const previous = queryClient.getQueriesData<ContainerBackupPlan[]>({
        queryKey: containerBackupKeys.plansAll,
      });
      queryClient.setQueriesData<ContainerBackupPlan[]>({ queryKey: containerBackupKeys.plansAll }, (current) =>
        (current ?? []).filter((plan) => plan.id !== id),
      );
      return { previous };
    },
    onError: (_error, _id, context) => {
      // Rollback the optimistic removal.
      if (!context) return;
      for (const [key, snapshot] of context.previous) {
        queryClient.setQueryData<ContainerBackupPlan[]>(key, snapshot);
      }
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: containerBackupKeys.plansAll });
    },
  });
}

export function useDeleteContainerBackupRun() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('containers');

  return useMutation({
    mutationFn: (id: string) => api.del(`/container-backups/runs/${id}`).then(assertSuccess),
    meta: { success: () => t('backups.toast.runDeleted') },
    onMutate: async (id) => {
      // Optimistic removal: drop the run from every cached query
      // so the table row disappears immediately. We snapshot the
      // prior state so onError can roll back if the server rejects
      // the delete (e.g. a dependency still references the run).
      await queryClient.cancelQueries({ queryKey: containerBackupKeys.runsAll });
      const previous = queryClient.getQueriesData<ContainerBackupRun[]>({
        queryKey: containerBackupKeys.runsAll,
      });
      queryClient.setQueriesData<ContainerBackupRun[]>({ queryKey: containerBackupKeys.runsAll }, (current) =>
        (current ?? []).filter((run) => run.id !== id),
      );
      return { previous };
    },
    onError: (_error, _id, context) => {
      if (!context) return;
      for (const [key, snapshot] of context.previous) {
        queryClient.setQueryData<ContainerBackupRun[]>(key, snapshot);
      }
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: containerBackupKeys.runsAll });
    },
  });
}

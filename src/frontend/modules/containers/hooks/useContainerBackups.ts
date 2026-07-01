// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQueries, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api, type ApiResponse } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
import { assertSuccess } from '@resources/utils/api-mutation';

export type ContainerBackupOption = {
  key: string;
  type: 'config' | 'image' | 'filesystem' | 'logs' | 'volume' | 'bind';
  label: string;
  description: string;
  default: boolean;
  required: boolean;
};

export type ContainerBackupOptions = {
  containerId: string;
  containerName: string;
  options: ContainerBackupOption[];
};

export type ContainerBackupPlan = {
  id: string;
  name: string;
  environmentId: string;
  containerId: string;
  containerName: string;
  storageLocationId?: string;
  storageLocationIds: string[];
  includeConfig: boolean;
  includeLogs: boolean;
  includeFilesystem: boolean;
  includeImage: boolean;
  selectedMounts: string[];
  logTailLines?: number;
  cron?: string;
  enabled: boolean;
  retentionCount: number;
  retentionDays: number;
  lastRunAt?: string;
  nextRunAt?: string;
  createdAt: string;
  updatedAt: string;
};

export type ContainerBackupRun = {
  id: string;
  planId?: string;
  operation: 'backup' | 'restore';
  sourceRunId?: string;
  environmentId: string;
  containerId: string;
  containerName?: string;
  status: 'running' | 'success' | 'failure' | 'cancelled';
  archivePath?: string;
  archiveSize: number;
  archiveEncryption?: string;
  archiveKeyId?: string;
  requiresSecretKey?: boolean;
  error?: string;
  progressStage?: string;
  progressMessage?: string;
  progressUpdatedAt?: string;
  destinations: ContainerBackupRunDestination[];
  startedAt: string;
  completedAt?: string;
  durationMs: number;
  createdAt: string;
  updatedAt: string;
};

export type ContainerBackupRunDestination = {
  id: string;
  runId: string;
  storageLocationId?: string;
  storageLocationName: string;
  locationType: string;
  status: 'uploading' | 'success' | 'failure';
  path: string;
  error?: string;
  uploadedAt?: string;
  bytesUploaded?: number;
  bytesTotal?: number;
  createdAt: string;
  updatedAt: string;
};

export type ContainerBackupRestoreInput = {
  id: string;
  secretKey?: string;
  restoreItems: string[];
};

export type ContainerBackupUploadRestoreInput = {
  file: File;
  secretKey?: string;
};

export type ContainerBackupRestoreResult = {
  runId: string;
  restored: string[];
};

export type ContainerBackupRestoreOption = {
  key: string;
  type: 'image' | 'filesystem' | 'mount';
  label: string;
  description: string;
  default: boolean;
  required: boolean;
};

export type ContainerBackupRestoreOptions = {
  runId: string;
  items: ContainerBackupRestoreOption[];
  message?: string;
};

export type ContainerBackupInput = {
  name: string;
  storageLocationId?: string;
  storageLocationIds: string[];
  includeConfig: boolean;
  includeLogs: boolean;
  includeFilesystem: boolean;
  includeImage: boolean;
  selectedMounts: string[];
  logTailLines?: number;
  cron?: string;
  enabled?: boolean;
  retentionCount: number;
  retentionDays: number;
};

function envQuery(envId?: string | null) {
  return envId ? `?env=${envId}` : '';
}

function restoreSecretCacheKey(secretKey?: string) {
  const value = secretKey?.trim();
  if (!value) return 'current';
  let hash = 0;
  for (let index = 0; index < value.length; index += 1) {
    hash = ((hash << 5) - hash + value.charCodeAt(index)) | 0;
  }
  return `secret:${value.length}:${hash}`;
}

export function containerBackupDownloadUrl(runId: string) {
  return `/api/container-backups/runs/${encodeURIComponent(runId)}/download`;
}

export function useContainerBackupOptions(containerId: string) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['container-backup-options', envId, containerId],
    queryFn: () =>
      api
        .get<ContainerBackupOptions>(`/containers/${containerId}/backups/options`, envId ? { env: envId } : {})
        .then((r) => r.data),
    enabled: !!containerId,
  });
}

export function useContainerBackupPlans(containerId: string) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['container-backup-plans', envId, containerId],
    queryFn: () =>
      api
        .get<ContainerBackupPlan[]>('/container-backups', envId ? { env: envId, containerId } : { containerId })
        .then((r) => r.data ?? []),
    enabled: !!containerId,
  });
}

// useAllContainerBackupPlans returns backup plans across all
// environments (or one environment if `envId` is provided). When
// `envId === ''` (the global "All environments" selection in the
// header) the hook fans out one request per environment in parallel
// via useQueries and aggregates the results; with a single envId it
// falls back to one request, matching the existing single-env hook.
export function useAllContainerBackupPlans(envId: string | undefined | null) {
  const environments = useEnvironmentStore((s) => s.environments);
  const envs =
    envId && envId.length > 0
      ? environments.filter((env) => env.id === envId)
      : environments;
  const queries = useQueries({
    queries: envs.map((env) => ({
      queryKey: ['container-backup-plans-all', env.id],
      queryFn: () =>
        api
          .get<ContainerBackupPlan[]>(
            `/container-backups?env=${encodeURIComponent(env.id)}`,
          )
          .then((r) => r.data ?? []),
      staleTime: 15_000,
    })),
    combine: (results) => {
      const plans: ContainerBackupPlan[] = [];
      for (const r of results) {
        if (r.data) plans.push(...r.data);
      }
      plans.sort((a, b) => a.name.localeCompare(b.name));
      const isLoading = results.some((r) => r.isLoading);
      return { data: plans, isLoading };
    },
  });
  return queries as { data: ContainerBackupPlan[]; isLoading: boolean };
}

export function useContainerBackupRuns(containerId: string) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['container-backup-runs', envId, containerId],
    queryFn: () =>
      api
        .get<ContainerBackupRun[]>('/container-backups/runs', envId ? { env: envId, containerId } : { containerId })
        .then((r) => r.data ?? []),
    enabled: !!containerId,
    // Poll while a backup is running OR while any destination is
    // uploading. The latter covers destination retry uploads that
    // run against a previously-completed run (so run.status is
    // 'success' / 'cancelled' / 'failure') — without this branch
    // the operator never sees byte progress for a retry because
    // the run status alone doesn't change.
    refetchInterval: (query) => {
      const data = query.state.data;
      if (!data) return false;
      const hasRunningRun = data.some((run) => run.status === 'running');
      const hasUploadingDestination = data.some((run) =>
        (run.destinations ?? []).some((destination) => destination.status === 'uploading'),
      );
      return hasRunningRun || hasUploadingDestination ? 2000 : false;
    },
  });
}

// useAllContainerBackupRuns returns recent runs across all
// environments (or one if `envId` is set). Like its plans sibling,
// polls while any run is uploading so live progress is visible.
export function useAllContainerBackupRuns(envId: string | undefined | null) {
  const environments = useEnvironmentStore((s) => s.environments);
  const envs =
    envId && envId.length > 0
      ? environments.filter((env) => env.id === envId)
      : environments;
  const queries = useQueries({
    queries: envs.map((env) => ({
      queryKey: ['container-backup-runs-all', env.id],
      queryFn: () =>
        api
          .get<ContainerBackupRun[]>(
            `/container-backups/runs?env=${encodeURIComponent(env.id)}`,
          )
          .then((r) => r.data ?? []),
      staleTime: 10_000,
      refetchInterval: (query: {
        state: { data?: ContainerBackupRun[] | undefined };
      }) => {
        const data = query.state.data;
        if (!data) return false;
        const hasRunning = data.some((run) => run.status === 'running');
        const hasUploading = data.some((run) =>
          (run.destinations ?? []).some((destination) => destination.status === 'uploading'),
        );
        return hasRunning || hasUploading ? 2000 : false;
      },
    })),
    combine: (results) => {
      const runs: ContainerBackupRun[] = [];
      for (const r of results) {
        if (r.data) runs.push(...r.data);
      }
      runs.sort(
        (a, b) =>
          new Date(b.startedAt).getTime() - new Date(a.startedAt).getTime(),
      );
      const isLoading = results.some((r) => r.isLoading);
      return { data: runs, isLoading };
    },
  });
  return queries as { data: ContainerBackupRun[]; isLoading: boolean };
}

export function useContainerBackupRun(containerId: string, runId?: string) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['container-backup-run', envId, containerId, runId],
    queryFn: () =>
      api
        .get<ContainerBackupRun>(
          `/container-backups/runs/${encodeURIComponent(runId ?? '')}`,
          envId ? { env: envId } : {},
        )
        .then((r) => r.data),
    enabled: !!containerId && !!runId,
    refetchInterval: (query) => (query.state.data?.status === 'running' ? 1000 : false),
  });
}

export function useRunContainerBackup(containerId: string) {
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);
  const { t } = useTranslation('containers');

  return useMutation({
    mutationFn: (body: ContainerBackupInput) =>
      api.post<ContainerBackupRun>(`/containers/${containerId}/backups/run${envQuery(envId)}`, body).then(assertSuccess),
    meta: { success: () => t('backups.toast.started') },
    onMutate: async () => {
      const queryKey = ['container-backup-runs', envId, containerId] as const;
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
      queryClient.invalidateQueries({ queryKey: ['container-backup-runs'] });
    },
    onError: (_error, _variables, context) => {
      if (!context) return;
      queryClient.setQueryData<ContainerBackupRun[]>(context.queryKey, (current = []) =>
        current.filter((run) => run.id !== context.optimisticRunId),
      );
      queryClient.invalidateQueries({ queryKey: ['container-backup-runs'] });
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
        .post<ContainerBackupPlan>(`/container-backups${envQuery(envId)}`, { ...body, containerId })
        .then(assertSuccess),
    meta: { success: () => t('backups.toast.scheduleCreated') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['container-backup-plans'] });
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
      queryClient.invalidateQueries({ queryKey: ['container-backup-plans'] });
    },
  });
}

export function useDeleteContainerBackupPlan() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('containers');

  return useMutation({
    mutationFn: (id: string) => api.del(`/container-backups/${id}`).then(assertSuccess),
    meta: { success: () => t('backups.toast.scheduleDeleted') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['container-backup-plans'] });
    },
  });
}

export function useDeleteContainerBackupRun() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('containers');

  return useMutation({
    mutationFn: (id: string) => api.del(`/container-backups/runs/${id}`).then(assertSuccess),
    meta: { success: () => t('backups.toast.runDeleted') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['container-backup-runs'] });
    },
  });
}

export function useCancelContainerBackupRun() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('containers');

  return useMutation({
    mutationFn: (id: string) =>
      api
        .post<ContainerBackupRun>(`/container-backups/runs/${id}/cancel`)
        .then(assertSuccess),
    meta: { success: () => t('backups.toast.runCancelled') },
    onSuccess: (run, id) => {
      // Optimistically reflect the cancelled state in the runs list and
      // the focused run query so the UI updates without waiting for the
      // next refetch.
      queryClient.setQueryData<ContainerBackupRun[]>(
        ['container-backup-runs'],
        (current) =>
          (current ?? []).map((entry) =>
            entry.id === id ? { ...entry, ...run } : entry,
          ),
      );
      queryClient.setQueryData<ContainerBackupRun>(
        ['container-backup-run', undefined, undefined, id],
        (current) => (current ? { ...current, ...run } : run),
      );
      queryClient.invalidateQueries({ queryKey: ['container-backup-runs'] });
      queryClient.invalidateQueries({ queryKey: ['container-backup-run'] });
    },
  });
}

export function useRunContainerBackupPlan() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('containers');

  return useMutation({
    mutationFn: (id: string) => api.post<ContainerBackupRun>(`/container-backups/${id}/run`).then(assertSuccess),
    meta: { success: () => t('backups.toast.started') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['container-backup-runs'] });
      queryClient.invalidateQueries({ queryKey: ['container-backup-plans'] });
    },
  });
}

export function useContainerBackupRestoreOptions(runId?: string, secretKey?: string, enabled = false) {
  return useQuery({
    queryKey: ['container-backup-restore-options', runId, restoreSecretCacheKey(secretKey)],
    queryFn: () =>
      api
        .post<ContainerBackupRestoreOptions>(
          `/container-backups/runs/${runId}/restore-options`,
          secretKey?.trim() ? { secretKey: secretKey.trim() } : {},
        )
        .then(assertSuccess),
    enabled: !!runId && enabled,
    retry: false,
  });
}

export function useRestoreContainerBackup(containerId: string) {
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);
  const { t } = useTranslation('containers');

  return useMutation({
    mutationFn: ({ id, secretKey, restoreItems }: ContainerBackupRestoreInput) =>
      api
        .post<ContainerBackupRun>(
          `/container-backups/runs/${id}/restore`,
          {
            ...(secretKey?.trim() ? { secretKey: secretKey.trim() } : {}),
            restoreItems,
          },
        )
        .then(assertSuccess),
    meta: { success: () => t('backups.toast.restoreStarted') },
    onSuccess: (run) => {
      const queryKey = ['container-backup-runs', envId, containerId] as const;
      queryClient.setQueryData<ContainerBackupRun[]>(queryKey, (current = []) => [
        run,
        ...current.filter((item) => item.id !== run.id),
      ]);
      queryClient.invalidateQueries({ queryKey: ['container-backup-runs'] });
    },
  });
}

export function useUploadRestoreContainerBackup(containerId: string) {
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);
  const { t } = useTranslation('containers');

  return useMutation({
    mutationFn: async ({ file, secretKey }: ContainerBackupUploadRestoreInput) => {
      const form = new FormData();
      form.append('file', file);
      if (secretKey?.trim()) {
        form.append('secretKey', secretKey.trim());
      }

      const stored = typeof window !== 'undefined'
        ? localStorage.getItem('mcharbor-language')
        : null;
      let lang = 'en';
      if (stored) {
        try {
          lang = JSON.parse(stored)?.state?.language || 'en';
        } catch {
          lang = stored;
        }
      }

      const response = await fetch(`/api/containers/${containerId}/backups/restore-upload${envQuery(envId)}`, {
        method: 'POST',
        credentials: 'include',
        headers: { 'Accept-Language': lang },
        body: form,
      });
      const payload = await response.json() as ApiResponse<ContainerBackupRestoreResult>;
      return assertSuccess(payload);
    },
    meta: { success: () => t('backups.toast.restored') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['container-backup-runs'] });
    },
  });
}

// Retry upload of the on-disk archive to one previously-failed
// destination. The HTTP call returns immediately with the destination
// in `uploading` state; the actual upload runs in the background.
// The runs list query is refetched on settle so the destination row
// (status, bytes_uploaded, bytes_total) updates through the regular
// polling loop — that's how the inline progress bar tracks bytes.
export function useRetryDestinationUpload() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('containers');

  return useMutation({
    mutationFn: ({ runId, destinationId }: { runId: string; destinationId: string }) =>
      api
        .post<ContainerBackupRunDestination>(
          `/container-backups/runs/${encodeURIComponent(runId)}/destinations/${encodeURIComponent(destinationId)}/retry-upload`,
        )
        .then(assertSuccess),
    meta: { success: () => t('backups.toast.retryUploadStarted') },
    onSuccess: () => {
      // Kick the runs query immediately so the destination row
      // flips to `uploading` and the polling loop picks up byte
      // progress without waiting for the next refetchInterval tick.
      queryClient.invalidateQueries({ queryKey: ['container-backup-runs'] });
      queryClient.invalidateQueries({ queryKey: ['container-backup-run'] });
    },
  });
}

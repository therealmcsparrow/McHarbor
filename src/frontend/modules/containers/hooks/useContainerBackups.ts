// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
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
  environmentId: string;
  containerId: string;
  status: 'running' | 'success' | 'failure';
  archivePath?: string;
  archiveSize: number;
  archiveEncryption?: string;
  archiveKeyId?: string;
  requiresSecretKey?: boolean;
  error?: string;
  startedAt: string;
  completedAt?: string;
  durationMs: number;
};

export type ContainerBackupRestoreInput = {
  id: string;
  secretKey?: string;
};

export type ContainerBackupRestoreResult = {
  runId: string;
  restored: string[];
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
  cron?: string;
  enabled?: boolean;
  retentionCount: number;
  retentionDays: number;
};

function envQuery(envId?: string | null) {
  return envId ? `?env=${envId}` : '';
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

export function useContainerBackupRuns(containerId: string) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['container-backup-runs', envId, containerId],
    queryFn: () =>
      api
        .get<ContainerBackupRun[]>('/container-backups/runs', envId ? { env: envId, containerId } : { containerId })
        .then((r) => r.data ?? []),
    enabled: !!containerId,
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
    onSuccess: () => {
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

export function useRestoreContainerBackup() {
  const { t } = useTranslation('containers');

  return useMutation({
    mutationFn: ({ id, secretKey }: ContainerBackupRestoreInput) =>
      api
        .post<ContainerBackupRestoreResult>(`/container-backups/runs/${id}/restore`, secretKey ? { secretKey } : {})
        .then(assertSuccess),
    meta: { success: () => t('backups.toast.restored') },
  });
}

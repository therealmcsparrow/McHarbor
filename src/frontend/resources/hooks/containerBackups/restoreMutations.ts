// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
import { assertSuccess } from '@resources/utils/api-mutation';
import type {
  ContainerBackupRun,
  ContainerBackupRunDestination,
  ContainerBackupRelinkAllResult,
  ContainerBackupRestoreInput,
  ContainerBackupUploadRestoreInput,
} from './types';
import { envQuery } from './shared';
import { postFormWithProgress } from './uploadWithProgress';

export function useCancelContainerBackupRun() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('containers');

  return useMutation({
    mutationFn: (id: string) =>
      api.post<ContainerBackupRun>(`/container-backups/runs/${id}/cancel`).then(assertSuccess),
    meta: { success: () => t('backups.toast.runCancelled') },
    onSuccess: (run, id) => {
      // Optimistically reflect the cancelled state in the runs list and
      // the focused run query so the UI updates without waiting for the
      // next refetch.
      queryClient.setQueryData<ContainerBackupRun[]>(['container-backup-runs'], (current) =>
        (current ?? []).map((entry) => (entry.id === id ? { ...entry, ...run } : entry)),
      );
      queryClient.setQueryData<ContainerBackupRun>(['container-backup-run', undefined, undefined, id], (current) =>
        current ? { ...current, ...run } : run,
      );
      void queryClient.invalidateQueries({ queryKey: ['container-backup-runs'] });
      void queryClient.invalidateQueries({ queryKey: ['container-backup-run'] });
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
      void queryClient.invalidateQueries({ queryKey: ['container-backup-runs'] });
      void queryClient.invalidateQueries({ queryKey: ['container-backup-plans'] });
    },
  });
}

export function useRestoreContainerBackup(containerId: string) {
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);
  const { t } = useTranslation('containers');

  return useMutation({
    mutationFn: ({ id, secretKey, restoreItems }: ContainerBackupRestoreInput) =>
      api
        .post<ContainerBackupRun>(`/container-backups/runs/${id}/restore`, {
          ...(secretKey?.trim() ? { secretKey: secretKey.trim() } : {}),
          restoreItems,
        })
        .then(assertSuccess),
    meta: { success: () => t('backups.toast.restoreStarted') },
    onSuccess: (run) => {
      const queryKey = ['container-backup-runs', envId, containerId] as const;
      queryClient.setQueryData<ContainerBackupRun[]>(queryKey, (current = []) => [
        run,
        ...current.filter((item) => item.id !== run.id),
      ]);
      void queryClient.invalidateQueries({ queryKey: ['container-backup-runs'] });
    },
  });
}

/**
 * Stores an uploaded archive as a backup run without restoring it; the caller hands the
 * returned run to the restore dialog, which reads its contents for a selective restore.
 */
export function useStageUploadedContainerBackup(containerId: string) {
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);

  return useMutation({
    mutationFn: ({ file, secretKey, onProgress }: ContainerBackupUploadRestoreInput) => {
      const form = new FormData();
      form.append('file', file);
      if (secretKey?.trim()) {
        form.append('secretKey', secretKey.trim());
      }
      return postFormWithProgress<ContainerBackupRun>(
        `/api/containers/${containerId}/backups/upload${envQuery(envId)}`,
        form,
        onProgress,
      );
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['container-backup-runs'] });
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
      void queryClient.invalidateQueries({ queryKey: ['container-backup-runs'] });
      void queryClient.invalidateQueries({ queryKey: ['container-backup-run'] });
    },
  });
}

// useRelinkAllContainerBackups runs a one-shot reconciliation
// pass against the orchestrator: every plan and run with a stale
// container_id gets its id refreshed to the live one. Designed
// for after a bulk container recreate (docker compose up). The
// list endpoints already auto-refresh on read, so the bulk action
// is mostly for batch-fixing rows the user hasn't loaded yet.
export function useRelinkAllContainerBackups() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('containers');

  return useMutation({
    mutationFn: () =>
      api.post<ContainerBackupRelinkAllResult>('/container-backups/admin/relink-all').then(assertSuccess),
    meta: {
      success: (result: ContainerBackupRelinkAllResult | undefined) => {
        const refreshed = (result?.plansRefreshed ?? 0) + (result?.runsRefreshed ?? 0);
        return t('backups.toast.relinkAllSuccess', { count: refreshed });
      },
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['container-backup-plans'] });
      void queryClient.invalidateQueries({ queryKey: ['container-backup-runs'] });
    },
  });
}

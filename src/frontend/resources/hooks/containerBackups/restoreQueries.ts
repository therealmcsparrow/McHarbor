// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';

import { api } from '@core/api/client';

import { assertSuccess } from '@resources/utils/api-mutation';
import type { ContainerBackupRestoreOptions } from './types';
import { restoreSecretCacheKey } from './shared';

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

// useContainerBackupRestoreFilesystemEntries returns the first-level
// children of `innerPath` inside the container filesystem layer of a
// backup archive. Drives the restore dialog's per-file/per-folder
// tree when the operator disables the "Full restore" toggle.
export function useContainerBackupRestoreFilesystemEntries(
  runId?: string,
  innerPath = '',
  secretKey?: string,
  options: { enabled?: boolean } = {},
) {
  return useQuery({
    queryKey: ['container-backup-restore-filesystem-entries', runId, innerPath, restoreSecretCacheKey(secretKey)],
    queryFn: () =>
      api
        .post<ContainerBackupRestoreOptions>(`/container-backups/runs/${runId}/restore-filesystem-entries`, {
          ...(secretKey?.trim() ? { secretKey: secretKey.trim() } : {}),
          innerPath,
        })
        .then(assertSuccess),
    enabled: !!runId && (options.enabled ?? true),
    retry: false,
  });
}

// useContainerBackupRestoreMountEntries returns the first-level
// children of `innerPath` inside `mounts/<mountTarget>.tar`. The
// mountTarget must match a source-mount path the archive actually
// contains; a 404 maps to an inline error in the picker.
export function useContainerBackupRestoreMountEntries(
  runId?: string,
  mountTarget?: string,
  innerPath = '',
  secretKey?: string,
  options: { enabled?: boolean } = {},
) {
  return useQuery({
    queryKey: [
      'container-backup-restore-mount-entries',
      runId,
      mountTarget,
      innerPath,
      restoreSecretCacheKey(secretKey),
    ],
    queryFn: () =>
      api
        .post<ContainerBackupRestoreOptions>(`/container-backups/runs/${runId}/restore-mount-entries`, {
          ...(secretKey?.trim() ? { secretKey: secretKey.trim() } : {}),
          mountTarget,
          innerPath,
        })
        .then(assertSuccess),
    enabled: !!runId && !!mountTarget && (options.enabled ?? true),
    retry: false,
  });
}

// useContainerBackupRestoreConfigEntries returns the synthetic
// config tree children under `parentKey` (empty for the root).
// Drives the per-key config picker when the operator disables the
// "Full config" toggle.
export function useContainerBackupRestoreConfigEntries(
  runId?: string,
  parentKey = '',
  secretKey?: string,
  options: { enabled?: boolean } = {},
) {
  return useQuery({
    queryKey: ['container-backup-restore-config-entries', runId, parentKey, restoreSecretCacheKey(secretKey)],
    queryFn: () =>
      api
        .post<ContainerBackupRestoreOptions>(`/container-backups/runs/${runId}/restore-config-entries`, {
          ...(secretKey?.trim() ? { secretKey: secretKey.trim() } : {}),
          parentKey,
        })
        .then(assertSuccess),
    enabled: !!runId && (options.enabled ?? true),
    retry: false,
  });
}

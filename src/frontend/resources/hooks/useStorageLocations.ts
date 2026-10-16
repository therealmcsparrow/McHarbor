// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { assertRestartScheduled, runWithMcHarborRestart } from '@resources/stores/mcharbor-restart';
import type {
  StorageLocation,
  StorageLocationInput,
  StorageOAuthAuthorizeResponse,
  StorageLocationBackupMigrationResult,
  StorageLocationTestResult,
  StorageLocationSecrets,
  BackupEncryptionKeyResponse,
  BackupEncryptionKeyInstallResponse,
  BackupEncryptionKeyStatus,
  InstallBackupKeyInput,
  StorageLocationUsage,
} from './storageLocationTypes';

export type {
  StorageLocationType,
  StorageLocation,
  StorageLocationInput,
  StorageOAuthAuthorizeResponse,
  StorageLocationBackupMigrationResult,
  StorageLocationTestStep,
  StorageLocationTestResult,
  StorageLocationSecrets,
  BackupEncryptionKeyResponse,
  BackupEncryptionKeyInstallResponse,
  BackupEncryptionKeyStatus,
  InstallBackupKeyInput,
  StorageLocationUsage,
} from './storageLocationTypes';

/**
 * useStorageLocationSecrets fetches the decrypted credential
 * columns for a single storage location. The fetch is lazy and
 * gated by the `enabled` flag so the dialog only requests secrets
 * when it's actually opening an existing destination.
 */
export function useStorageLocationSecrets(id: string | undefined, enabled: boolean) {
  return useQuery({
    queryKey: ['storageLocationSecrets', id],
    enabled: enabled && !!id,
    queryFn: () =>
      api.get<StorageLocationSecrets>(`/storage-locations/${id}/secrets`).then((r) => assertSuccess(r) ?? {}),
    // Secrets are sensitive — never persist to disk or survive
    // across the cache TTL. The hook is GC'd when the dialog closes.
    staleTime: 30_000,
    gcTime: 0,
    retry: false,
  });
}

export function useStorageLocations() {
  return useQuery({
    queryKey: ['storageLocationsList'],
    queryFn: () => api.get<StorageLocation[]>('/storage-locations').then((r) => assertSuccess(r) ?? []),
  });
}

export function useCreateStorageLocation() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');

  return useMutation({
    mutationFn: (body: StorageLocationInput) =>
      api.post<StorageLocation>('/storage-locations', body).then(assertSuccess),
    meta: { success: () => t('toast.storageLocationCreated') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['storageLocationsList'] });
    },
  });
}

export function useUpdateStorageLocation() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');

  return useMutation({
    mutationFn: ({ id, ...body }: { id: string } & Partial<StorageLocationInput>) =>
      api.put<StorageLocation>(`/storage-locations/${id}`, body).then(assertSuccess),
    meta: { success: () => t('toast.storageLocationUpdated') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['storageLocationsList'] });
    },
  });
}

export function useDeleteStorageLocation() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');

  return useMutation({
    mutationFn: (id: string) => api.del(`/storage-locations/${id}`).then(assertSuccess),
    meta: { success: () => t('toast.storageLocationDeleted') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['storageLocationsList'] });
    },
  });
}

export function useAuthorizeStorageLocation() {
  const { t } = useTranslation('settings');

  return useMutation({
    mutationFn: (id: string) =>
      api.post<StorageOAuthAuthorizeResponse>(`/storage-locations/${id}/oauth/authorize`).then(assertSuccess),
    meta: { error: t('toast.storageConsentFailed') },
  });
}

export function useMigrateStorageLocationBackups() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');

  return useMutation({
    mutationFn: (id: string) =>
      api
        .post<StorageLocationBackupMigrationResult>(`/storage-locations/${id}/migrate-container-backups`)
        .then(assertSuccess),
    meta: {
      success: (data: unknown) => {
        const result = data as StorageLocationBackupMigrationResult;
        return t('toast.storageBackupsMigrated', {
          migrated: result.migrated,
          skipped: result.skipped,
          failed: result.failed,
        });
      },
      error: t('toast.storageBackupsMigrationFailed'),
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['container-backup-runs'] });
    },
  });
}

// useTestStorageLocation hits POST /storage-locations/{id}/test
// and returns the per-step report (write / change / delete for
// local; TCP-probe + credential-decrypt for cloud / network
// storage). The result is always returned to the caller; the
// caller (the card) shows a dialog with the steps and overall
// status. We intentionally don't toast on success or failure —
// the dialog is the source of truth because the operator needs
// the per-step breakdown, not a single yes/no.
export function useTestStorageLocation() {
  return useMutation({
    mutationFn: (id: string) =>
      api.post<StorageLocationTestResult>(`/storage-locations/${id}/test`).then(assertSuccess),
  });
}

export function useGenerateBackupEncryptionKey() {
  const { t } = useTranslation('settings');

  return useMutation({
    mutationFn: () => api.post<BackupEncryptionKeyResponse>('/settings/backup-key/generate').then(assertSuccess),
    meta: { success: () => t('toast.backupKeyGenerated') },
  });
}

export function useBackupEncryptionKeyStatus() {
  return useQuery({
    queryKey: ['backup-encryption-key-status'],
    queryFn: () => api.get<BackupEncryptionKeyStatus>('/settings/backup-key/status').then(assertSuccess),
  });
}

export function useInstallBackupEncryptionKey() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');

  return useMutation({
    mutationFn: (input: InstallBackupKeyInput) =>
      runWithMcHarborRestart(() =>
        api
          .post<BackupEncryptionKeyInstallResponse>('/settings/backup-key/install', input)
          .then((res) => assertRestartScheduled(res, t('system.restartDialog.notSupervised', { ns: 'common' }))),
      ),
    meta: {
      success: () => t('toast.backupKeyInstalled'),
      error: t('toast.backupKeyInstallFailed'),
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: ['backup-encryption-key-status'],
      });
    },
  });
}

// useStorageLocationUsage reads GET /storage-locations/{id}/usage
// for the card's used-of-total bar. Capacity moves slowly, so the
// value is cached for a minute and refreshed in the background.
export function useStorageLocationUsage(id: string, enabled = true) {
  return useQuery({
    queryKey: ['storageLocationUsage', id],
    enabled: enabled && !!id,
    queryFn: () =>
      api.get<StorageLocationUsage>(`/storage-locations/${id}/usage`).then((r) => assertSuccess(r) ?? null),
    staleTime: 60_000,
    retry: false,
  });
}

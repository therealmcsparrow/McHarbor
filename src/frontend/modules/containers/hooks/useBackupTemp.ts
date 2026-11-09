// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import i18n from 'i18next';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { PERM } from '@core/auth/permissionKeys';
import { usePermission } from '@core/auth/usePermission';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { BackupTempAbout } from '../components/backupTempRemedy';

export type BackupTempStatus = {
  path: string;
  source: 'env' | 'setting' | 'default';
  settingPath: string;
  envOverride: boolean;
  envInvalidCode: string;
  defaultPath: string;
  sizeBytes: number;
  entries: number;
  activeRuns: number;
  retiredPaths: string[];
};

export type BackupTempCleanupResult = {
  removedEntries: number;
  removedBytes: number;
  keptOwned: number;
  keptRecent: number;
  activeRuns: number;
};

const SOURCES = ['env', 'setting', 'default'];

function isRecord(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v);
}

function isCount(v: unknown): v is number {
  return typeof v === 'number' && Number.isFinite(v);
}

export function isBackupTempStatus(v: unknown): v is BackupTempStatus {
  return (
    isRecord(v) &&
    typeof v.path === 'string' &&
    typeof v.source === 'string' &&
    SOURCES.includes(v.source) &&
    typeof v.settingPath === 'string' &&
    typeof v.envOverride === 'boolean' &&
    typeof v.envInvalidCode === 'string' &&
    typeof v.defaultPath === 'string' &&
    isCount(v.sizeBytes) &&
    isCount(v.entries) &&
    isCount(v.activeRuns) &&
    Array.isArray(v.retiredPaths) &&
    v.retiredPaths.every((p) => typeof p === 'string')
  );
}

export function isBackupTempCleanupResult(v: unknown): v is BackupTempCleanupResult {
  return (
    isRecord(v) &&
    isCount(v.removedEntries) &&
    isCount(v.removedBytes) &&
    isCount(v.keptOwned) &&
    isCount(v.keptRecent) &&
    isCount(v.activeRuns)
  );
}

function parseStatus(v: unknown): BackupTempStatus {
  if (!isBackupTempStatus(v)) throw new Error(i18n.t('backupTemp.reply.malformedStatus', { ns: 'containers' }));
  return v;
}

function parseCleanup(v: unknown): BackupTempCleanupResult {
  if (!isBackupTempCleanupResult(v)) throw new Error(i18n.t('backupTemp.reply.malformedCleanup', { ns: 'containers' }));
  return v;
}

const STATUS_KEY = ['container-backups', 'temp'] as const;

/** Reads the temp location only for a user holding the key, because the route needs a global `backups.manage`. */
export function useBackupTempStatus() {
  const allowed = usePermission(PERM.BACKUPS_MANAGE);
  return useQuery({
    queryKey: STATUS_KEY,
    queryFn: () => api.get<unknown>('/container-backups/temp').then(assertSuccess).then(parseStatus),
    enabled: allowed,
    retry: false,
  });
}

/**
 * Shares the footer's `['about']` query. Stays undefined until loaded so the remedy fails closed
 * instead of guessing the install kind.
 */
export function useBackupTempAbout(): BackupTempAbout | undefined {
  const { data } = useQuery({
    queryKey: ['about'],
    queryFn: () => api.get<{ version: string; native?: boolean; sandboxed?: boolean }>('/about').then(assertSuccess),
    staleTime: 60_000,
  });
  if (!data) return undefined;
  return { native: data.native === true, sandboxed: data.sandboxed === true };
}

export function useSaveBackupTemp() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('containers');
  return useMutation({
    mutationFn: (path: string) => api.put<unknown>('/container-backups/temp', { path }).then(assertSuccess).then(parseStatus),
    meta: { error: t('backupTemp.toast.saveFailed'), success: t('backupTemp.toast.saved') },
    onSuccess: (saved) => {
      queryClient.setQueryData(STATUS_KEY, saved);
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: STATUS_KEY });
    },
  });
}

export function useBackupTempCleanup() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('containers');
  return useMutation({
    mutationFn: () => api.post<unknown>('/container-backups/temp/cleanup').then(assertSuccess).then(parseCleanup),
    meta: { error: t('backupTemp.toast.cleanupFailed') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: STATUS_KEY });
    },
  });
}

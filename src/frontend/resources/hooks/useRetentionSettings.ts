// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { RetentionSettingsData } from '@resources/types/settings';

export type RetentionSettingsInput = Omit<RetentionSettingsData, 'auditRetentionPresets'>;

export type PurgeResponse = {
  deleted: number;
  retentionDays: number;
  vacuuming: boolean;
};

/** The API-key purge reports two counts, so it has its own shape. */
export type APIKeyUsagePurgeResult = {
  eventsDeleted: number;
  tombstonesDeleted: number;
  retentionDays: number;
};

export function useRetentionSettings() {
  return useQuery({
    queryKey: ['settings', 'retention'],
    queryFn: () => api.get<RetentionSettingsData>('/settings/retention').then(assertSuccess),
  });
}

export function useSaveRetentionSettings() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');
  return useMutation({
    mutationFn: (data: RetentionSettingsInput) => api.put('/settings/retention', data).then(assertSuccess),
    meta: { success: t('toast.retentionUpdated') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['settings', 'retention'] });
    },
  });
}

export function usePurgeAuditLog() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');
  return useMutation({
    mutationFn: () => api.del<PurgeResponse>('/audit', { vacuum: 'true' }).then(assertSuccess),
    meta: {
      success: (data: PurgeResponse | undefined) => {
        if (data?.vacuuming) {
          return t('toast.auditLogPurgedVacuuming', { count: data.deleted ?? 0 });
        }
        return t('toast.auditLogPurged', { count: data?.deleted ?? 0 });
      },
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['audit-logs'] });
    },
  });
}

export function usePurgeActivityLog() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');
  return useMutation({
    mutationFn: () => api.del<PurgeResponse>('/activity', { vacuum: 'true' }).then(assertSuccess),
    meta: {
      success: (data: PurgeResponse | undefined) => {
        if (data?.vacuuming) {
          return t('toast.activityLogPurgedVacuuming', { count: data.deleted ?? 0 });
        }
        return t('toast.activityLogPurged', { count: data?.deleted ?? 0 });
      },
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['events'] });
    },
  });
}

export function usePurgeLifecycleLog() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');
  return useMutation({
    mutationFn: () => api.del<PurgeResponse>('/lifecycle', { vacuum: 'true' }).then(assertSuccess),
    meta: {
      success: (data: PurgeResponse | undefined) => {
        if (data?.vacuuming) {
          return t('toast.lifecycleLogPurgedVacuuming', { count: data.deleted ?? 0 });
        }
        return t('toast.lifecycleLogPurged', { count: data?.deleted ?? 0 });
      },
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['lifecycle'] });
    },
  });
}

export function usePurgeBackupLog() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');
  return useMutation({
    mutationFn: () => api.del<PurgeResponse>('/backup-logs', { vacuum: 'true' }).then(assertSuccess),
    meta: {
      success: (data: PurgeResponse | undefined) => {
        if (data?.vacuuming) {
          return t('toast.backupLogPurgedVacuuming', { count: data.deleted ?? 0 });
        }
        return t('toast.backupLogPurged', { count: data?.deleted ?? 0 });
      },
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['backup-logs'] });
    },
  });
}

/**
 * Clears every API-key usage event and every deleted-key tombstone.
 *
 * Unlike the other purges this one can make rows vanish from a list the user
 * may be looking at — deleted keys are shown until retention removes them —
 * so the API-key queries are invalidated alongside the usage ones. Live and
 * revoked credentials are untouched.
 */
export function usePurgeAPIKeyUsage() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');
  return useMutation({
    mutationFn: () => api.del<APIKeyUsagePurgeResult>('/api-keys/usage').then(assertSuccess),
    meta: {
      success: (data: APIKeyUsagePurgeResult | undefined) =>
        t('toast.apiKeyUsagePurged', { count: data?.eventsDeleted ?? 0 }),
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['api-keys'] });
    },
  });
}

export function usePurgeScanHistory() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');
  return useMutation({
    mutationFn: () => api.del<PurgeResponse>('/scans', { vacuum: 'true' }).then(assertSuccess),
    meta: {
      success: (data: PurgeResponse | undefined) => {
        if (data?.vacuuming) {
          return t('toast.scanHistoryPurgedVacuuming', { count: data.deleted ?? 0 });
        }
        return t('toast.scanHistoryPurged', { count: data?.deleted ?? 0 });
      },
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['scans'] });
    },
  });
}

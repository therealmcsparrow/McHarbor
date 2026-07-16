// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api, type PaginatedData } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export type VersionCheck = {
  currentVersion: string;
  latestVersion: string;
  updateAvailable: boolean;
  releaseUrl?: string;
  publishedAt?: string;
  releaseNotes?: string;
};

export type SelfUpdateState = VersionCheck & {
  lastCheckedAt?: string;
  lastError?: string;
  nextCheckAt?: string;
  intervalHours: number;
  notifiedVersion?: string;
};

export type SelfUpdateSettings = {
  intervalHours: number;
  channelIds: string[];
  lastSeenVersion: string;
  enabled: boolean;
};

export type NotificationChannel = {
  id: string;
  name: string;
  type: string;
  enabled: boolean;
};

export type UpdatePolicy = {
  id: string;
  name: string;
  containerMatch: string;
  imageMatch: string;
  schedule: string;
  strategy: string;
  autoRestart: boolean;
  enabled: boolean;
  lastRunAt: string;
  lastRunStatus: string;
  createdAt: string;
  updatedAt: string;
};

export type CreatePolicyInput = {
  name: string;
  containerMatch: string;
  imageMatch: string;
  schedule: string;
  strategy: string;
  autoRestart: boolean;
};

export type UpdatePolicyInput = {
  name?: string;
  containerMatch?: string;
  imageMatch?: string;
  schedule?: string;
  strategy?: string;
  autoRestart?: boolean;
  enabled?: boolean;
};

export function useCheckUpdate() {
  return useQuery({
    queryKey: ['updates', 'check'],
    queryFn: async () => {
      const res = await api.get<VersionCheck>('/updates/check');
      if (!res.success || !res.data) {
        throw new Error(res.error ?? 'Update check failed');
      }
      return res.data;
    },
    staleTime: 5 * 60_000,
    enabled: false,
  });
}

// useSelfUpdateState polls the cached self-update result. The
// background checker on the server keeps the result fresh, so the
// frontend just reads it. A short polling interval lets the in-app
// banner react within a minute of a new release appearing.
export function useSelfUpdateState(intervalMs = 60_000) {
  return useQuery({
    queryKey: ['updates', 'self'],
    queryFn: async () => {
      const res = await api.get<SelfUpdateState>('/updates/state');
      if (!res.success || !res.data) {
        throw new Error(res.error ?? 'Update state unavailable');
      }
      return res.data;
    },
    refetchInterval: intervalMs,
    refetchOnWindowFocus: true,
    staleTime: 30_000,
  });
}

export function useSelfUpdateSettings() {
  return useQuery({
    queryKey: ['updates', 'self', 'settings'],
    queryFn: async () => {
      const res = await api.get<SelfUpdateSettings>('/updates/settings');
      if (!res.success || !res.data) {
        throw new Error(res.error ?? 'Update settings unavailable');
      }
      return res.data;
    },
  });
}

export function useSaveSelfUpdateSettings() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');
  return useMutation({
    mutationFn: (input: SelfUpdateSettings) =>
      api.put('/updates/settings', input).then(assertSuccess),
    meta: { success: t('toast.updateCheckSettingsSaved') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['updates', 'self'] });
    },
  });
}

export function useDismissUpdate() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');
  return useMutation({
    mutationFn: (version: string) =>
      api.post('/updates/dismiss', { version }).then(assertSuccess),
    meta: { success: t('toast.updateCheckDismissed') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['updates', 'self'] });
    },
  });
}

export function useUpdatePolicies() {
  return useQuery({
    queryKey: ['updates', 'policies'],
    queryFn: () =>
      api
        .get<PaginatedData<UpdatePolicy>>('/updates')
        .then((r) => r.data?.items ?? []),
  });
}

export function useCreateUpdatePolicy() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');
  return useMutation({
    mutationFn: (input: CreatePolicyInput) =>
      api.post('/updates', input).then(assertSuccess),
    meta: { success: t('toast.policyCreated') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['updates', 'policies'] });
    },
  });
}

export function useUpdateUpdatePolicy() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');
  return useMutation({
    mutationFn: ({ id, ...input }: UpdatePolicyInput & { id: string }) =>
      api.put(`/updates/${id}`, input).then(assertSuccess),
    meta: { success: t('toast.policyUpdated') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['updates', 'policies'] });
    },
  });
}

export function useDeleteUpdatePolicy() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');
  return useMutation({
    mutationFn: (id: string) => api.del(`/updates/${id}`).then(assertSuccess),
    meta: { success: t('toast.policyDeleted') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['updates', 'policies'] });
    },
  });
}

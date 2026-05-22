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
    queryFn: () => api.get<VersionCheck>('/updates/check').then((r) => r.data),
    staleTime: 5 * 60_000,
    enabled: false,
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

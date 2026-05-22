// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api, type PaginatedData } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export type Registry = {
  id: string;
  name: string;
  url: string;
  username: string;
  isDefault: boolean;
  createdAt: string;
  updatedAt: string;
};

export type CreateRegistryInput = {
  name: string;
  url: string;
  username: string;
  password: string;
};

export type UpdateRegistryInput = {
  name?: string;
  url?: string;
  username?: string;
  password?: string;
  isDefault?: boolean;
};

export type ConnectionTestResult = {
  success: boolean;
  message: string;
  latencyMs: number;
};

export function useRegistries() {
  return useQuery({
    queryKey: ['registries'],
    queryFn: () =>
      api
        .get<PaginatedData<Registry>>('/registries')
        .then((r) => r.data),
    refetchInterval: 30_000,
  });
}

export function useCreateRegistry() {
  const { t } = useTranslation('docker');
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (input: CreateRegistryInput) =>
      api.post<Registry>('/registries', input).then(assertSuccess),
    meta: { success: t('registries.toast.created') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['registries'] });
    },
  });
}

export function useUpdateRegistry() {
  const { t } = useTranslation('docker');
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, input }: { id: string; input: UpdateRegistryInput }) =>
      api.put<Registry>(`/registries/${id}`, input).then(assertSuccess),
    meta: { success: t('registries.toast.updated') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['registries'] });
    },
  });
}

export function useDeleteRegistry() {
  const { t } = useTranslation('docker');
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: string) =>
      api.del(`/registries/${id}`).then(assertSuccess),
    meta: { success: t('registries.toast.deleted') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['registries'] });
    },
  });
}

export function useTestRegistry() {
  const { t } = useTranslation('docker');

  return useMutation({
    mutationFn: (id: string) =>
      api.post<ConnectionTestResult>(`/registries/${id}/test`).then(assertSuccess),
    meta: { success: t('registries.toast.testSuccess') },
  });
}

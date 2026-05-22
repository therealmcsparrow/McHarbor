// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export type Role = {
  id: string;
  name: string;
  description: string;
  permissions: string[];
  isSystem: boolean;
  createdAt: string;
  updatedAt: string;
};

export function useRoles() {
  return useQuery({
    queryKey: ['roles'],
    queryFn: () => api.get<Role[]>('/roles').then((r) => r.data ?? []),
  });
}

export function useRole(id: string) {
  return useQuery({
    queryKey: ['roles', id],
    queryFn: () => api.get<Role>(`/roles/${id}`).then((r) => r.data),
    enabled: !!id,
  });
}

export function useAvailablePermissions() {
  return useQuery({
    queryKey: ['permissions'],
    queryFn: () => api.get<string[]>('/roles/permissions').then((r) => r.data ?? []),
  });
}

export function useCreateRole() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('security');

  return useMutation({
    mutationFn: (body: { name: string; description: string; permissions: string[] }) =>
      api.post<Role>('/roles', body).then(assertSuccess),
    meta: { success: () => t('toast.roleCreated') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['roles'] });
    },
  });
}

export function useUpdateRole() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('security');

  return useMutation({
    mutationFn: ({ id, ...body }: { id: string; name?: string; description?: string; permissions?: string[] }) =>
      api.put<Role>(`/roles/${id}`, body).then(assertSuccess),
    meta: { success: () => t('toast.roleUpdated') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['roles'] });
    },
  });
}

export function useDeleteRole() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('security');

  return useMutation({
    mutationFn: (id: string) => api.del(`/roles/${id}`).then(assertSuccess),
    meta: { success: () => t('toast.roleDeleted') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['roles'] });
    },
  });
}

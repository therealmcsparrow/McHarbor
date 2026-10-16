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
  userCount?: number;
  groupCount?: number;
  apiKeyCount?: number;
};

export function useRoles() {
  return useQuery({
    queryKey: ['roles'],
    queryFn: () => api.get<Role[]>('/roles').then((r) => r.data ?? []),
  });
}

// Phase 5 of the RBAC roadmap: predefined permission sets. The UI
// uses these to seed a new custom role's permission list. The data
// itself is server-side seed (migration 102_role_presets.sql).
export type RolePreset = {
  key: string;
  name: string;
  description: string;
  permissions: string[];
};

export function useRolePresets() {
  return useQuery({
    queryKey: ['roles', 'presets'],
    queryFn: () => api.get<{ presets: RolePreset[] }>('/roles/presets').then((r) => r.data?.presets ?? []),
    staleTime: 5 * 60_000,
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
      void queryClient.invalidateQueries({ queryKey: ['roles'] });
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
      void queryClient.invalidateQueries({ queryKey: ['roles'] });
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
      void queryClient.invalidateQueries({ queryKey: ['roles'] });
    },
  });
}

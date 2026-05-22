// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api, type PaginatedData } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export type UserItem = {
  id: string;
  username: string;
  displayName: string | null;
  email: string | null;
  role: string;
  isActive: boolean;
  lastLogin: string | null;
  createdAt: string;
  updatedAt: string;
};

export type UserRole = {
  id: string;
  roleId: string;
  roleName: string;
  environmentId: string | null;
  environmentName: string | null;
  stackName: string | null;
};

export type UserGroup = {
  id: string;
  groupId: string;
  groupName: string;
  memberCount: number;
  isSystem: boolean;
};

export function useUsers() {
  return useQuery({
    queryKey: ['users'],
    queryFn: () =>
      api
        .get<PaginatedData<UserItem>>('/users', { per_page: '100' })
        .then((r) => r.data?.items ?? []),
  });
}

export function useUserGroups(userId: string) {
  return useQuery({
    queryKey: ['user-groups', userId],
    queryFn: () => api.get<UserGroup[]>(`/users/${userId}/groups`).then((r) => r.data ?? []),
    enabled: !!userId,
  });
}

export function useUserRoles(userId: string) {
  return useQuery({
    queryKey: ['user-roles', userId],
    queryFn: () => api.get<UserRole[]>(`/users/${userId}/roles`).then((r) => r.data ?? []),
    enabled: !!userId,
  });
}

export function useAssignUserRole() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('security');

  return useMutation({
    mutationFn: ({
      userId,
      roleId,
      environmentId,
      stackNames,
    }: {
      userId: string;
      roleId: string;
      environmentId?: string;
      stackNames?: string[];
    }) => {
      const scopes = stackNames && stackNames.length > 0 ? stackNames : [null];
      return Promise.all(
        scopes.map((stackName) =>
          api.post(`/users/${userId}/roles`, {
            roleId,
            environmentId: environmentId || null,
            stackName,
          }).then(assertSuccess),
        ),
      );
    },
    meta: { success: () => t('toast.roleAssigned') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['user-roles'] });
    },
  });
}

export function useUnassignUserRole() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('security');

  return useMutation({
    mutationFn: ({ userId, assignmentIds }: { userId: string; assignmentIds: string[] }) =>
      Promise.all(
        assignmentIds.map((assignmentId) =>
          api.del(`/users/${userId}/roles/${assignmentId}`).then(assertSuccess),
        ),
      ),
    meta: { success: () => t('toast.roleUnassigned') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['user-roles'] });
    },
  });
}

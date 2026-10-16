// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api, type PaginatedData } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { userKeys } from './userKeys';

export type UserItem = {
  id: string;
  username: string;
  displayName: string | null;
  email: string | null;
  role: string;
  isActive: boolean;
  lastLogin: string | null;
  /**
   * `password_locked_until`. Set while the account is locked out after too
   * many failed sign-ins — which is a different thing from `isActive`
   * being false, and the table draws them differently.
   */
  lockedUntil: string | null;
  createdAt: string;
  updatedAt: string;
  roleNames?: string[];
  groupNames?: string[];
  kind?: PrincipalKind;
};

export type PrincipalKind = 'user' | 'service';

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

export type CreateUserInput = {
  username: string;
  password?: string;
  displayName?: string;
  email?: string;
  roleId?: string;
  isActive: boolean;
  kind?: PrincipalKind;
};

export function useUsers() {
  return useQuery({
    queryKey: userKeys.all,
    queryFn: () => api.get<PaginatedData<UserItem>>('/users', { per_page: '100' }).then((r) => r.data?.items ?? []),
  });
}

/** One user, for the detail page. Shares the `users` key prefix so every
 *  existing `invalidateQueries({ queryKey: ['users'] })` refreshes it too. */
export function useUser(userId: string | undefined) {
  return useQuery({
    queryKey: userKeys.detail(userId),
    queryFn: () => api.get<UserItem>(`/users/${userId}`).then((r) => r.data ?? null),
    enabled: !!userId,
  });
}

export function useCreateUser() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('security');

  return useMutation({
    mutationFn: (body: CreateUserInput) => api.post<UserItem>('/users', body).then(assertSuccess),
    meta: { success: () => t('toast.userCreated') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: userKeys.all });
    },
  });
}

export function useUserGroups(userId: string) {
  return useQuery({
    queryKey: userKeys.groups(userId),
    queryFn: () => api.get<UserGroup[]>(`/users/${userId}/groups`).then((r) => r.data ?? []),
    enabled: !!userId,
  });
}

export function useUserRoles(userId: string) {
  return useQuery({
    queryKey: userKeys.roles(userId),
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
          api
            .post(`/users/${userId}/roles`, {
              roleId,
              environmentId: environmentId || null,
              stackName,
            })
            .then(assertSuccess),
        ),
      );
    },
    meta: { success: () => t('toast.roleAssigned') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: userKeys.rolesAll });
    },
  });
}

export function useUnassignUserRole() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('security');

  return useMutation({
    mutationFn: ({ userId, assignmentIds }: { userId: string; assignmentIds: string[] }) =>
      Promise.all(
        assignmentIds.map((assignmentId) => api.del(`/users/${userId}/roles/${assignmentId}`).then(assertSuccess)),
      ),
    meta: { success: () => t('toast.roleUnassigned') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: userKeys.rolesAll });
    },
  });
}

// Phase 2: per-user grant/deny permission overrides.
// user_permissions (migrations/070_rbac_v2.sql) stores entries; the
// EffectivePermissions call merges role grants with these rows. This
// hook exposes the CRUD surface to the UI.
export type UserPermissionOverride = {
  id: string;
  permissionKey: string;
  effect: 'grant' | 'deny';
  environmentId: string | null;
  stackName: string | null;
  resourceType: string | null;
  resourceId: string | null;
  grantedBy: string;
  expiresAt: string | null;
  reason: string | null;
  createdAt: string;
  updatedAt: string;
};

export function useUserOverrides(userId: string | null | undefined) {
  return useQuery({
    queryKey: userKeys.overrides(userId),
    queryFn: () =>
      api
        .get<{ overrides: UserPermissionOverride[] }>(`/users/${userId}/overrides`)
        .then((r) => r.data?.overrides ?? []),
    enabled: !!userId,
  });
}

export function useSetUserOverrides() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('security');
  return useMutation({
    mutationFn: ({ userId, overrides }: { userId: string; overrides: UserPermissionOverride[] }) =>
      api.put<{ added: number; removed: number }>(`/users/${userId}/overrides`, { overrides }).then(assertSuccess),
    meta: { success: () => t('toast.overridesUpdated') },
    onSuccess: (_data, vars) => {
      void queryClient.invalidateQueries({ queryKey: userKeys.overrides(vars.userId) });
      // The override set can change the effective permission list,
      // so invalidate the /me/permissions query too.
      void queryClient.invalidateQueries({ queryKey: ['auth', 'me', 'permissions'] });
    },
  });
}

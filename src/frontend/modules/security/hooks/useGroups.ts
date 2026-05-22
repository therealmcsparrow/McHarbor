// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

type GroupMember = {
  id: string;
  userId: string;
  username: string;
};

type GroupRole = {
  id: string;
  roleId: string;
  roleName: string;
  environmentId: string | null;
  environmentName: string | null;
  stackName: string | null;
};

export type Group = {
  id: string;
  name: string;
  description: string;
  isSystem: boolean;
  memberCount: number;
  members?: GroupMember[];
  roles?: GroupRole[];
  createdAt: string;
  updatedAt: string;
};

export function useGroups() {
  return useQuery({
    queryKey: ['groups'],
    queryFn: () => api.get<Group[]>('/groups').then((r) => r.data ?? []),
  });
}

export function useGroup(id: string) {
  return useQuery({
    queryKey: ['groups', id],
    queryFn: () => api.get<Group>(`/groups/${id}`).then((r) => r.data),
    enabled: !!id,
  });
}

export function useCreateGroup() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('security');

  return useMutation({
    mutationFn: (body: { name: string; description: string }) =>
      api.post<Group>('/groups', body).then(assertSuccess),
    meta: { success: () => t('toast.groupCreated') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['groups'] });
    },
  });
}

export function useUpdateGroup() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('security');

  return useMutation({
    mutationFn: ({ id, ...body }: { id: string; name?: string; description?: string }) =>
      api.put<Group>(`/groups/${id}`, body).then(assertSuccess),
    meta: { success: () => t('toast.groupUpdated') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['groups'] });
    },
  });
}

export function useDeleteGroup() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('security');

  return useMutation({
    mutationFn: (id: string) => api.del(`/groups/${id}`).then(assertSuccess),
    meta: { success: () => t('toast.groupDeleted') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['groups'] });
    },
  });
}

export function useAddGroupMember() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('security');

  return useMutation({
    mutationFn: ({ groupId, userId }: { groupId: string; userId: string }) =>
      api.post(`/groups/${groupId}/members`, { userId }).then(assertSuccess),
    meta: { success: () => t('toast.memberAdded') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['groups'] });
      queryClient.invalidateQueries({ queryKey: ['user-groups'] });
    },
  });
}

export function useRemoveGroupMember() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('security');

  return useMutation({
    mutationFn: ({ groupId, userId }: { groupId: string; userId: string }) =>
      api.del(`/groups/${groupId}/members/${userId}`).then(assertSuccess),
    meta: { success: () => t('toast.memberRemoved') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['groups'] });
      queryClient.invalidateQueries({ queryKey: ['user-groups'] });
    },
  });
}

export function useAssignGroupRole() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('security');

  return useMutation({
    mutationFn: ({
      groupId,
      roleId,
      environmentId,
      stackNames,
    }: {
      groupId: string;
      roleId: string;
      environmentId?: string;
      stackNames?: string[];
    }) => {
      const scopes = stackNames && stackNames.length > 0 ? stackNames : [null];
      return Promise.all(
        scopes.map((stackName) =>
          api.post(`/groups/${groupId}/roles`, {
            roleId,
            environmentId: environmentId || null,
            stackName,
          }).then(assertSuccess),
        ),
      );
    },
    meta: { success: () => t('toast.roleAssigned') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['groups'] });
    },
  });
}

export function useUnassignGroupRole() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('security');

  return useMutation({
    mutationFn: ({ groupId, assignmentIds }: { groupId: string; assignmentIds: string[] }) =>
      Promise.all(
        assignmentIds.map((assignmentId) =>
          api.del(`/groups/${groupId}/roles/${assignmentId}`).then(assertSuccess),
        ),
      ),
    meta: { success: () => t('toast.roleUnassigned') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['groups'] });
    },
  });
}

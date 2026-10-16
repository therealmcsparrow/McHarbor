// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { usePermission } from '@core/auth/usePermission';
import { PERM } from '@core/auth/permissionKeys';
import { api, type PaginatedData } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export interface InAppNotification {
  id: string;
  level: 'info' | 'success' | 'warning' | 'error';
  title: string;
  message: string;
  action?: string;
  entityType?: string;
  entityId?: string;
  createdAt: string;
  read: boolean;
  readAt?: string;
}

type InAppNotificationCount = {
  count: number;
};

export const inAppNotificationQueryKeys = {
  all: ['in-app-notifications'] as const,
  list: (page: number, perPage: number, filter?: InAppNotificationFilter) =>
    ['in-app-notifications', 'list', page, perPage, filter ?? {}] as const,
  counts: ['in-app-notifications', 'counts'] as const,
  unreadCount: ['in-app-notifications', 'unread-count'] as const,
};

export type InAppNotificationFilter = {
  q?: string;
  mode?: 'contains' | 'exact' | 'regex';
  status?: 'all' | 'read' | 'unread';
  level?: string;
};

export type InAppNotificationCounts = { total: number; unread: number; byLevel: Record<string, number> };

type UseInAppNotificationsOptions = {
  page?: number;
  perPage?: number;
  filter?: InAppNotificationFilter;
};

/**
 * Both reads below are gated on in_app_notifications.view, which the backend
 * enforces on the same routes. Without the key the bell has nothing to show,
 * so asking for it produced a 403 on a timer.
 */
export function useInAppNotifications(options: UseInAppNotificationsOptions = {}) {
  const page = options.page ?? 1;
  const perPage = options.perPage ?? 12;
  const allowed = usePermission(PERM.IN_APP_NOTIFICATIONS_VIEW);

  return useQuery({
    enabled: allowed,
    queryKey: inAppNotificationQueryKeys.list(page, perPage, options.filter),
    queryFn: async () => {
      const f = options.filter ?? {};
      const params: Record<string, string> = { page: String(page), per_page: String(perPage) };
      if (f.q?.trim()) {
        params.q = f.q.trim();
        params.mode = f.mode ?? 'contains';
      }
      if (f.status && f.status !== 'all') params.status = f.status;
      if (f.level && f.level !== 'all') params.level = f.level;
      const response = await api.get<PaginatedData<InAppNotification>>('/in-app-notifications', params);
      return assertSuccess(response);
    },
    staleTime: 10_000,
    refetchInterval: 15_000,
  });
}

export function useUnreadInAppNotificationCount() {
  const allowed = usePermission(PERM.IN_APP_NOTIFICATIONS_VIEW);
  return useQuery({
    enabled: allowed,
    queryKey: inAppNotificationQueryKeys.unreadCount,
    queryFn: async () => {
      const response = await api.get<InAppNotificationCount>('/in-app-notifications/unread-count');
      return assertSuccess(response).count;
    },
    staleTime: 10_000,
    refetchInterval: 15_000,
  });
}

export function useMarkInAppNotificationRead() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (notificationID: string) => {
      const response = await api.post<void>(`/in-app-notifications/${notificationID}/read`);
      return assertSuccess(response);
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: inAppNotificationQueryKeys.all });
    },
  });
}

export function useMarkInAppNotificationsReadBatch() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (notificationIDs: string[]) => {
      await Promise.all(
        notificationIDs.map(async (notificationID) => {
          const response = await api.post<void>(`/in-app-notifications/${notificationID}/read`);
          return assertSuccess(response);
        }),
      );
      return notificationIDs.length;
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: inAppNotificationQueryKeys.all });
    },
  });
}

export function useMarkAllInAppNotificationsRead() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async () => {
      const response = await api.post<{ count: number }>('/in-app-notifications/read-all');
      return assertSuccess(response);
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: inAppNotificationQueryKeys.all });
    },
  });
}

export function useDeleteInAppNotification() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (notificationID: string) => {
      const response = await api.del<void>(`/in-app-notifications/${notificationID}`);
      return assertSuccess(response);
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: inAppNotificationQueryKeys.all });
    },
  });
}

export function useDeleteInAppNotificationsBatch() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (notificationIDs: string[]) => {
      await Promise.all(
        notificationIDs.map(async (notificationID) => {
          const response = await api.del<void>(`/in-app-notifications/${notificationID}`);
          return assertSuccess(response);
        }),
      );
      return notificationIDs.length;
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: inAppNotificationQueryKeys.all });
    },
  });
}

export function useInAppNotificationCounts() {
  const allowed = usePermission(PERM.IN_APP_NOTIFICATIONS_VIEW);
  return useQuery({
    enabled: allowed,
    queryKey: inAppNotificationQueryKeys.counts,
    queryFn: async () => assertSuccess(await api.get<InAppNotificationCounts>('/in-app-notifications/counts')),
    staleTime: 10_000,
    refetchInterval: 30_000,
  });
}

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type PaginatedData } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export interface InAppNotification {
  id: string;
  level: 'info' | 'success' | 'warning';
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
  list: (page: number, perPage: number) => ['in-app-notifications', 'list', page, perPage] as const,
  unreadCount: ['in-app-notifications', 'unread-count'] as const,
};

type UseInAppNotificationsOptions = {
  page?: number;
  perPage?: number;
};

export function useInAppNotifications(options: UseInAppNotificationsOptions = {}) {
  const page = options.page ?? 1;
  const perPage = options.perPage ?? 12;

  return useQuery({
    queryKey: inAppNotificationQueryKeys.list(page, perPage),
    queryFn: async () => {
      const response = await api.get<PaginatedData<InAppNotification>>('/in-app-notifications', {
        page: String(page),
        per_page: String(perPage),
      });
      return assertSuccess(response);
    },
    staleTime: 10_000,
    refetchInterval: 15_000,
  });
}

export function useUnreadInAppNotificationCount() {
  return useQuery({
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
      queryClient.invalidateQueries({ queryKey: inAppNotificationQueryKeys.all });
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
      queryClient.invalidateQueries({ queryKey: inAppNotificationQueryKeys.all });
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
      queryClient.invalidateQueries({ queryKey: inAppNotificationQueryKeys.all });
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
      queryClient.invalidateQueries({ queryKey: inAppNotificationQueryKeys.all });
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
      queryClient.invalidateQueries({ queryKey: inAppNotificationQueryKeys.all });
    },
  });
}

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useNavigate } from 'react-router';
import {
  useDeleteInAppNotificationsBatch,
  useDeleteInAppNotification,
  useInAppNotifications,
  useMarkInAppNotificationsReadBatch,
  useMarkAllInAppNotificationsRead,
  useMarkInAppNotificationRead,
  useUnreadInAppNotificationCount,
  type InAppNotification,
} from '@resources/hooks/useInAppNotifications';
import { getNotificationTarget } from '../utils';

export const notificationsPageSize = 25;

export function useNotificationsPageState() {
  const navigate = useNavigate();
  const [page, setPage] = useState(1);
  const [pendingDelete, setPendingDelete] = useState<InAppNotification | null>(null);
  const notificationsQuery = useInAppNotifications({ page, perPage: notificationsPageSize });
  const unreadCountQuery = useUnreadInAppNotificationCount();
  const markReadMutation = useMarkInAppNotificationRead();
  const markReadBatchMutation = useMarkInAppNotificationsReadBatch();
  const markAllReadMutation = useMarkAllInAppNotificationsRead();
  const deleteMutation = useDeleteInAppNotification();
  const deleteBatchMutation = useDeleteInAppNotificationsBatch();

  const notifications = notificationsQuery.data?.items ?? [];
  const total = notificationsQuery.data?.total ?? 0;
  const totalPages = notificationsQuery.data?.total_pages ?? 1;
  const unreadCount = unreadCountQuery.data ?? 0;
  const from = total === 0 ? 0 : (page - 1) * notificationsPageSize + 1;
  const to = notifications.length === 0 ? 0 : from + notifications.length - 1;

  function handleOpen(notification: InAppNotification) {
    const target = getNotificationTarget(notification);
    if (!target) {
      return;
    }
    if (!notification.read) {
      markReadMutation.mutate(notification.id);
    }
    navigate(target);
  }

  async function handleConfirmDelete() {
    if (!pendingDelete) {
      return;
    }

    await deleteMutation.mutateAsync(pendingDelete.id);
    if (notifications.length === 1 && page > 1) {
      setPage((current) => current - 1);
    }
    setPendingDelete(null);
  }

  async function handleBatchMarkRead(selectedNotifications: InAppNotification[]) {
    const unreadIDs = selectedNotifications
      .filter((notification) => !notification.read)
      .map((notification) => notification.id);

    if (unreadIDs.length > 0) {
      await markReadBatchMutation.mutateAsync(unreadIDs);
    }
  }

  async function handleBatchDelete(selectedNotifications: InAppNotification[]) {
    if (selectedNotifications.length === 0) {
      return;
    }

    await deleteBatchMutation.mutateAsync(selectedNotifications.map((notification) => notification.id));
    if (selectedNotifications.length >= notifications.length && page > 1) {
      setPage((current) => current - 1);
    }
  }

  return {
    page,
    setPage,
    pendingDelete,
    setPendingDelete,
    notificationsQuery,
    markAllReadMutation,
    deleteMutation,
    notifications,
    totalPages,
    unreadCount,
    from,
    to,
    handleOpen,
    handleConfirmDelete,
    handleBatchMarkRead,
    handleBatchDelete,
  };
}

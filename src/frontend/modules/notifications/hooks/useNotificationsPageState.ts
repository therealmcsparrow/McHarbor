// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router';
import {
  useDeleteInAppNotificationsBatch,
  useDeleteInAppNotification,
  useInAppNotifications,
  useInAppNotificationCounts,
  type InAppNotificationFilter,
  useMarkInAppNotificationsReadBatch,
  useMarkAllInAppNotificationsRead,
  useMarkInAppNotificationRead,
  useUnreadInAppNotificationCount,
  type InAppNotification,
} from '@resources/hooks/useInAppNotifications';
import { tableDefaultPageSize, tablePageSizeOptions } from '@resources/components/TablePagination';
import { getNotificationTarget } from '../utils';

// The shared list sizes, re-exported so this module and the grid cannot drift.
// 100 stays the ceiling: `response.ParsePagination` clamps `per_page` there, so
// a larger option would be trimmed server-side and the pager would lie.
export const notificationsPageSizeOptions = tablePageSizeOptions;
export const notificationsDefaultPageSize = tableDefaultPageSize;

export function useNotificationsPageState() {
  const navigate = useNavigate();
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState<number>(notificationsDefaultPageSize);
  const [pendingDelete, setPendingDelete] = useState<InAppNotification | null>(null);
  const [query, setQueryState] = useState('');
  const [debouncedQuery, setDebouncedQuery] = useState('');
  const [mode, setModeState] = useState<NonNullable<InAppNotificationFilter['mode']>>('contains');
  const [statusValue, setStatusState] = useState<NonNullable<InAppNotificationFilter['status']>>('all');
  const [level, setLevelState] = useState('all');
  useEffect(() => {
    const id = window.setTimeout(() => setDebouncedQuery(query), 300);
    return () => window.clearTimeout(id);
  }, [query]);
  const filter: InAppNotificationFilter = { q: debouncedQuery, mode, status: statusValue, level };
  const notificationsQuery = useInAppNotifications({ page, perPage: pageSize, filter });
  const countsQuery = useInAppNotificationCounts();
  const withReset =
    <T>(set: (v: T) => void) =>
    (v: T) => {
      set(v);
      setPage(1);
    };
  const filters = {
    query,
    setQuery: withReset(setQueryState),
    mode,
    setMode: withReset(setModeState),
    status: statusValue,
    setStatus: withReset(setStatusState),
    level,
    setLevel: withReset(setLevelState),
    counts: countsQuery.data,
  };
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
  const from = total === 0 ? 0 : (page - 1) * pageSize + 1;
  const to = notifications.length === 0 ? 0 : from + notifications.length - 1;

  // Typed page numbers arrive unvalidated, so clamp here rather than trusting
  // the input — the list only ever asks the API for a page that exists.
  function handlePageChange(next: number) {
    if (!Number.isFinite(next)) {
      return;
    }
    setPage(Math.min(Math.max(Math.trunc(next), 1), Math.max(totalPages, 1)));
  }

  // A bigger page makes the current offset meaningless, so go back to page 1.
  function handlePageSizeChange(next: number) {
    setPageSize(next);
    setPage(1);
  }

  function handleOpen(notification: InAppNotification) {
    const target = getNotificationTarget(notification);
    if (!target) {
      return;
    }
    if (!notification.read) {
      markReadMutation.mutate(notification.id);
    }
    void navigate(target);
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
    filters,
    page,
    pageSize,
    handlePageChange,
    handlePageSizeChange,
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

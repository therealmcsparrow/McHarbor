// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@tanstack/react-table';
import { IconCheck, IconExternalLink, IconTrash } from '@tabler/icons-react';
import { DataGrid, type BatchAction } from '@resources/components/DataGrid';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { formatDate, timeAgo } from '@resources/utils/format';
import type { InAppNotification } from '@resources/hooks/useInAppNotifications';
import { notificationsPageSize } from '../hooks/useNotificationsPageState';
import {
  getNotificationTarget,
  getNotificationTypeLabel,
  notificationLevelVariant,
} from '../utils';

type NotificationsTableProps = {
  notifications: InAppNotification[];
  loading: boolean;
  emptyMessage: string;
  unreadCount: number;
  from: number;
  to: number;
  total: number;
  page: number;
  totalPages: number;
  deletePending: boolean;
  onOpen: (notification: InAppNotification) => void;
  onDelete: (notification: InAppNotification) => void;
  onBatchMarkRead: (notifications: InAppNotification[]) => Promise<void>;
  onBatchDelete: (notifications: InAppNotification[]) => Promise<void>;
  onPreviousPage: () => void;
  onNextPage: () => void;
};

export function NotificationsTable({
  notifications,
  loading,
  emptyMessage,
  unreadCount,
  from,
  to,
  total,
  page,
  totalPages,
  deletePending,
  onOpen,
  onDelete,
  onBatchMarkRead,
  onBatchDelete,
  onPreviousPage,
  onNextPage,
}: NotificationsTableProps) {
  const { t } = useTranslation('common');

  const columns = useMemo<ColumnDef<InAppNotification, unknown>[]>(
    () => [
      {
        id: 'level',
        accessorFn: (notification) => notification.level,
        header: t('notifications.columnLevel'),
        cell: ({ row }) => (
          <Badge variant={notificationLevelVariant(row.original.level)}>
            {t(`notifications.level.${row.original.level}`)}
          </Badge>
        ),
      },
      {
        accessorKey: 'title',
        header: t('notifications.columnTitle'),
        cell: ({ row }) => <span className="font-medium">{row.original.title}</span>,
      },
      {
        accessorKey: 'message',
        header: t('notifications.columnMessage'),
        cell: ({ row }) => (
          <span className="line-clamp-2 max-w-2xl text-muted-foreground">{row.original.message}</span>
        ),
      },
      {
        id: 'type',
        accessorFn: (notification) => getNotificationTypeLabel(notification, t),
        header: t('notifications.columnType'),
        cell: ({ row }) => getNotificationTypeLabel(row.original, t),
      },
      {
        id: 'status',
        accessorFn: (notification) => (notification.read ? 'read' : 'unread'),
        header: t('notifications.columnStatus'),
        cell: ({ row }) => (
          <Badge variant={row.original.read ? 'secondary' : 'default'}>
            {row.original.read ? t('notifications.statusRead') : t('notifications.statusUnread')}
          </Badge>
        ),
      },
      {
        id: 'createdAt',
        accessorFn: (notification) => notification.createdAt,
        header: t('notifications.columnDate'),
        cell: ({ row }) => (
          <span className="text-muted-foreground" title={formatDate(row.original.createdAt)}>
            {timeAgo(row.original.createdAt)}
          </span>
        ),
      },
      {
        id: 'actions',
        header: () => <span className="ml-auto">{t('notifications.columnActions')}</span>,
        cell: ({ row }) => {
          const target = getNotificationTarget(row.original);

          return (
            <div className="flex items-center justify-end gap-1">
              <Button
                type="button"
                variant="ghost"
                size="icon"
                title={t('notifications.openItem')}
                aria-label={t('notifications.openItem')}
                disabled={!target}
                onClick={(event) => {
                  event.stopPropagation();
                  onOpen(row.original);
                }}
              >
                <IconExternalLink className="h-4 w-4" />
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                title={t('actions.delete')}
                aria-label={t('actions.delete')}
                disabled={deletePending}
                onClick={(event) => {
                  event.stopPropagation();
                  onDelete(row.original);
                }}
              >
                <IconTrash className="h-4 w-4 text-destructive" />
              </Button>
            </div>
          );
        },
      },
    ],
    [deletePending, onDelete, onOpen, t],
  );

  const batchActions = useMemo<BatchAction[]>(
    () => [
      {
        label: t('batch.markRead'),
        icon: IconCheck,
        variant: 'default',
        onClick: (rows) => {
          void onBatchMarkRead(rows as InAppNotification[]);
        },
      },
      {
        label: t('actions.delete'),
        icon: IconTrash,
        variant: 'destructive',
        confirm: true,
        onClick: (rows) => {
          void onBatchDelete(rows as InAppNotification[]);
        },
      },
    ],
    [onBatchDelete, onBatchMarkRead, t],
  );

  return (
    <div className="space-y-4">
      <DataGrid
        data={notifications}
        columns={columns}
        searchKey="title"
        loading={loading}
        emptyMessage={emptyMessage}
        pageSize={notificationsPageSize}
        selectable
        batchActions={batchActions}
        getRowId={(row) => row.id}
      />

      {totalPages > 1 && (
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-border bg-card px-4 py-3 shadow-sm">
          <div className="space-y-1">
            <p className="text-sm text-muted-foreground">
              {unreadCount > 0 ? t('notifications.unreadCount', { count: unreadCount }) : t('notifications.allCaughtUp')}
            </p>
            <p className="text-sm text-muted-foreground">{t('dataGrid.showing', { from, to, total })}</p>
          </div>
          <div className="flex items-center gap-2">
            <Button type="button" variant="outline" size="sm" disabled={page <= 1} onClick={onPreviousPage}>
              {t('dataGrid.prev')}
            </Button>
            <Button type="button" variant="outline" size="sm" disabled={page >= totalPages} onClick={onNextPage}>
              {t('dataGrid.next')}
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}

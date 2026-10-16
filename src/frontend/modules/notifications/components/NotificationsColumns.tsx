// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The notifications table's seven columns. Extracted from NotificationsTable so
// both files stay under the 200-line component rule.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@resources/components/table-features';
import { IconExternalLink, IconTrash } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Hint } from '@resources/components/ui/Hint';
import { formatDate, timeAgo } from '@resources/utils/format';
import type { InAppNotification } from '@resources/hooks/useInAppNotifications';
import type { TableColumnMeta } from '@resources/layout/table-page/types';
import { textColumn, dateColumn } from '@resources/layout/table-page/optional-columns';
import { getNotificationTarget, getNotificationTypeLabel, notificationLevelVariant } from '../utils';

type UseNotificationColumnsArgs = {
  deletePending: boolean;
  onOpen: (notification: InAppNotification) => void;
  onDelete: (notification: InAppNotification) => void;
};

export function useNotificationColumns({
  deletePending,
  onOpen,
  onDelete,
}: UseNotificationColumnsArgs): ColumnDef<InAppNotification, unknown>[] {
  const { t } = useTranslation('common');
  const group = t('notifications.columnsGroup');

  return useMemo<ColumnDef<InAppNotification, unknown>[]>(
    () => [
      {
        id: 'level',
        accessorFn: (notification) => notification.level,
        header: t('notifications.columnLevel'),
        meta: { width: 'sm' } satisfies TableColumnMeta,
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
        cell: ({ row }) => <span className="line-clamp-2 max-w-2xl text-muted-foreground">{row.original.message}</span>,
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
        meta: { width: 'sm' } satisfies TableColumnMeta,
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
        meta: { width: 'md' } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <Hint text={formatDate(row.original.createdAt)}>
            <span className="text-muted-foreground">{timeAgo(row.original.createdAt)}</span>
          </Hint>
        ),
      },
      textColumn<InAppNotification>('action', t('notifications.columnAction'), (r) => r.action, { group }),
      textColumn<InAppNotification>('entityId', t('notifications.columnEntityId'), (r) => r.entityId, { group }),
      dateColumn<InAppNotification>('readAt', t('notifications.columnReadAt'), (r) => r.readAt, { group }),
      {
        id: 'actions',
        header: t('notifications.columnActions'),
        enableSorting: false,
        meta: {
          width: 'sm',
          locked: true,
          noExport: true,
          label: t('notifications.columnActions'),
        } satisfies TableColumnMeta,
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
                onClick={(eventItem) => {
                  eventItem.stopPropagation();
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
                onClick={(evt) => {
                  evt.stopPropagation();
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
    [deletePending, onDelete, onOpen, t, group],
  );
}

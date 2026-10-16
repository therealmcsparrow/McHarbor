// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { IconCheck, IconTrash } from '@tabler/icons-react';
import { TablePage } from '@resources/layout/TablePage';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { Select } from '@resources/components/ui/Select';
import type { SearchMode } from '@resources/utils/search-filter';
import type { InAppNotificationCounts, InAppNotificationFilter } from '@resources/hooks/useInAppNotifications';
import type { BatchAction } from '@resources/layout/table-page/types';
import { useNotificationColumns } from './NotificationsColumns';
import type { InAppNotification } from '@resources/hooks/useInAppNotifications';

export type NotificationFilters = {
  query: string;
  setQuery: (v: string) => void;
  mode: NonNullable<InAppNotificationFilter['mode']>;
  setMode: (v: NonNullable<InAppNotificationFilter['mode']>) => void;
  status: NonNullable<InAppNotificationFilter['status']>;
  setStatus: (v: NonNullable<InAppNotificationFilter['status']>) => void;
  level: string;
  setLevel: (v: string) => void;
  counts?: InAppNotificationCounts;
};

const LEVELS = ['info', 'success', 'warning', 'error'] as const;

type NotificationsTableProps = {
  filters: NotificationFilters;
  notifications: InAppNotification[];
  loading: boolean;
  emptyMessage: string;
  unreadCount: number;
  from: number;
  to: number;
  total: number;
  page: number;
  totalPages: number;
  pageSize: number;
  pageSizeOptions: readonly number[];
  deletePending: boolean;
  onOpen: (notification: InAppNotification) => void;
  onDelete: (notification: InAppNotification) => void;
  onBatchMarkRead: (notifications: InAppNotification[]) => Promise<void>;
  onBatchDelete: (notifications: InAppNotification[]) => Promise<void>;
  onPageChange: (page: number) => void;
  onPageSizeChange: (pageSize: number) => void;
};

export function NotificationsTable({
  filters,
  notifications,
  loading,
  emptyMessage,
  unreadCount,
  from,
  to,
  total,
  page,
  totalPages,
  pageSize,
  pageSizeOptions,
  deletePending,
  onOpen,
  onDelete,
  onBatchMarkRead,
  onBatchDelete,
  onPageChange,
  onPageSizeChange,
}: NotificationsTableProps) {
  const { t } = useTranslation('common');
  const columns = useNotificationColumns({ deletePending, onOpen, onDelete });

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
    <TablePage
      id="notifications"
      // NotificationsPage renders its own PageHeader above this component.
      header={false}
      title={t('notifications.columnTitle')}
      search={{
        value: filters.query,
        onChange: filters.setQuery,
        mode: filters.mode,
        onModeChange: (m: SearchMode) => filters.setMode(m),
        placeholder: t('notifications.searchPlaceholder'),
      }}
      facets={{
        items: [
          { id: 'all', label: t('notifications.filterAll'), count: filters.counts?.total ?? 0 },
          { id: 'unread', label: t('notifications.statusUnread'), count: filters.counts?.unread ?? 0 },
          {
            id: 'read',
            label: t('notifications.statusRead'),
            count: Math.max((filters.counts?.total ?? 0) - (filters.counts?.unread ?? 0), 0),
          },
        ],
        value: filters.status,
        onChange: (v) => filters.setStatus(v as NotificationFilters['status']),
      }}
      filters={
        <Select
          value={filters.level}
          onChange={filters.setLevel}
          ariaLabel={t('notifications.columnLevel')}
          searchable={false}
          options={[
            { value: 'all', label: t('notifications.allLevels') },
            ...LEVELS.map((l) => ({
              value: l,
              label: `${t(`notifications.level.${l}`)} (${filters.counts?.byLevel[l] ?? 0})`,
            })),
          ]}
        />
      }
      toolbar={{ columns: true, density: true, export: { filename: 'mcharbor-notifications' } }}
      columns={columns}
      data={notifications}
      loading={loading}
      empty={{ title: emptyMessage }}
      selectable
      batchActions={batchActions}
      getRowId={(row) => row.id}
      pagination={
        total > 0
          ? {
              mode: 'server',
              page,
              totalPages,
              pageSize,
              pageSizeOptions,
              total,
              from,
              to,
              onPageChange,
              onPageSizeChange,
              // Was NotificationsPagination's only real addition.
              leading: (
                <TypographyText size="xs" tone="muted">
                  {unreadCount > 0
                    ? t('notifications.unreadCount', { count: unreadCount })
                    : t('notifications.allCaughtUp')}
                </TypographyText>
              ),
            }
          : { mode: 'none' }
      }
    />
  );
}

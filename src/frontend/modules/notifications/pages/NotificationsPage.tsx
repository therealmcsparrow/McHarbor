// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { PageHeader } from '@resources/layout/PageHeader';
import { NotificationsTable } from '../components/NotificationsTable';
import { useNotificationsPageState } from '../hooks/useNotificationsPageState';

export default function NotificationsPage() {
  const { t } = useTranslation('common');
  const {
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
  } = useNotificationsPageState();
  const total = notificationsQuery.data?.total ?? 0;
  const gridEmptyMessage = notificationsQuery.isError
    ? t('notifications.unavailable')
    : t('notifications.empty');

  return (
    <div className="space-y-6">
      <PageHeader
        title={t('notifications.title')}
        description={t('notifications.pageDescription')}
        actions={
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={unreadCount === 0 || markAllReadMutation.isPending}
            onClick={() => markAllReadMutation.mutate()}
          >
            {t('notifications.markAllRead')}
          </Button>
        }
      />

      <NotificationsTable
        notifications={notifications}
        loading={notificationsQuery.isPending}
        emptyMessage={gridEmptyMessage}
        unreadCount={unreadCount}
        from={from}
        to={to}
        total={total}
        page={page}
        totalPages={totalPages}
        deletePending={deleteMutation.isPending}
        onOpen={handleOpen}
        onDelete={setPendingDelete}
        onBatchMarkRead={handleBatchMarkRead}
        onBatchDelete={handleBatchDelete}
        onPreviousPage={() => setPage((current) => current - 1)}
        onNextPage={() => setPage((current) => current + 1)}
      />

      <ConfirmDialog
        open={pendingDelete !== null}
        onOpenChange={(open) => !open && setPendingDelete(null)}
        title={t('notifications.deleteTitle')}
        description={t('notifications.deleteDescription', { title: pendingDelete?.title ?? '' })}
        confirmLabel={t('actions.delete')}
        loading={deleteMutation.isPending}
        onConfirm={() => {
          void handleConfirmDelete();
        }}
      />
    </div>
  );
}

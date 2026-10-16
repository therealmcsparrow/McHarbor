// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { PageHeader } from '@resources/layout/PageHeader';
import { NotificationsTable } from '../components/NotificationsTable';
import { notificationsPageSizeOptions, useNotificationsPageState } from '../hooks/useNotificationsPageState';

export default function NotificationsPage() {
  const { t } = useTranslation('common');
  const {
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
  } = useNotificationsPageState();
  const total = notificationsQuery.data?.total ?? 0;
  const gridEmptyMessage = notificationsQuery.isError
    ? [t('notifications.unavailable'), notificationsQuery.error?.message].filter(Boolean).join(' — ')
    : t('notifications.empty');

  return (
    <div className="flex h-full min-h-0 flex-col gap-4">
      <PageHeader
        headingLeaf
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

      {/* The route is declared `scroll: 'data'`: the shell is a fixed frame and
          the page itself never scrolls. The table claims the remaining height
          and scrolls its own rows, so its toolbar and pagination band stay put. */}
      <NotificationsTable
        filters={filters}
        notifications={notifications}
        loading={notificationsQuery.isPending}
        emptyMessage={gridEmptyMessage}
        unreadCount={unreadCount}
        from={from}
        to={to}
        total={total}
        page={page}
        totalPages={totalPages}
        pageSize={pageSize}
        pageSizeOptions={notificationsPageSizeOptions}
        deletePending={deleteMutation.isPending}
        onOpen={handleOpen}
        onDelete={setPendingDelete}
        onBatchMarkRead={handleBatchMarkRead}
        onBatchDelete={handleBatchDelete}
        onPageChange={handlePageChange}
        onPageSizeChange={handlePageSizeChange}
      />

      <ConfirmDialog
        open={pendingDelete !== null}
        onOpenChange={(isOpen) => !isOpen && setPendingDelete(null)}
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

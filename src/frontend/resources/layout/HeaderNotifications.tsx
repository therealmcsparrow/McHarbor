// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { IconBell } from '@tabler/icons-react';
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import { InAppNotificationListItem } from '@resources/components/InAppNotificationListItem';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import {
  useInAppNotifications,
  useMarkAllInAppNotificationsRead,
  useMarkInAppNotificationRead,
  useUnreadInAppNotificationCount,
} from '@resources/hooks/useInAppNotifications';

const panelID = 'header-notifications-panel';

export function HeaderNotifications() {
  const { t } = useTranslation('common');
  const navigate = useNavigate();
  const [isOpen, setOpen] = useState(false);
  const [refreshingOnOpen, setRefreshingOnOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);
  const notificationsQuery = useInAppNotifications({ perPage: 12 });
  const unreadCountQuery = useUnreadInAppNotificationCount();
  const { refetch: refetchNotifications } = notificationsQuery;
  const { refetch: refetchUnreadCount } = unreadCountQuery;
  const markReadMutation = useMarkInAppNotificationRead();
  const markAllReadMutation = useMarkAllInAppNotificationsRead();

  const totalNotifications = notificationsQuery.data?.total ?? 0;
  const unreadNotifications = (notificationsQuery.data?.items ?? []).filter((notification) => !notification.read);
  const unreadCount = unreadCountQuery.data ?? 0;
  const unreadBadge = unreadCount > 9 ? '9+' : String(unreadCount);
  const hasUnread = unreadCount > 0;

  async function handleToggleOpen() {
    if (isOpen) {
      setOpen(false);
      return;
    }

    setRefreshingOnOpen(true);
    setOpen(true);
    try {
      await Promise.all([refetchNotifications(), refetchUnreadCount()]);
    } finally {
      setRefreshingOnOpen(false);
    }
  }

  function handleOpenPage() {
    setOpen(false);
    void navigate('/notifications');
  }

  useEffect(() => {
    if (!isOpen) {
      return;
    }

    function handlePointerDown(evt: MouseEvent) {
      if (rootRef.current && !rootRef.current.contains(evt.target as Node)) {
        setOpen(false);
      }
    }

    function handleKeyDown(evt: KeyboardEvent) {
      if (evt.key === 'Escape') {
        setOpen(false);
      }
    }

    document.addEventListener('mousedown', handlePointerDown);
    document.addEventListener('keydown', handleKeyDown);

    return () => {
      document.removeEventListener('mousedown', handlePointerDown);
      document.removeEventListener('keydown', handleKeyDown);
    };
  }, [isOpen, refetchNotifications, refetchUnreadCount]);

  return (
    <div ref={rootRef} className="relative">
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            aria-label={t('notifications.open')}
            aria-controls={isOpen ? panelID : undefined}
            aria-expanded={isOpen}
            aria-haspopup="dialog"
            className="relative text-muted-foreground hover:text-foreground"
            onClick={() => {
              void handleToggleOpen();
            }}
          >
            <IconBell className="size-4.5" />
            {hasUnread && (
              <span className="absolute top-1 right-1 flex min-w-4 items-center justify-center bg-primary px-1 text-[10px] leading-none font-semibold text-primary-foreground">
                {unreadBadge}
              </span>
            )}
          </Button>
        </TooltipTrigger>
        <TooltipContent>{t('notifications.open')}</TooltipContent>
      </Tooltip>

      {isOpen && (
        <dialog
          open
          id={panelID}
          aria-label={t('notifications.title')}
          className="absolute top-full right-0 left-auto z-50 m-0 mt-2 block h-auto w-[22rem] max-w-[calc(100vw-2rem)] overflow-hidden rounded-xl border border-border bg-popover p-0 text-foreground shadow-xl"
        >
          <div className="flex items-start justify-between gap-3 border-b border-border px-4 py-3">
            <div className="min-w-0">
              <p className="text-sm font-semibold text-foreground">{t('notifications.title')}</p>
              <p className="text-xs text-muted-foreground">
                {hasUnread ? t('notifications.unreadCount', { count: unreadCount }) : t('notifications.allCaughtUp')}
              </p>
            </div>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              aria-label={t('notifications.markAllRead')}
              disabled={!hasUnread || markAllReadMutation.isPending}
              onClick={() => markAllReadMutation.mutate()}
            >
              {t('notifications.markAllRead')}
            </Button>
          </div>

          <div className="max-h-[70vh] overflow-y-auto">
            {(notificationsQuery.isPending || refreshingOnOpen) && (
              <div className="px-4 py-6 text-sm text-muted-foreground">{t('notifications.loading')}</div>
            )}

            {notificationsQuery.isError && (
              <div className="px-4 py-6 text-sm text-muted-foreground">
                {[t('notifications.unavailable'), notificationsQuery.error?.message].filter(Boolean).join(' — ')}
              </div>
            )}

            {!notificationsQuery.isPending &&
              !refreshingOnOpen &&
              !notificationsQuery.isError &&
              unreadNotifications.length === 0 && (
                <div className="px-4 py-6">
                  <p className="text-sm font-medium text-foreground">
                    {totalNotifications === 0 ? t('notifications.empty') : t('notifications.allCaughtUp')}
                  </p>
                  <p className="mt-1 text-sm text-muted-foreground">
                    {totalNotifications === 0
                      ? t('notifications.emptyDescription')
                      : t('notifications.pageDescription')}
                  </p>
                </div>
              )}

            {!notificationsQuery.isPending &&
              !refreshingOnOpen &&
              !notificationsQuery.isError &&
              unreadNotifications.length > 0 && (
                <div>
                  {unreadNotifications.map((notification) => {
                    return (
                      <InAppNotificationListItem
                        key={notification.id}
                        notification={notification}
                        className="border-b border-border last:border-b-0"
                        markReadDisabled={markReadMutation.isPending}
                        onMarkRead={(notificationID) => markReadMutation.mutate(notificationID)}
                      />
                    );
                  })}
                </div>
              )}
          </div>

          <div className="border-t border-border p-2">
            <Button type="button" variant="ghost" size="sm" className="w-full justify-center" onClick={handleOpenPage}>
              {t('notifications.viewAll')}
            </Button>
          </div>
        </dialog>
      )}
    </div>
  );
}

export default HeaderNotifications;

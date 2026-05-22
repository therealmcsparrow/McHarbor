// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { IconCheck } from '@tabler/icons-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import type { InAppNotification } from '@resources/hooks/useInAppNotifications';
import { cn } from '@resources/utils/cn';
import { formatDate, timeAgo } from '@resources/utils/format';

function toneClasses(level: InAppNotification['level']) {
  switch (level) {
    case 'success':
      return {
        dot: 'bg-emerald-500',
        ring: 'border-emerald-500/30 bg-emerald-500/10',
      };
    case 'warning':
      return {
        dot: 'bg-amber-500',
        ring: 'border-amber-500/30 bg-amber-500/10',
      };
    default:
      return {
        dot: 'bg-sky-500',
        ring: 'border-sky-500/30 bg-sky-500/10',
      };
  }
}

type InAppNotificationListItemProps = {
  notification: InAppNotification;
  className?: string;
  markReadDisabled?: boolean;
  onMarkRead?: (notificationID: string) => void;
};

export function InAppNotificationListItem({
  notification,
  className,
  markReadDisabled = false,
  onMarkRead,
}: InAppNotificationListItemProps) {
  const { t } = useTranslation('common');
  const tone = toneClasses(notification.level);

  return (
    <article
      className={cn(
        'flex gap-3 px-4 py-3',
        !notification.read && 'bg-muted/20',
        className,
      )}
    >
      <div className={cn('mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-full border', tone.ring)}>
        <span className={cn('size-2 rounded-full', tone.dot)} />
      </div>

      <div className="min-w-0 flex-1">
        <div className="flex items-start gap-2">
          <div className="min-w-0 flex-1">
            <p className="text-sm font-medium text-foreground">{notification.title}</p>
            <p className="mt-1 break-words text-sm text-muted-foreground">{notification.message}</p>
          </div>

          {!notification.read && onMarkRead && (
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label={t('notifications.markRead')}
              disabled={markReadDisabled}
              onClick={() => onMarkRead(notification.id)}
            >
              <IconCheck className="size-4" />
            </Button>
          )}
        </div>

        <p className="mt-2 text-xs text-muted-foreground" title={formatDate(notification.createdAt)}>
          {timeAgo(notification.createdAt)}
        </p>
      </div>
    </article>
  );
}

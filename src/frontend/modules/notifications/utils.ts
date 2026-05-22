// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { TFunction } from 'i18next';
import type { InAppNotification } from '@resources/hooks/useInAppNotifications';

export function notificationLevelVariant(level: InAppNotification['level']) {
  switch (level) {
    case 'success':
      return 'success';
    case 'warning':
      return 'warning';
    default:
      return 'secondary';
  }
}

export function getNotificationTarget(notification: InAppNotification) {
  switch (notification.entityType) {
    case 'container':
      return notification.entityId ? `/containers/${encodeURIComponent(notification.entityId)}` : null;
    case 'alert':
      return '/settings?tab=alerts';
    default:
      return null;
  }
}

export function getNotificationTypeLabel(notification: InAppNotification, t: TFunction<'common'>) {
  switch (notification.entityType) {
    case 'container':
      return t('notifications.typeContainer');
    case 'alert':
      return t('notifications.typeAlert');
    default:
      return notification.entityType ?? t('notifications.typeSystem');
  }
}

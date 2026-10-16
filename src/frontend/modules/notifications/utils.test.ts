// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { describe, expect, it, vi } from 'vitest';
import type { InAppNotification } from '@resources/hooks/useInAppNotifications';
import { getNotificationTarget, getNotificationTypeLabel, notificationLevelVariant } from './utils';

function notification(overrides: Partial<InAppNotification>): InAppNotification {
  return {
    id: 'notification-id',
    level: 'info',
    title: 'Title',
    message: 'Message',
    entityType: undefined,
    entityId: undefined,
    read: false,
    createdAt: '2026-05-28T12:00:00Z',
    ...overrides,
  };
}

describe('notification utilities', () => {
  it('maps notification levels to badge variants', () => {
    expect(notificationLevelVariant('success')).toBe('success');
    expect(notificationLevelVariant('warning')).toBe('warning');
    expect(notificationLevelVariant('info')).toBe('secondary');
    expect(notificationLevelVariant('error')).toBe('destructive');
  });

  it('builds notification targets by entity type', () => {
    expect(getNotificationTarget(notification({ entityType: 'container', entityId: 'abc/123' }))).toBe(
      '/containers/abc%2F123',
    );
    expect(getNotificationTarget(notification({ entityType: 'container', entityId: undefined }))).toBeNull();
    expect(getNotificationTarget(notification({ entityType: 'alert' }))).toBe('/settings?tab=alerts');
    expect(getNotificationTarget(notification({ entityType: 'system' }))).toBeNull();
  });

  it('labels notification types through i18n and fallbacks', () => {
    const t = vi.fn((key: string) => key) as unknown as Parameters<typeof getNotificationTypeLabel>[1];

    expect(getNotificationTypeLabel(notification({ entityType: 'container' }), t)).toBe('notifications.typeContainer');
    expect(getNotificationTypeLabel(notification({ entityType: 'alert' }), t)).toBe('notifications.typeAlert');
    expect(getNotificationTypeLabel(notification({ entityType: 'custom' }), t)).toBe('custom');
    expect(getNotificationTypeLabel(notification({ entityType: undefined }), t)).toBe('notifications.typeSystem');
  });
});

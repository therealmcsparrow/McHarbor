// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import type { PaginatedData } from '@core/api/client';
import { PERM } from '@core/auth/permissionKeys';
import { usePermission } from '@core/auth/usePermission';
import type { InAppNotification } from '@resources/hooks/useInAppNotifications';
import type { AlertEvent } from '@resources/hooks/useAlertEvents';
import { oldestCachedAt, useOnCallQuery } from '@resources/hooks/useOnCallQuery';

export type InboxItem = {
  id: string;
  kind: 'alert' | 'notice';
  severity: string;
  title: string;
  message: string;
  state?: AlertEvent['state'];
  environmentId?: string;
  environmentName?: string;
  subjectId?: string;
  subjectName?: string;
  sourceType?: string;
  conditionKey?: string;
  alertId?: string;
  createdAt: string;
};

const NOTICE_SEVERITY: Record<InAppNotification['level'], string> = {
  info: 'info',
  success: 'success',
  warning: 'warning',
  error: 'error',
};

export function mergeInbox(alerts: AlertEvent[], notices: InAppNotification[]): InboxItem[] {
  const items: InboxItem[] = [
    ...alerts.map((evt) => ({
      id: `alert-${evt.id}`,
      kind: 'alert' as const,
      severity: evt.severity,
      title: evt.alertName,
      message: evt.message,
      state: evt.state,
      environmentId: evt.environmentId,
      environmentName: evt.environmentName,
      subjectId: evt.subjectId,
      subjectName: evt.subjectName,
      sourceType: evt.sourceType,
      conditionKey: evt.conditionKey,
      alertId: evt.alertId,
      createdAt: evt.createdAt,
    })),
    ...notices.map((notice) => ({
      id: `notice-${notice.id}`,
      kind: 'notice' as const,
      severity: NOTICE_SEVERITY[notice.level] ?? 'info',
      title: notice.title,
      message: notice.message,
      createdAt: notice.createdAt,
    })),
  ];
  return items.sort((a, b) => Date.parse(b.createdAt) - Date.parse(a.createdAt));
}

export function useInbox() {
  const canAlerts = usePermission(PERM.ALERTS_VIEW);
  const canNotices = usePermission(PERM.IN_APP_NOTIFICATIONS_VIEW);
  const alerts = useOnCallQuery<PaginatedData<AlertEvent>>(
    ['alerts'],
    '/alerts/events',
    { page: '1', per_page: '50' },
    { enabled: canAlerts, refetchInterval: 30_000 },
  );
  const notices = useOnCallQuery<PaginatedData<InAppNotification>>(
    ['notices'],
    '/in-app-notifications',
    { page: '1', per_page: '25' },
    { enabled: canNotices, refetchInterval: 30_000 },
  );
  const items = useMemo(
    () => mergeInbox(alerts.data?.data.items ?? [], notices.data?.data.items ?? []),
    [alerts.data, notices.data],
  );
  return {
    items,
    canRead: canAlerts || canNotices,
    isLoading: alerts.isLoading || notices.isLoading,
    error: alerts.error ?? notices.error,
    refetch: () => Promise.all([alerts.refetch(), notices.refetch()]),
    cachedAt: oldestCachedAt(alerts.data?.cachedAt, notices.data?.cachedAt),
  };
}

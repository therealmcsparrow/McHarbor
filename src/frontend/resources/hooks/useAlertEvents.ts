// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api, type PaginatedData } from '@core/api/client';
import type { AlertSeverity } from './useAlerts';

export type AlertEventState = 'firing' | 'resolved';

export type AlertEventDelivery = {
  channelId: string;
  ok: boolean;
  errorCode?: string;
};

export type AlertEvent = {
  id: string;
  alertId: string;
  alertName: string;
  sourceType: string;
  conditionKey: string;
  environmentId?: string;
  environmentName?: string;
  subjectId?: string;
  subjectName?: string;
  state: AlertEventState;
  severity: AlertSeverity;
  value: number;
  valueLabel?: string;
  message: string;
  deliveries: AlertEventDelivery[];
  createdAt: string;
};

const EMPTY_EVENTS_PAGE: PaginatedData<AlertEvent> = {
  items: [],
  total: 0,
  page: 1,
  per_page: 25,
  total_pages: 0,
};

type UseAlertEventsOptions = {
  page?: number;
  perPage?: number;
  alertId?: string;
  state?: AlertEventState | '';
  severity?: AlertSeverity | '';
  enabled?: boolean;
};

/**
 * Alert history. Rows outlive the rule that produced them, so a
 * filtered-by-alert view can legitimately return events for a rule
 * that no longer exists.
 */
export function useAlertEvents({
  page = 1,
  perPage = 25,
  alertId,
  state,
  severity,
  enabled = true,
}: UseAlertEventsOptions = {}) {
  return useQuery({
    queryKey: ['alert-events', page, perPage, alertId ?? '', state ?? '', severity ?? ''],
    queryFn: () => {
      const params: Record<string, string> = { page: String(page), per_page: String(perPage) };
      if (alertId) params.alertId = alertId;
      if (state) params.state = state;
      if (severity) params.severity = severity;

      return api
        .get<PaginatedData<AlertEvent>>('/alerts/events', params)
        .then((response) => response.data ?? { ...EMPTY_EVENTS_PAGE, per_page: perPage });
    },
    enabled,
    // An alert firing while this panel is open should show up without the
    // operator reloading the page.
    refetchInterval: 30_000,
  });
}

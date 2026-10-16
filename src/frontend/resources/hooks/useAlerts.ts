// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api, type PaginatedData } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export type AlertSeverity = 'critical' | 'warning' | 'info';

/**
 * The structured replacement for the old free-text condition string.
 * Which fields apply depends on the condition's declared kind, which
 * the catalog reports — see useAlertConditions.
 */
export type AlertConditionConfig = {
  operator?: string;
  threshold?: number;
  unit?: string;
  forSeconds?: number;
  windowSeconds?: number;
  params?: Record<string, string>;
};

export type AlertDestination = {
  id: string;
  channelId: string;
  minSeverity?: string;
  /** Empty when the channel behind this destination was deleted. */
  channelName?: string;
  channelType?: string;
};

export type AlertDestinationInput = {
  channelId: string;
  minSeverity?: string;
};

export type AlertRule = {
  id: string;
  name: string;
  severity: AlertSeverity;
  sourceType: string;
  conditionKey: string;
  condition: AlertConditionConfig;
  target: string;
  environmentIds: string[];
  destinations: AlertDestination[];
  sendInApp: boolean;
  cooldownSeconds: number;
  notifyOnRecovery: boolean;
  messageTemplate: string;
  enabled: boolean;
  /** False when the plugin owning this rule's source is not installed. */
  sourceAvailable: boolean;
  createdAt: string;
  updatedAt: string;
};

export type CreateAlertInput = {
  name: string;
  severity: AlertSeverity;
  sourceType: string;
  conditionKey: string;
  condition: AlertConditionConfig;
  target: string;
  environmentIds: string[];
  destinations: AlertDestinationInput[];
  sendInApp: boolean;
  cooldownSeconds: number;
  notifyOnRecovery: boolean;
  messageTemplate: string;
};

export type UpdateAlertInput = Partial<CreateAlertInput> & { enabled?: boolean };

export type AlertTestResult = {
  deliveries: { channelId: string; ok: boolean; errorCode?: string }[];
  message: string;
};

const EMPTY_ALERTS_PAGE: PaginatedData<AlertRule> = {
  items: [],
  total: 0,
  page: 1,
  per_page: 100,
  total_pages: 0,
};

type UseAlertsOptions = {
  perPage?: number;
  refetchInterval?: number;
};

export function useAlerts({ perPage = 100, refetchInterval }: UseAlertsOptions = {}) {
  return useQuery({
    queryKey: ['alerts', perPage],
    queryFn: () =>
      api
        .get<PaginatedData<AlertRule>>('/alerts', { page: '1', per_page: String(perPage) })
        .then((response) => response.data ?? { ...EMPTY_ALERTS_PAGE, per_page: perPage }),
    refetchInterval,
  });
}

export function useCreateAlert() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');

  return useMutation({
    mutationFn: (body: CreateAlertInput) => api.post<AlertRule>('/alerts', body).then(assertSuccess),
    meta: { success: () => t('toast.alertCreated') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['alerts'] });
    },
  });
}

export function useUpdateAlert() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');

  return useMutation({
    mutationFn: ({ id, ...body }: { id: string } & UpdateAlertInput) =>
      api.put<AlertRule>(`/alerts/${id}`, body).then(assertSuccess),
    meta: { success: () => t('toast.alertUpdated') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['alerts'] });
    },
  });
}

export function useDeleteAlert() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');

  return useMutation({
    mutationFn: (id: string) => api.del(`/alerts/${id}`).then(assertSuccess),
    meta: { success: () => t('toast.alertDeleted') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['alerts'] });
    },
  });
}

/**
 * Fires a sample alert through a rule's real destinations. The backend
 * applies the same per-destination severity filtering a real alert
 * would, so a test that reaches nobody is a true answer, not a bug.
 */
export function useTestAlert() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');

  return useMutation({
    mutationFn: (id: string) => api.post<AlertTestResult>(`/alerts/${id}/test`, {}).then(assertSuccess),
    meta: { success: () => t('toast.alertTestSent') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['alert-events'] });
    },
  });
}

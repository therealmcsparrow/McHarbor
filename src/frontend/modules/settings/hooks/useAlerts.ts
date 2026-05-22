// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api, type PaginatedData } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export type AlertSeverity = 'critical' | 'warning' | 'info';
export type AlertType = 'cpu' | 'memory' | 'disk' | 'container_down' | 'image_update';

export type AlertRule = {
  id: string;
  name: string;
  severity: AlertSeverity;
  type: AlertType;
  condition: string;
  target: string;
  channelId: string;
  sendInApp: boolean;
  enabled: boolean;
  createdAt: string;
  updatedAt: string;
};

export type CreateAlertInput = {
  name: string;
  severity: AlertSeverity;
  type: AlertType;
  condition: string;
  target: string;
  channelId: string;
  sendInApp: boolean;
};

export type UpdateAlertInput = {
  name?: string;
  severity?: AlertSeverity;
  type?: AlertType;
  condition?: string;
  target?: string;
  channelId?: string;
  sendInApp?: boolean;
  enabled?: boolean;
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
      queryClient.invalidateQueries({ queryKey: ['alerts'] });
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
      queryClient.invalidateQueries({ queryKey: ['alerts'] });
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
      queryClient.invalidateQueries({ queryKey: ['alerts'] });
    },
  });
}

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { TLSStatus } from '../types';

export function useTLSStatus() {
  return useQuery({
    queryKey: ['settings', 'tls'],
    queryFn: () => api.get<TLSStatus>('/settings/tls').then((r) => r.data),
  });
}

export function useSaveTLS() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');
  return useMutation({
    mutationFn: (data: { cert?: string; key?: string; enabled?: boolean; forceHttps?: boolean }) =>
      api.put<TLSStatus>('/settings/tls', data).then(assertSuccess),
    meta: { success: t('toast.tlsUpdated') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['settings', 'tls'] });
    },
  });
}

export function getCertExpiryStatus(notAfter: string): {
  label: string;
  variant: 'success' | 'warning' | 'destructive';
} {
  const expiry = new Date(notAfter);
  const now = new Date();
  const daysLeft = Math.floor((expiry.getTime() - now.getTime()) / (1000 * 60 * 60 * 24));

  if (daysLeft < 0)
    return { label: `Expired ${Math.abs(daysLeft)}d ago`, variant: 'destructive' };
  if (daysLeft < 30) return { label: `Expires in ${daysLeft}d`, variant: 'warning' };
  return { label: `Valid (${daysLeft}d)`, variant: 'success' };
}

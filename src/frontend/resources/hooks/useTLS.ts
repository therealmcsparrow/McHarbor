// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { TLSStatus } from '@core/types/tls';

export function useTLSStatus() {
  return useQuery({
    queryKey: ['settings', 'tls'],
    queryFn: () => api.get<TLSStatus>('/settings/tls').then(assertSuccess),
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
      void queryClient.invalidateQueries({ queryKey: ['settings', 'tls'] });
    },
  });
}

// useRegenerateTLS asks the host to mint a fresh self-signed cert and
// reload the listeners in place. Wired up to the security HTTPS tab;
// the backend route (`POST /api/settings/tls/regenerate`) lands in
// Phase 7 of the default-HTTPS rollout.
export function useRegenerateTLS() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');
  return useMutation({
    mutationFn: () => api.post<TLSStatus>('/settings/tls/regenerate', {}).then(assertSuccess),
    meta: { success: t('toast.tlsRegenerated') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['settings', 'tls'] });
    },
  });
}

export function useDownloadCA() {
  const { t } = useTranslation('security');
  return useMutation({
    mutationFn: async () => {
      const response = await api.download('/settings/tls/ca');
      if (!response.ok) {
        throw new Error(t('https.ca.downloadError'));
      }
      const blob = await response.blob();
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = 'mcharbor-root-ca.pem';
      link.click();
      URL.revokeObjectURL(url);
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

  if (daysLeft < 0) return { label: `Expired ${Math.abs(daysLeft)}d ago`, variant: 'destructive' };
  if (daysLeft < 30) return { label: `Expires in ${daysLeft}d`, variant: 'warning' };
  return { label: `Valid (${daysLeft}d)`, variant: 'success' };
}

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export type BreakGlassSession = {
  id: string;
  userId: string;
  username: string;
  reason: string;
  startedAt: string;
  expiresAt: string;
  endedAt: string | null;
  endedBy: string | null;
};

export type BreakGlassStatus = {
  designated: boolean;
  maxMinutes: number;
  active: BreakGlassSession | null;
};

export type BreakGlassAccount = {
  id: string;
  userId: string;
  username: string;
  maxMinutes: number;
  note: string;
  createdAt: string;
  active: BreakGlassSession | null;
};

const KEY = ['break-glass'] as const;

export function useBreakGlassStatus() {
  return useQuery({
    queryKey: [...KEY, 'status'],
    queryFn: () => api.get<BreakGlassStatus>('/break-glass/status').then(assertSuccess),
  });
}

export function useBreakGlassAccounts(enabled: boolean) {
  return useQuery({
    queryKey: [...KEY, 'accounts'],
    enabled,
    queryFn: () => api.get<BreakGlassAccount[]>('/break-glass/accounts').then(assertSuccess),
  });
}

export function useBreakGlassSessions(enabled: boolean) {
  return useQuery({
    queryKey: [...KEY, 'sessions'],
    enabled,
    queryFn: () => api.get<BreakGlassSession[]>('/break-glass/sessions').then(assertSuccess),
  });
}

function useInvalidate() {
  const client = useQueryClient();
  return () => {
    void client.invalidateQueries({ queryKey: KEY });
    void client.invalidateQueries({ queryKey: ['auth', 'me', 'permissions'] });
  };
}

export function useActivateBreakGlass() {
  const { t } = useTranslation('security');
  const invalidate = useInvalidate();
  return useMutation({
    // The server refuses activation without a live break_glass step-up window,
    // so the password opens one first. It lives only in these variables.
    mutationFn: async ({ password, ...input }: { reason: string; minutes: number; password: string }) => {
      assertSuccess(await api.post('/auth/sudo', { scope: 'break_glass', password }));
      return api.post<BreakGlassSession>('/break-glass/activate', input).then(assertSuccess);
    },
    meta: { success: () => t('governance.breakGlass.activated') },
    onSuccess: invalidate,
  });
}

export function useEndBreakGlass() {
  const { t } = useTranslation('security');
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: (userId: string | null) =>
      (userId ? api.post(`/break-glass/accounts/${userId}/end`) : api.post('/break-glass/end')).then(assertSuccess),
    meta: { success: () => t('governance.breakGlass.ended') },
    onSuccess: invalidate,
  });
}

export function useDesignateBreakGlass() {
  const { t } = useTranslation('security');
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: ({ userId, maxMinutes, note }: { userId: string; maxMinutes: number; note: string }) =>
      api.put(`/break-glass/accounts/${userId}`, { maxMinutes, note }).then(assertSuccess),
    meta: { success: () => t('governance.breakGlass.designated') },
    onSuccess: invalidate,
  });
}

export function useUndesignateBreakGlass() {
  const { t } = useTranslation('security');
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: (userId: string) => api.del(`/break-glass/accounts/${userId}`).then(assertSuccess),
    meta: { success: () => t('governance.breakGlass.undesignated') },
    onSuccess: invalidate,
  });
}

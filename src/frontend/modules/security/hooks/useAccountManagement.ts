// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api, type PaginatedData } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

/* ─── Account status (disable / enable) ───────────────────────── */

export function useDisableAccount() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('security');
  return useMutation({
    mutationFn: ({ userId, reason }: { userId: string; reason: string }) =>
      api.post(`/users/${userId}/disable`, { reason }).then(assertSuccess),
    meta: { success: () => t('toast.accountDisabled') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['users'] });
      void queryClient.invalidateQueries({ queryKey: ['user-sessions'] });
    },
  });
}

export function useEnableAccount() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('security');
  return useMutation({
    mutationFn: (userId: string) => api.post(`/users/${userId}/enable`).then(assertSuccess),
    meta: { success: () => t('toast.accountEnabled') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['users'] });
    },
  });
}

export function useForceResetPassword() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('security');
  return useMutation({
    mutationFn: (userId: string) => api.post(`/users/${userId}/force-reset-password`).then(assertSuccess),
    meta: { success: () => t('toast.forceResetSent') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['users'] });
    },
  });
}

export function useAdminSetPassword() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('security');
  return useMutation({
    mutationFn: ({ userId, newPassword }: { userId: string; newPassword: string }) =>
      api.post(`/users/${userId}/password/admin`, { newPassword }).then(assertSuccess),
    meta: { success: () => t('toast.passwordSet') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['users'] });
    },
  });
}

export function useUnlockAccount() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('security');
  return useMutation({
    mutationFn: (userId: string) => api.post(`/users/${userId}/unlock`).then(assertSuccess),
    meta: { success: () => t('toast.accountUnlocked') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['users'] });
      void queryClient.invalidateQueries({ queryKey: ['user-sessions'] });
    },
  });
}

// Re-export PaginatedData so other security hooks can import alongside.
export type { PaginatedData };

// Session hooks moved to @resources/hooks/useSessions so the profile module can
// use them without importing across modules. Re-exported here so existing
// security-module call sites keep their import path.
export {
  useUserSessions,
  useRevokeUserSession,
  useRevokeMySession,
  useRevokeAllUserSessions,
  useRevokeAllMySessions,
  type SessionView,
} from '@resources/hooks/useSessions';

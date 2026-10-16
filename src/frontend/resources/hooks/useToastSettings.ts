// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { useAuth } from '@core/auth/useAuth';
import type { ToastSettings } from '@resources/types/settings';

/** The query key the toaster and both settings cards share. */
export const TOAST_SETTINGS_KEY = ['settings', 'toast'] as const;

/**
 * The app-wide toast defaults.
 *
 * `GET /settings/toast` is authenticated but not permission-gated, because
 * every signed-in user needs these to render their own toasts — not just the
 * operators who can change them.
 *
 * `staleTime` is deliberately long: the toaster reads this on every page and
 * the answer changes about once a year. An operator's own save invalidates
 * the key, so they still see their change immediately.
 */
export function useToastSettings(options: { enabled?: boolean } = {}) {
  return useQuery({
    queryKey: TOAST_SETTINGS_KEY,
    queryFn: () => api.get<ToastSettings>('/settings/toast').then(assertSuccess),
    staleTime: 5 * 60 * 1000,
    // The toaster mounts on the login screen too, where the session cookie
    // does not exist yet and this would 401 on every render.
    enabled: options.enabled ?? true,
  });
}

export function useSaveToastSettings() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');
  return useMutation({
    mutationFn: (data: ToastSettings) => api.put('/settings/toast', data).then(assertSuccess),
    meta: { success: t('toast.toastSettingsUpdated') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: TOAST_SETTINGS_KEY });
    },
  });
}

/** How many accounts a reset actually touched. */
type ResetOverridesResult = { reset: number };

/**
 * Clears every account's toast overrides so the whole install follows the
 * app-wide default again.
 *
 * The session is re-read too: the operator running this may well have had an
 * override of their own, and their toaster has to stop using it without
 * waiting for a reload. `checkSession` rather than a query invalidation —
 * the signed-in user is Zustand state, not a TanStack query, so there is no
 * key to invalidate.
 */
export function useResetToastOverrides() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');
  return useMutation({
    mutationFn: () => api.post<ResetOverridesResult>('/settings/toast/reset-overrides', {}).then(assertSuccess),
    meta: {
      success: (data: ResetOverridesResult | undefined) => t('toast.toastOverridesReset', { count: data?.reset ?? 0 }),
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: TOAST_SETTINGS_KEY });
      void useAuth.getState().checkSession();
    },
  });
}

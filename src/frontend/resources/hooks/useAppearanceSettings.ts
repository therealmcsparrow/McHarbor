// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { useAuth } from '@core/auth/useAuth';
import type { AppearanceSettings } from '@resources/types/settings';

/** The query key the token applier and both appearance cards share. */
export const APPEARANCE_SETTINGS_KEY = ['settings', 'appearance'] as const;

/**
 * The app-wide appearance defaults.
 *
 * `GET /settings/appearance` is authenticated but not permission-gated,
 * because every signed-in user needs these to paint their own theme and font
 * — not just the operators who can change them.
 *
 * `staleTime` is deliberately long: this drives the paint on every page and
 * the answer changes about as often as an operator opens Settings. Their own
 * save invalidates the key, so they still see the change immediately.
 */
export function useAppearanceSettings(options: { enabled?: boolean } = {}) {
  return useQuery({
    queryKey: APPEARANCE_SETTINGS_KEY,
    queryFn: () => api.get<AppearanceSettings>('/settings/appearance').then(assertSuccess),
    staleTime: 5 * 60 * 1000,
    // The applier runs on the login screen too, where the session cookie does
    // not exist yet and this would 401 on every render.
    enabled: options.enabled ?? true,
  });
}

export function useSaveAppearanceSettings() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');
  return useMutation({
    mutationFn: (data: AppearanceSettings) => api.put('/settings/appearance', data).then(assertSuccess),
    meta: { success: t('toast.appearanceSettingsUpdated') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: APPEARANCE_SETTINGS_KEY });
    },
  });
}

/** How many accounts a reset actually touched. */
type ResetOverridesResult = { reset: number };

/**
 * Clears every account's appearance overrides so the whole install follows
 * the app-wide default again.
 *
 * The session is re-read too: the operator running this may well have had an
 * override of their own, and their own UI has to stop using it without
 * waiting for a reload. `checkSession` rather than a query invalidation — the
 * signed-in user is Zustand state, not a TanStack query, so there is no key
 * to invalidate.
 */
export function useResetAppearanceOverrides() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');
  return useMutation({
    mutationFn: () => api.post<ResetOverridesResult>('/settings/appearance/reset-overrides', {}).then(assertSuccess),
    meta: {
      success: (data: ResetOverridesResult | undefined) =>
        t('toast.appearanceOverridesReset', { count: data?.reset ?? 0 }),
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: APPEARANCE_SETTINGS_KEY });
      void useAuth.getState().checkSession();
    },
  });
}

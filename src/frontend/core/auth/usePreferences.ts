// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { useAuth, type ToastPosition, type User } from './useAuth';
import type { ThemeAccent, ThemeFont } from '@resources/stores/themeCustomizer';

export type UpdatePreferencesInput = {
  timeFormat?: '12h' | '24h';
  dateFormat?: 'ddmmyyyy' | 'mmddyyyy';

  // clearToastOverride drops this account's toast overrides so it follows
  // the app-wide default again. It is a separate flag rather than a set of
  // nulls because an omitted field already means "leave unchanged" — there
  // would otherwise be no way to say "unset this".
  clearToastOverride?: boolean;
  toastPosition?: ToastPosition;
  toastDurationMs?: number;
  toastMaxVisible?: number;
  toastRichColors?: boolean;
  toastCloseButton?: boolean;

  // clearAppearanceOverride is the appearance equivalent of
  // clearToastOverride: it drops this account's theme, accent, font and base
  // font size so it follows the app-wide default again.
  clearAppearanceOverride?: boolean;
  appearanceThemeId?: string;
  appearanceAccent?: ThemeAccent;
  appearanceFont?: ThemeFont;
  appearanceBaseFontPx?: number;
};

// useUpdatePreferences writes per-user UI preferences to
// /auth/preferences. The same endpoint handles preferredLanguage,
// timeFormat, dateFormat and the toast overrides; this hook wraps the call so
// components don't need to know the URL or the response shape.
//
// The auth store is kept in sync after the server confirms the
// change so the new preference immediately drives downstream
// formatters (e.g. useLocaleFormat).
export function useUpdatePreferences(operation?: string) {
  return useMutation({
    meta: { error: operation },
    mutationFn: (input: UpdatePreferencesInput) => api.put<User>('/auth/preferences', input).then(assertSuccess),
    onSuccess: (user) => {
      // The endpoint returns the updated user, and the auth store is the
      // only subscriber that matters: `session` is Zustand state populated
      // by checkSession(), not a TanStack query, so invalidating a query key
      // for it would notify nothing. Writing the fresh user straight into
      // the store is what makes a saved preference take effect without a
      // reload — which the toast settings depend on, since the toaster
      // reads its position and duration from this same user object.
      if (user) {
        useAuth.setState({ user });
      }
    },
  });
}

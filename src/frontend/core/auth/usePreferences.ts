// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export type UpdatePreferencesInput = {
  timeFormat?: '12h' | '24h';
  dateFormat?: 'ddmmyyyy' | 'mmddyyyy';
};

// useUpdatePreferences writes per-user UI preferences to
// /auth/preferences. The same endpoint handles preferredLanguage,
// timeFormat, and dateFormat; this hook wraps the call so
// components don't need to know the URL or the response shape.
//
// The auth store is kept in sync after the server confirms the
// change so the new preference immediately drives downstream
// formatters (e.g. useLocaleFormat).
export function useUpdatePreferences() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: UpdatePreferencesInput) =>
      api.put('/auth/preferences', input).then(assertSuccess),
    onSuccess: (data) => {
      // The /auth/preferences endpoint returns the updated user.
      // Invalidate the auth-check query so the next session
      // re-read picks up the new fields. Components reading from
      // the auth store update immediately because the success
      // handler returns the fresh user object.
      void queryClient.invalidateQueries({ queryKey: ['auth', 'session'] });
      void data;
    },
  });
}

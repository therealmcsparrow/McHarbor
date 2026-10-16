// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

type SettingRow = {
  id: string;
  key: string;
  value: string;
  category: string;
  updatedAt: string;
};

/**
 * Automatic sign-out is stored as a single settings row holding a number of
 * seconds, where 0 means "never". Keeping it as one value is what lets the
 * card show a switch and a duration without the two ever disagreeing.
 */
export const IDLE_KEY = 'security.session_idle_timeout_seconds';

/** Mirrors the bounds enforced in core/auth/session_policy.go. */
export const DEFAULT_IDLE_SECONDS = 600;
export const MIN_IDLE_SECONDS = 60;
export const MAX_IDLE_SECONDS = 86400;

/** Reads the configured idle timeout; null when the row is absent or unparseable. */
export function useIdleTimeout() {
  return useQuery({
    queryKey: ['settings', IDLE_KEY],
    queryFn: async () => {
      const res = await api.get<SettingRow>(`/settings/${IDLE_KEY}`);
      if (!res.success || !res.data) return null;
      const seconds = parseInt(res.data.value, 10);
      return Number.isFinite(seconds) ? seconds : null;
    },
  });
}

/** Writes the idle timeout in seconds; 0 disables automatic sign-out. */
export function useSaveIdleTimeout() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (seconds: number) =>
      api
        .put(`/settings/${IDLE_KEY}`, {
          key: IDLE_KEY,
          value: String(seconds),
          category: 'security',
        })
        .then(assertSuccess),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['settings', IDLE_KEY] });
    },
  });
}

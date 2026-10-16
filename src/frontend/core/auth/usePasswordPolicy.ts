// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { DEFAULT_PASSWORD_POLICY, type PasswordPolicy } from './passwordPolicy';

/**
 * Reads the runtime password policy from `GET /auth/password-policy`.
 *
 * The endpoint is unauthenticated (it sits beside /auth/status) so the
 * setup wizard and the token-based reset page can call it while logged
 * out. On any failure the hook falls back to `DEFAULT_PASSWORD_POLICY`
 * — never to "no policy" — so a failed request cannot let a weak
 * password sail past the client-side check.
 *
 * The policy changes only when an admin edits it, so it is cached for
 * five minutes rather than refetched per keystroke.
 */
export function usePasswordPolicy() {
  const query = useQuery({
    queryKey: ['auth', 'password-policy'],
    queryFn: async (): Promise<PasswordPolicy> => {
      const res = await api.get<PasswordPolicy>('/auth/password-policy');
      if (!res.success || !res.data) {
        return DEFAULT_PASSWORD_POLICY;
      }
      return { ...DEFAULT_PASSWORD_POLICY, ...res.data };
    },
    staleTime: 5 * 60 * 1000,
    retry: 1,
  });

  return {
    policy: query.data ?? DEFAULT_PASSWORD_POLICY,
    isLoading: query.isLoading,
  };
}

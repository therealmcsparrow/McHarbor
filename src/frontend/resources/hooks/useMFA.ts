// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

// === MFA / 2FA hooks ===
//
// These hooks live with the security module because MFA is an
// authentication concern. They were previously lumped into
// `containers/hooks/useContainerAdvanced2.ts` along with unrelated
// container/image hooks, which made `profile/MFASettingsPanel.tsx`
// import from the containers module (a cross-module violation).
// Extracting them here lets the profile module depend only on
// `security` for MFA state.

export type MFAStatus = {
  enabled: boolean;
  enrolledAt?: string;
  lastUsedAt?: string;
  hasRecovery: boolean;
  /** Unused recovery codes left, so the UI can warn before they run out. */
  recoveryRemaining: number;
};

export type MFAEnrollment = {
  secret: string;
  otpauthUri: string;
  recoveryCodes: string[];
};

// Every call goes through `assertSuccess`. The API client resolves
// non-2xx responses as `{ success: false, error }` instead of
// rejecting, so a bare `.then(r => r.data)` turns a failed request
// into a *successful* mutation carrying `undefined` — which is how
// "click Enable, nothing happens" used to look.
export function useMFAStatus() {
  return useQuery({
    queryKey: ['mfa-status'],
    queryFn: () => api.get<MFAStatus>('/auth/mfa/status').then(assertSuccess),
  });
}

export function useMFABeginEnrollment() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => api.post<MFAEnrollment>('/auth/mfa/enroll/begin', {}).then(assertSuccess),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['mfa-status'] });
    },
  });
}

export function useMFAConfirmEnrollment() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (payload: { code: string }) =>
      api.post<{ ok: boolean }>('/auth/mfa/enroll/confirm', payload).then(assertSuccess),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['mfa-status'] });
    },
  });
}

export function useMFADisable() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => api.del<{ disabled: boolean }>('/auth/mfa').then(assertSuccess),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['mfa-status'] });
    },
  });
}

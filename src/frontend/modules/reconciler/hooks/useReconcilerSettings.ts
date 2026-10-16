// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { refusalOf } from './refusal';

export type ReconcilerSettings = {
  confirmPasses: number;
  enforceEnabled: boolean;
  maxActionsPerPass: number;
  cooldownSeconds: number;
  failureThreshold: number;
  breakerOpen: boolean;
  breakerReason?: string;
  breakerOpenedAt?: string;
  consecutiveFailures: number;
};

export type EnforcementInput = Pick<
  ReconcilerSettings,
  'enforceEnabled' | 'maxActionsPerPass' | 'cooldownSeconds' | 'failureThreshold'
>;

const KEY = ['reconciler-settings'] as const;

export function useReconcilerSettings(enabled = true) {
  return useQuery({
    queryKey: KEY,
    enabled,
    queryFn: () => api.get<ReconcilerSettings>('/reconciler/settings').then(assertSuccess),
    refetchInterval: 30_000,
  });
}

function useSettingsMutation<V>(fn: (v: V) => Promise<ReconcilerSettings>) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSuccess: (data) => client.setQueryData(KEY, data),
  });
}

export function useSetConfirmPasses() {
  return useSettingsMutation((confirmPasses: number) =>
    api.put<ReconcilerSettings>('/reconciler/settings', { confirmPasses }).then(refusalOf),
  );
}

export function useSetEnforcement() {
  return useSettingsMutation((input: EnforcementInput) =>
    api.put<ReconcilerSettings>('/reconciler/enforcement', input).then(refusalOf),
  );
}

export function useResetBreaker() {
  return useSettingsMutation(() => api.post<ReconcilerSettings>('/reconciler/enforcement/reset').then(refusalOf));
}

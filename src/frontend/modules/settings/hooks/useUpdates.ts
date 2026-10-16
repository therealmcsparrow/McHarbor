// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api, type PaginatedData } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { runWithMcHarborRestart } from '@resources/stores/mcharbor-restart';

import type {
  VersionCheck,
  SelfUpdateState,
  SelfUpdateSettings,
  AgentUpdateResponse,
  UpdatePolicy,
  CreatePolicyInput,
  UpdatePolicyInput,
  FactoryResetResponse,
  SelfUpdateApplyResponse,
} from './updates-types';
import { updatesKeys } from './queryKeys';

export * from './updates-types';

export function useCheckUpdate() {
  return useQuery({
    queryKey: updatesKeys.check,
    queryFn: async () => {
      const res = await api.get<VersionCheck>('/updates/check');
      if (!res.success || !res.data) {
        throw new Error(res.error ?? 'Update check failed');
      }
      return res.data;
    },
    staleTime: 5 * 60_000,
    enabled: false,
  });
}

// useSelfUpdateState polls the cached self-update result. The
// background checker on the server keeps the result fresh, so the
// frontend just reads it. A short polling interval lets the in-app
// banner react within a minute of a new release appearing.
export function useSelfUpdateState(intervalMs = 60_000) {
  return useQuery({
    queryKey: updatesKeys.self,
    queryFn: async () => {
      const res = await api.get<SelfUpdateState>('/updates/state');
      if (!res.success || !res.data) {
        throw new Error(res.error ?? 'Update state unavailable');
      }
      return res.data;
    },
    refetchInterval: intervalMs,
    refetchOnWindowFocus: true,
    staleTime: 30_000,
  });
}

export function useSelfUpdateSettings() {
  return useQuery({
    queryKey: updatesKeys.selfSettings,
    queryFn: async () => {
      const res = await api.get<SelfUpdateSettings>('/updates/settings');
      if (!res.success || !res.data) {
        throw new Error(res.error ?? 'Update settings unavailable');
      }
      return res.data;
    },
  });
}

export function useSaveSelfUpdateSettings() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');
  return useMutation({
    mutationFn: (input: SelfUpdateSettings) => api.put('/updates/settings', input).then(assertSuccess),
    meta: { success: t('toast.updateCheckSettingsSaved') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: updatesKeys.self });
    },
  });
}

// useUpdateAgentsNow asks every connected, outdated remote agent to
// replace itself right away (POST /api/updates/agents/update). The call
// returns once each agent acknowledged or failed; agents that started an
// update reconnect with the new version a little later.
export function useUpdateAgentsNow() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');
  return useMutation({
    mutationFn: () =>
      api.post<AgentUpdateResponse>('/updates/agents/update', {}).then((res) => {
        if (!res.success || !res.data) {
          throw new Error(res.error ?? 'agent update failed');
        }
        return res.data;
      }),
    meta: {
      success: (data: unknown) => {
        const r = data as AgentUpdateResponse | undefined;
        return r?.message ?? t('toast.agentUpdateTriggered');
      },
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: updatesKeys.self });
      void queryClient.invalidateQueries({ queryKey: ['agents'] });
      void queryClient.invalidateQueries({ queryKey: ['versions'] });
    },
  });
}

// useResetSystem triggers a McHarbor factory reset via
// POST /api/system/reset. The backend requires the literal string
// "RESET" in the `confirm` field as a double-confirmation guard;
// the helper container wipes `/app/data` and recreates the McHarbor
// container. Like the self-update mutation, onSuccess fires before
// the API process exits so the toast can surface the scheduled
// message before the websocket is torn down.
export function useResetSystem() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');
  return useMutation({
    mutationFn: (confirmToken: string) =>
      runWithMcHarborRestart(() =>
        api
          .post<FactoryResetResponse>('/system/reset', {
            confirm: confirmToken,
          })
          .then((res) => {
            if (!res.success) {
              throw new Error(res.error ?? 'factory reset failed');
            }
            return assertSuccess(res);
          }),
      ),
    meta: {
      success: (data: unknown) => {
        const r = data as FactoryResetResponse | undefined;
        return r?.message ?? t('about.toast.resetScheduled');
      },
      error: t('about.toast.resetFailed'),
    },
    onSuccess: () => {
      void queryClient.invalidateQueries();
    },
  });
}

export function useDismissUpdate() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');
  return useMutation({
    mutationFn: (version: string) => api.post('/updates/dismiss', { version }).then(assertSuccess),
    meta: { success: t('toast.updateCheckDismissed') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: updatesKeys.self });
    },
  });
}

// useApplySelfUpdate triggers a McHarbor self-update via
// POST /api/versions/self-update. The current process exits shortly
// after the helper starts; the mutation's onSuccess fires before that,
// so the toast can surface the localized "scheduled" message before
// the websocket is torn down. The `image` field is optional; when
// empty the current image is reused.
export function useApplySelfUpdate() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');
  return useMutation({
    mutationFn: (input: { image?: string }) =>
      runWithMcHarborRestart(() =>
        api.post<SelfUpdateApplyResponse>('/versions/self-update', input).then((res) => {
          if (!res.success) {
            throw new Error(res.error ?? 'self-update failed');
          }
          return assertSuccess(res);
        }),
      ),
    meta: {
      success: (data: unknown) => {
        const r = data as SelfUpdateApplyResponse | undefined;
        return r?.message ?? t('about.toast.selfUpdateScheduled');
      },
      error: t('about.toast.selfUpdateFailed'),
    },
    onSuccess: () => {
      void queryClient.invalidateQueries();
    },
  });
}

export function useUpdatePolicies() {
  return useQuery({
    queryKey: updatesKeys.policies,
    queryFn: () => api.get<PaginatedData<UpdatePolicy>>('/updates').then((r) => r.data?.items ?? []),
  });
}

export function useCreateUpdatePolicy() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');
  return useMutation({
    mutationFn: (input: CreatePolicyInput) => api.post('/updates', input).then(assertSuccess),
    meta: { success: t('toast.policyCreated') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: updatesKeys.policies });
    },
  });
}

export function useUpdateUpdatePolicy() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');
  return useMutation({
    mutationFn: ({ id, ...input }: UpdatePolicyInput & { id: string }) =>
      api.put(`/updates/${id}`, input).then(assertSuccess),
    meta: { success: t('toast.policyUpdated') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: updatesKeys.policies });
    },
  });
}

export function useDeleteUpdatePolicy() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');
  return useMutation({
    mutationFn: (id: string) => api.del(`/updates/${id}`).then(assertSuccess),
    meta: { success: t('toast.policyDeleted') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: updatesKeys.policies });
    },
  });
}

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { useEnvironmentStore } from '@resources/stores/environment';
import { useEnvironmentList, type EnvironmentListItem } from '@resources/hooks/useEnvironmentList';
import type { EnvironmentInfo, EnvironmentTestResult } from './useEnvironments';

export { useEnvironmentList, type EnvironmentListItem };
export * from './useAgentDeploy';

export type CreateEnvironmentData = {
  name: string;
  connectionType?: string;
  socketPath?: string;
  host?: string;
  port?: number;
  /**
   * Existing agent token to import. When the operator has already
   * deployed an McHarbor agent on another host (typical multi-host
   * setups where one control plane points at several agents), they
   * paste the existing plaintext here instead of receiving a fresh
   * token. The backend validates the `agt_<64 hex>` shape, hashes +
   * stores it, and does NOT echo it back in the response (the user
   * already has it).
   */
  agentToken?: string;
};

export type CreateResponse = {
  environment?: EnvironmentListItem;
  agentToken?: string;
} & EnvironmentListItem;

export type UpdateEnvironmentData = {
  name?: string;
  connectionType?: string;
  socketPath?: string;
  host?: string;
  port?: number;
  isDefault?: boolean;
  isActive?: boolean;
  scheduledUpdateCheckEnabled?: boolean;
  automaticImagePruningEnabled?: boolean;
  trackContainerEventsEnabled?: boolean;
  collectContainerMetricsEnabled?: boolean;
  highlightContainerChangesEnabled?: boolean;
  dockerDiskUsageNotificationsEnabled?: boolean;
  dockerDiskUsageThresholdPercent?: number;
  timezone?: string;
  // Omitting a secret leaves the stored one in place — the backend COALESCEs.
};

export function useCreateEnvironment() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('environments');
  return useMutation({
    mutationFn: (data: CreateEnvironmentData) => api.post<CreateResponse>('/environments', data).then(assertSuccess),
    meta: { success: t('toast.created') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['environments'] });
    },
  });
}

export function useTestEnvironment() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('environments');
  return useMutation<EnvironmentTestResult, Error, string>({
    mutationFn: (id: string) =>
      api.post<EnvironmentTestResult>(`/environments/${id}/test`).then((res) => {
        if (!res.success) {
          throw new Error(res.error ?? 'Test connection failed');
        }
        return assertSuccess(res);
      }),
    onSuccess: (result) => {
      // Best-effort toast on top of the dialog the caller renders.
      if (result.overall === 'pass') {
        toast.success(t('toast.connectionSuccessful'));
      }
      void queryClient.invalidateQueries({ queryKey: ['environments'] });
      void queryClient.invalidateQueries({ queryKey: ['environments', result.envId] });
    },
  });
}

export function useUpdateEnvironment() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('environments');

  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: UpdateEnvironmentData }) =>
      api.put<EnvironmentInfo>(`/environments/${id}`, data).then(assertSuccess),
    meta: { success: t('toast.updated') },
    onSuccess: (environment, { id }) => {
      queryClient.setQueryData(['environments', id], environment);
      queryClient.setQueryData<EnvironmentListItem[] | undefined>(['environments'], (current) => {
        if (!current) {
          return current;
        }

        return current.map((item) => (item.id === environment.id ? { ...item, ...environment } : item));
      });
      useEnvironmentStore.getState().upsertEnvironment(environment);
      if (!environment.collectContainerMetricsEnabled) {
        queryClient.removeQueries({
          queryKey: ['containers-bulk-stats', environment.id],
          exact: true,
        });
        queryClient.removeQueries({ queryKey: ['dashboard-stats', environment.id], exact: true });
      }
      void queryClient.invalidateQueries({ queryKey: ['environments'] });
      void queryClient.invalidateQueries({ queryKey: ['environments', id] });
    },
  });
}

export function useRemoveEnvironment() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('environments');
  return useMutation({
    mutationFn: (id: string) => api.del(`/environments/${id}`).then(assertSuccess),
    meta: { success: t('toast.removed') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['environments'] });
    },
  });
}

// --- Install Token ---

export type InstallTokenResponse = {
  token: string;
  agentToken?: string;
  expiresAt: string;
  script: string;
  serverUrl?: string;
  agentImage?: string;
  transferListen?: string;
  transferAdvertiseUrl?: string;
  // forceTls reflects whether the host is HTTPS-only. The install-token
  // endpoint derives this from the request scheme so the agent dials
  // wss:// instead of ws:// when the host is HTTPS.
  forceTls?: boolean;
  caFingerprint?: string;
};

export function useCreateInstallToken() {
  return useMutation({
    mutationFn: (envId: string) => api.post<InstallTokenResponse>(`/agents/${envId}/install-token`).then(assertSuccess),
  });
}

export function useRegenerateToken() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('environments');

  return useMutation({
    mutationFn: (envId: string) =>
      api.post<{ token: string; caFingerprint?: string }>(`/agents/${envId}/regenerate-token`).then(assertSuccess),
    meta: { success: t('toast.tokenRegenerated') },
    onSuccess: (_, envId) => {
      void queryClient.invalidateQueries({ queryKey: ['environments', envId] });
    },
  });
}

export function deriveEnvironmentStatus(
  env: Pick<EnvironmentListItem, 'connectionType' | 'agentStatus' | 'dockerVersion' | 'lastConnected'>,
): string {
  if (env.connectionType === 'agent') {
    return env.agentStatus ?? 'disconnected';
  }
  if (env.dockerVersion) return 'connected';
  if (env.lastConnected) return 'disconnected';
  return 'disconnected';
}

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { useEnvironmentStore } from '@resources/stores/environment';
import { useEnvironmentList, type EnvironmentListItem } from '@resources/hooks/useEnvironmentList';
import type { EnvironmentInfo } from './useEnvironments';

export { useEnvironmentList, type EnvironmentListItem };

export type CreateEnvironmentData = {
  name: string;
  orchestratorType: string;
  connectionType?: string;
  socketPath?: string;
  host?: string;
  port?: number;
  kubeconfig?: string;
  k8sNamespace?: string;
  k8sServerUrl?: string;
  k8sBearerToken?: string;
};

export type CreateResponse = {
  environment?: EnvironmentListItem;
  agentToken?: string;
} & EnvironmentListItem;

export type UpdateEnvironmentData = {
  name?: string;
  orchestratorType?: string;
  connectionType?: string;
  socketPath?: string;
  host?: string;
  port?: number;
  isDefault?: boolean;
  isActive?: boolean;
  kubeconfig?: string;
  k8sNamespace?: string;
  k8sServerUrl?: string;
  k8sBearerToken?: string;
  scheduledUpdateCheckEnabled?: boolean;
  automaticImagePruningEnabled?: boolean;
  trackContainerEventsEnabled?: boolean;
  collectContainerMetricsEnabled?: boolean;
  highlightContainerChangesEnabled?: boolean;
  dockerDiskUsageNotificationsEnabled?: boolean;
  dockerDiskUsageThresholdPercent?: number;
  timezone?: string;
};

export function useCreateEnvironment() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('environments');
  return useMutation({
    mutationFn: (data: CreateEnvironmentData) =>
      api.post<CreateResponse>('/environments', data).then(assertSuccess),
    meta: { success: t('toast.created') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['environments'] });
    },
  });
}

export function useTestEnvironment() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('environments');
  return useMutation({
    mutationFn: (id: string) => api.post(`/environments/${id}/test`).then(assertSuccess),
    meta: { success: t('toast.connectionSuccessful') },
    onSuccess: (_, id) => {
      queryClient.invalidateQueries({ queryKey: ['environments'] });
      queryClient.invalidateQueries({ queryKey: ['environments', id] });
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
        queryClient.removeQueries({ queryKey: ['containers-bulk-stats', environment.id], exact: true });
        queryClient.removeQueries({ queryKey: ['dashboard-stats', environment.id], exact: true });
      }
      queryClient.invalidateQueries({ queryKey: ['environments'] });
      queryClient.invalidateQueries({ queryKey: ['environments', id] });
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
      queryClient.invalidateQueries({ queryKey: ['environments'] });
    },
  });
}

// --- Agent Deploy ---

export type DeployRequest = {
  sshHost: string;
  sshPort: number;
  sshUser: string;
  sshAuthType: 'key' | 'password';
  sshKey?: string;
  sshPassword?: string;
  hostKeyFingerprint: string;
  method: 'docker' | 'binary';
  agentImage?: string;
};

export type DeployResult = {
  success: boolean;
  output?: string;
  error?: string;
  code?: string;
  os?: string;
  arch?: string;
};

export function useDeployAgent() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('environments');
  return useMutation({
    mutationFn: ({ envId, data }: { envId: string; data: DeployRequest }) =>
      api.post<DeployResult>(`/agents/${envId}/deploy`, data).then(assertSuccess).then((result) => {
        if (!result.success) {
          throw new Error(result.error ?? t('toast.agentDeployFailed'));
        }
        return result;
      }),
    meta: { success: t('toast.agentDeployed') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['environments'] });
    },
  });
}

// --- Install Token ---

export type InstallTokenResponse = {
  token: string;
  expiresAt: string;
  script: string;
};

export function useCreateInstallToken() {
  return useMutation({
    mutationFn: (envId: string) =>
      api.post<InstallTokenResponse>(`/agents/${envId}/install-token`).then(assertSuccess),
  });
}

export function useRegenerateToken() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('environments');

  return useMutation({
    mutationFn: (envId: string) =>
      api.post<{ token: string }>(`/agents/${envId}/regenerate-token`).then(assertSuccess),
    meta: { success: t('toast.tokenRegenerated') },
    onSuccess: (_, envId) => {
      queryClient.invalidateQueries({ queryKey: ['environments', envId] });
    },
  });
}

export function deriveEnvironmentStatus(env: EnvironmentListItem): string {
  if (env.connectionType === 'agent') {
    return env.agentStatus ?? 'disconnected';
  }
  if (env.dockerVersion || env.k8sVersion) return 'connected';
  if (env.lastConnected) return 'disconnected';
  return 'disconnected';
}

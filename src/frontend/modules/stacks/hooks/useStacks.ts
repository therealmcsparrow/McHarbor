// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import type { ContainerInfo } from '@core/types/docker';
import { useStacksForEnvironment as useScopedStacksForEnvironment } from '@resources/hooks/useStacksForEnvironment';
import { assertSuccess } from '@resources/utils/api-mutation';
import { useEnvironmentStore } from '@resources/stores/environment';
import type {
  StackWebhook,
  CreateStackWebhookInput,
  UpdateStackWebhookInput,
  PruneResult,
} from '../types/stack-webhook';

export type StackSvc = {
  name: string;
  containerId?: string;
  status: string;
  image: string;
};

export type StackInfo = {
  id: string;
  name: string;
  status: string;
  services: StackSvc[];
  description?: string;
  type: 'managed' | 'discovered';
};

export function useStacks() {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['stacks', envId],
    queryFn: () =>
      api
        .get<StackInfo[]>('/stacks', envId ? { env: envId } : {})
        .then((r) => r.data ?? []),
    refetchInterval: 15_000,
  });
}

export function useStacksForEnvironment(envId?: string, enabled = true) {
  return useScopedStacksForEnvironment(envId, enabled);
}

export function useStackAction() {
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);
  const { t } = useTranslation('stacks');
  return useMutation({
    mutationFn: ({ name, action }: { name: string; action: string }) =>
      api.post(`/stacks/${name}/${action}${envId ? `?env=${envId}` : ''}`).then(assertSuccess),
    meta: {
      success: (_: unknown, vars: unknown) => {
        const { action } = vars as { action: string };
        const labels: Record<string, string> = {
          up: t('toast.started'),
          stop: t('toast.stopped'),
          down: t('toast.down'),
          restart: t('toast.restarted'),
        };
        return labels[action] ?? t('toast.actionCompleted');
      },
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['stacks'] });
    },
  });
}

export function useDeleteStack() {
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);
  const { t } = useTranslation('stacks');
  return useMutation({
    mutationFn: (name: string) =>
      api.del(`/stacks/${name}`, envId ? { env: envId } : {}).then(assertSuccess),
    meta: { success: t('toast.removed') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['stacks'] });
    },
  });
}

export function useStackLogs(name: string | null, tail = 500) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['stack-logs', envId, name, tail],
    queryFn: () =>
      api
        .get<Record<string, string>>(`/stacks/${name}/logs`, {
          tail: String(tail),
          ...(envId ? { env: envId } : {}),
        })
        .then((r) => r.data ?? {}),
    enabled: !!name,
  });
}

export function useStackCompose(name: string | null) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['stack-compose', envId, name],
    queryFn: () =>
      api
        .get<{ content: string }>(`/stacks/${name}/compose`, envId ? { env: envId } : {})
        .then((r) => r.data?.content ?? ''),
    enabled: !!name,
  });
}

export function useUpdateStack() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('stacks');
  return useMutation({
    mutationFn: ({ name, compose, description }: { name: string; compose?: string; description?: string }) =>
      api.put(`/stacks/${name}`, { compose, description }).then(assertSuccess),
    meta: { success: t('toast.updated') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['stacks'] });
    },
  });
}

export type StackDetail = {
  id: string;
  name: string;
  status: string;
  services: StackSvc[];
  description?: string;
  type: 'managed' | 'discovered';
  environmentId?: string;
  createdAt: string;
  updatedAt: string;
};

export function useStack(name: string) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['stack', envId, name],
    queryFn: () =>
      api
        .get<StackDetail>(`/stacks/${name}`, envId ? { env: envId } : {})
        .then((r) => {
          if (!r.data) {
            throw new Error('stack response missing data');
          }
          return r.data;
        }),
    refetchInterval: 10_000,
    enabled: !!name,
  });
}

export function useStackContainers(name: string) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['stack-containers', envId, name],
    queryFn: () =>
      api
        .get<ContainerInfo[]>(`/stacks/${name}/containers`, envId ? { env: envId } : {})
        .then((r) => r.data ?? []),
    refetchInterval: 10_000,
    enabled: !!name,
  });
}

export function useStackEnvVars(name: string | null) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['stack-env-vars', envId, name],
    queryFn: () =>
      api
        .get<Record<string, string>>(`/stacks/${name}/env-vars`, envId ? { env: envId } : {})
        .then((r) => r.data ?? {}),
    enabled: !!name,
  });
}

export function useUpdateStackEnvVars() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('stacks');
  return useMutation({
    mutationFn: ({ name, envVars }: { name: string; envVars: Record<string, string> }) =>
      api.put(`/stacks/${name}/env-vars`, envVars).then(assertSuccess),
    meta: { success: t('toast.envVarsSaved') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['stack-env-vars'] });
      queryClient.invalidateQueries({ queryKey: ['stacks'] });
    },
  });
}

export function usePruneStack() {
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);
  const { t } = useTranslation('stacks');
  return useMutation({
    mutationFn: (name: string) =>
      api
        .post<PruneResult>(`/stacks/${name}/prune${envId ? `?env=${envId}` : ''}`)
        .then((r) => {
          if (!r.data) {
            throw new Error('stack prune response missing data');
          }
          return r.data;
        }),
    meta: { success: (data: PruneResult) => t('toast.pruned', { count: data.count }) },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['stacks'] });
      queryClient.invalidateQueries({ queryKey: ['stack-containers'] });
    },
  });
}

export function useStackWebhooks(name: string | null) {
  return useQuery({
    queryKey: ['stack-webhooks', name],
    queryFn: () =>
      api
        .get<StackWebhook[]>(`/stacks/${name}/webhooks`)
        .then((r) => r.data ?? []),
    enabled: !!name,
  });
}

export function useCreateStackWebhook() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('stacks');
  return useMutation({
    mutationFn: ({ name, input }: { name: string; input: CreateStackWebhookInput }) =>
      api.post(`/stacks/${name}/webhooks`, input).then(assertSuccess),
    meta: { success: t('toast.webhookCreated') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['stack-webhooks'] });
    },
  });
}

export function useUpdateStackWebhook() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('stacks');
  return useMutation({
    mutationFn: ({
      stackName,
      webhookId,
      input,
    }: {
      stackName: string;
      webhookId: string;
      input: UpdateStackWebhookInput;
    }) => api.put(`/stacks/${stackName}/webhooks/${webhookId}`, input).then(assertSuccess),
    meta: { success: t('toast.webhookUpdated') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['stack-webhooks'] });
    },
  });
}

export function useDeleteStackWebhook() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('stacks');
  return useMutation({
    mutationFn: ({ stackName, webhookId }: { stackName: string; webhookId: string }) =>
      api.del(`/stacks/${stackName}/webhooks/${webhookId}`).then(assertSuccess),
    meta: { success: t('toast.webhookDeleted') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['stack-webhooks'] });
    },
  });
}

export function useTestStackWebhook() {
  const { t } = useTranslation('stacks');
  return useMutation({
    mutationFn: ({ stackName, webhookId }: { stackName: string; webhookId: string }) =>
      api
        .post<{ success: boolean; statusCode?: number; error?: string }>(
          `/stacks/${stackName}/webhooks/${webhookId}/test`,
        )
        .then((r) => {
          if (!r.data) {
            throw new Error('stack webhook test response missing data');
          }
          return r.data;
        }),
    meta: {
      success: t('webhooks.testSuccess'),
      error: t('webhooks.testFailed'),
    },
  });
}

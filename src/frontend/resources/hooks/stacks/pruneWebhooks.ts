// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';

import { assertSuccess } from '@resources/utils/api-mutation';
import { useEnvironmentStore } from '@resources/stores/environment';

import type {
  StackWebhook,
  CreateStackWebhookInput,
  UpdateStackWebhookInput,
  PruneResult,
} from '@resources/types/stack-webhook';

export function usePruneStack() {
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);
  const { t } = useTranslation('stacks');
  return useMutation({
    mutationFn: (itemName: string) =>
      api.post<PruneResult>(`/stacks/${itemName}/prune${envId ? `?env=${envId}` : ''}`).then((r) => {
        if (!r.data) {
          throw new Error('stack prune response missing data');
        }
        return r.data;
      }),
    meta: {
      success: (data: PruneResult) => t('toast.pruned', { count: data.count }),
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['stacks'] });
      void queryClient.invalidateQueries({ queryKey: ['stack-containers'] });
    },
  });
}

export function useStackWebhooks(itemName: string | null) {
  return useQuery({
    queryKey: ['stack-webhooks', itemName],
    queryFn: () => api.get<StackWebhook[]>(`/stacks/${itemName}/webhooks`).then((r) => r.data ?? []),
    enabled: !!itemName,
  });
}

export function useCreateStackWebhook() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('stacks');
  return useMutation({
    mutationFn: ({ name: itemName, input }: { name: string; input: CreateStackWebhookInput }) =>
      api.post(`/stacks/${itemName}/webhooks`, input).then(assertSuccess),
    meta: { success: t('toast.webhookCreated') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['stack-webhooks'] });
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
      void queryClient.invalidateQueries({ queryKey: ['stack-webhooks'] });
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
      void queryClient.invalidateQueries({ queryKey: ['stack-webhooks'] });
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

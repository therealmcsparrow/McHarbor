// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';

import { useStacksForEnvironment as useScopedStacksForEnvironment } from '@resources/hooks/useStacksForEnvironment';
import { assertSuccess } from '@resources/utils/api-mutation';
import { REVEAL_QUERY_OPTIONS } from '@resources/utils/env-redaction';
import { useEnvironmentStore } from '@resources/stores/environment';
import { runWithMcHarborRestart } from '@resources/stores/mcharbor-restart';

import type { StackInfo } from './types';

export function useStacks() {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['stacks', envId],
    queryFn: () => api.get<StackInfo[]>('/stacks', envId ? { env: envId } : {}).then((r) => r.data ?? []),
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
    mutationFn: ({ stack, action }: { stack: StackInfo; action: string }) => {
      const request = () =>
        api.post(`/stacks/${stack.name}/${action}${envId ? `?env=${envId}` : ''}`).then(assertSuccess);
      return action === 'restart' && stack.targetsSelf ? runWithMcHarborRestart(request) : request();
    },
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
      void queryClient.invalidateQueries({ queryKey: ['stacks'] });
    },
  });
}

export function useDeleteStack() {
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);
  const { t } = useTranslation('stacks');
  return useMutation({
    mutationFn: (itemName: string) => api.del(`/stacks/${itemName}`, envId ? { env: envId } : {}).then(assertSuccess),
    meta: { success: t('toast.removed') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['stacks'] });
    },
  });
}

export function useStackLogs(itemName: string | null, tail = 500) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['stack-logs', envId, itemName, tail],
    queryFn: () =>
      api
        .get<Record<string, string>>(`/stacks/${itemName}/logs`, {
          tail: String(tail),
          ...(envId ? { env: envId } : {}),
        })
        .then((r) => r.data ?? {}),
    enabled: !!itemName,
  });
}

export function useStackCompose(itemName: string | null) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['stack-compose', envId, itemName],
    queryFn: () =>
      api
        .get<{ content: string }>(`/stacks/${itemName}/compose`, envId ? { env: envId } : {})
        .then((r) => r.data?.content ?? ''),
    enabled: !!itemName,
  });
}

/**
 * The compose text above has credential-looking environment values masked. This is the
 * opt-in half: `?reveal=true` needs stacks.manage on the stack and is audit-logged, so it
 * runs only while the operator asks, and the plaintext never outlives its last subscriber.
 */
export function useStackComposeRevealed(itemName: string | null, enabled: boolean) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['stack-compose-revealed', envId, itemName],
    queryFn: () =>
      api
        .get<{ content: string }>(`/stacks/${itemName}/compose`, {
          ...(envId ? { env: envId } : {}),
          reveal: 'true',
        })
        .then(assertSuccess)
        .then((data) => data.content ?? ''),
    enabled: !!itemName && enabled,
    ...REVEAL_QUERY_OPTIONS,
  });
}

export function useUpdateStack() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('stacks');
  return useMutation({
    mutationFn: ({
      name: itemName,
      compose,
      description,
      secretFileMode,
    }: {
      name: string;
      compose?: string;
      description?: string;
      // Undefined leaves the stack's current setting alone — the backend
      // treats a nil pointer as "no change", not as "off".
      secretFileMode?: boolean;
    }) => api.put(`/stacks/${itemName}`, { compose, description, secretFileMode }).then(assertSuccess),
    meta: { success: t('toast.updated') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['stacks'] });
    },
  });
}

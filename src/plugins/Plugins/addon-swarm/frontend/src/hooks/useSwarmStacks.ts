// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import type { ApiResponse } from '@core/api/client';
import { useEnvironmentStore } from '../host-env';
import { assertSuccess } from '@resources/utils/api-mutation';
import type {
  SwarmDeployResult,
  SwarmOverlayNetwork,
  SwarmStack,
  SwarmStackDetail,
  SwarmStackServiceError,
} from '../types';

function envParams(envId: string | null | undefined): Record<string, string> {
  return envId ? { env: envId } : {};
}

function envSuffix(envId: string | null | undefined): string {
  return envId ? `?env=${envId}` : '';
}

export function useSwarmStacks(enabled = true) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['swarm', 'stacks', envId],
    queryFn: () => api.get<SwarmStack[]>('/swarm/stacks', envParams(envId)).then((r) => assertSuccess(r) ?? []),
    enabled,
    refetchInterval: 15_000,
  });
}

export function useSwarmStack(itemName: string | undefined) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['swarm', 'stack', itemName, envId],
    queryFn: () => api.get<SwarmStackDetail>(`/swarm/stacks/${itemName}`, envParams(envId)).then(assertSuccess),
    enabled: Boolean(itemName),
    refetchInterval: 15_000,
  });
}

export function useSwarmNetworks(enabled = true) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['swarm', 'networks', envId],
    queryFn: () =>
      api.get<SwarmOverlayNetwork[]>('/swarm/networks', envParams(envId)).then((r) => assertSuccess(r) ?? []),
    enabled,
    refetchInterval: 60_000,
  });
}

/**
 * A deploy that Swarm partially refused.
 *
 * The rejection is a 400 whose body still carries the per-service reasons, so
 * this error type keeps them attached rather than collapsing the whole thing
 * into one unactionable string. The dialog renders `rejected` as a list.
 */
export class SwarmDeployRejectedError extends Error {
  readonly rejected: SwarmStackServiceError[];

  constructor(message: string, rejected: SwarmStackServiceError[]) {
    super(message);
    this.name = 'SwarmDeployRejectedError';
    this.rejected = rejected;
  }
}

export function useDeploySwarmStack() {
  const { t } = useTranslation('swarm');
  const envId = useEnvironmentStore((s) => s.currentId);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (body: { name: string; compose: string; prune: boolean }) => {
      const res: ApiResponse<SwarmDeployResult> = await api.post<SwarmDeployResult>(
        `/swarm/stacks${envSuffix(envId)}`,
        body,
      );
      if (!res.success) {
        const rejected = res.data?.rejected ?? [];
        throw new SwarmDeployRejectedError(res.error ?? t('errors.deployFailed'), rejected);
      }
      return res.data as SwarmDeployResult;
    },
    meta: { success: t('toast.stackDeployed') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['swarm'] });
    },
  });
}

export function useRemoveSwarmStack() {
  const { t } = useTranslation('swarm');
  const envId = useEnvironmentStore((s) => s.currentId);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (itemName: string) => api.del(`/swarm/stacks/${itemName}`, envParams(envId)).then(assertSuccess),
    meta: { success: t('toast.stackRemoved') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['swarm'] });
    },
  });
}

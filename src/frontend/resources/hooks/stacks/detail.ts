// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import type { ContainerInfo } from '@core/types/docker';

import { assertSuccess } from '@resources/utils/api-mutation';
import { REVEAL_QUERY_OPTIONS } from '@resources/utils/env-redaction';
import { useEnvironmentStore } from '@resources/stores/environment';

import type { StackDetail } from './types';

export function useStack(itemName: string) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['stack', envId, itemName],
    queryFn: () =>
      api.get<StackDetail>(`/stacks/${itemName}`, envId ? { env: envId } : {}).then((r) => {
        if (!r.data) {
          throw new Error('stack response missing data');
        }
        return r.data;
      }),
    refetchInterval: 10_000,
    enabled: !!itemName,
  });
}

export function useStackContainers(itemName: string) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['stack-containers', envId, itemName],
    queryFn: () =>
      api.get<ContainerInfo[]>(`/stacks/${itemName}/containers`, envId ? { env: envId } : {}).then((r) => r.data ?? []),
    refetchInterval: 10_000,
    enabled: !!itemName,
  });
}

export function useStackEnvVars(itemName: string | null) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['stack-env-vars', envId, itemName],
    queryFn: () =>
      api
        .get<Record<string, string>>(`/stacks/${itemName}/env-vars`, envId ? { env: envId } : {})
        .then((r) => r.data ?? {}),
    enabled: !!itemName,
  });
}

/**
 * The masked payload above hides anything the backend's key heuristic reads as
 * a credential. This is the second half: the same endpoint with ?reveal=true,
 * which returns the real values and writes an audit entry each time it is
 * called. It is deliberately a separate hook so a caller has to opt in.
 *
 * The revealed values are never cached — staleTime 0 and gcTime 0 mean the
 * plaintext leaves the Query cache as soon as the last subscriber unmounts,
 * rather than surviving navigation and landing in any state dump.
 */
export function useStackEnvVarsRevealed(itemName: string | null, enabled: boolean) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['stack-env-vars-revealed', envId, itemName],
    queryFn: () =>
      api
        .get<Record<string, string>>(`/stacks/${itemName}/env-vars`, {
          ...(envId ? { env: envId } : {}),
          reveal: 'true',
        })
        .then((r) => r.data ?? {}),
    enabled: !!itemName && enabled,
    ...REVEAL_QUERY_OPTIONS,
  });
}

export function useUpdateStackEnvVars() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('stacks');
  return useMutation({
    mutationFn: ({ name: itemName, envVars }: { name: string; envVars: Record<string, string> }) =>
      api.put(`/stacks/${itemName}/env-vars`, envVars).then(assertSuccess),
    meta: { success: t('toast.envVarsSaved') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['stack-env-vars'] });
      void queryClient.invalidateQueries({ queryKey: ['stacks'] });
    },
  });
}

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { useEnvironmentStore } from '../host-env';
import { assertSuccess } from '@resources/utils/api-mutation';
import type {
  SwarmCluster,
  SwarmClusterUpdateRequest,
  SwarmInitRequest,
  SwarmJoinRequest,
  SwarmJoinTokens,
  SwarmOverview,
  SwarmUnlockKey,
} from '../types';

/**
 * `?env=` for a query-params object, and the same thing as a URL suffix for
 * the mutation helpers, which take a path rather than params.
 */
function envParams(envId: string | null | undefined): Record<string, string> {
  return envId ? { env: envId } : {};
}

function envSuffix(envId: string | null | undefined): string {
  return envId ? `?env=${envId}` : '';
}

export function useSwarmCluster(enabled = true) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['swarm', 'cluster', envId],
    queryFn: () => api.get<SwarmCluster>('/swarm', envParams(envId)).then(assertSuccess),
    enabled,
    refetchInterval: 60_000,
  });
}

export function useSwarmOverview(enabled = true) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['swarm', 'overview', envId],
    queryFn: () => api.get<SwarmOverview>('/swarm/overview', envParams(envId)).then(assertSuccess),
    enabled,
    refetchInterval: 15_000,
  });
}

/**
 * The join tokens.
 *
 * Disabled by default and never prefetched: a manager token is equivalent to
 * control of the cluster, and reading one is audited server-side. The dialog
 * that shows them enables this only while it is open.
 */
export function useSwarmJoinTokens(enabled: boolean) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['swarm', 'join-tokens', envId],
    queryFn: () => api.get<SwarmJoinTokens>('/swarm/join-tokens', envParams(envId)).then(assertSuccess),
    enabled,
    staleTime: 0,
    gcTime: 0,
  });
}

/** The autolock key. Same handling discipline as the join tokens. */
export function useSwarmUnlockKey(enabled: boolean) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['swarm', 'unlock-key', envId],
    queryFn: () => api.get<SwarmUnlockKey>('/swarm/unlock-key', envParams(envId)).then(assertSuccess),
    enabled,
    staleTime: 0,
    gcTime: 0,
  });
}

/**
 * Everything about the Swarm changes when the cluster's membership does, so
 * every lifecycle mutation drops the whole `swarm` query tree rather than
 * trying to name the parts that moved.
 */
function useSwarmInvalidation() {
  const queryClient = useQueryClient();
  return () => queryClient.invalidateQueries({ queryKey: ['swarm'] });
}

export function useSwarmInit() {
  const { t } = useTranslation('swarm');
  const envId = useEnvironmentStore((s) => s.currentId);
  const invalidate = useSwarmInvalidation();

  return useMutation({
    mutationFn: (body: SwarmInitRequest) =>
      api.post<{ nodeId: string }>(`/swarm/init${envSuffix(envId)}`, body).then(assertSuccess),
    meta: { success: t('toast.initialised') },
    onSuccess: invalidate,
  });
}

export function useSwarmJoin() {
  const { t } = useTranslation('swarm');
  const envId = useEnvironmentStore((s) => s.currentId);
  const invalidate = useSwarmInvalidation();

  return useMutation({
    mutationFn: (body: SwarmJoinRequest) => api.post(`/swarm/join${envSuffix(envId)}`, body).then(assertSuccess),
    meta: { success: t('toast.joined') },
    onSuccess: invalidate,
  });
}

export function useSwarmLeave() {
  const { t } = useTranslation('swarm');
  const envId = useEnvironmentStore((s) => s.currentId);
  const invalidate = useSwarmInvalidation();

  return useMutation({
    mutationFn: (force: boolean) => api.post(`/swarm/leave${envSuffix(envId)}`, { force }).then(assertSuccess),
    meta: { success: t('toast.left') },
    onSuccess: invalidate,
  });
}

export function useSwarmUpdate() {
  const { t } = useTranslation('swarm');
  const envId = useEnvironmentStore((s) => s.currentId);
  const invalidate = useSwarmInvalidation();

  return useMutation({
    mutationFn: (body: SwarmClusterUpdateRequest) => api.put(`/swarm${envSuffix(envId)}`, body).then(assertSuccess),
    meta: { success: t('toast.clusterUpdated') },
    onSuccess: invalidate,
  });
}

export function useSwarmUnlock() {
  const { t } = useTranslation('swarm');
  const envId = useEnvironmentStore((s) => s.currentId);
  const invalidate = useSwarmInvalidation();

  return useMutation({
    mutationFn: (unlockKey: string) => api.post(`/swarm/unlock${envSuffix(envId)}`, { unlockKey }).then(assertSuccess),
    meta: { success: t('toast.unlocked') },
    onSuccess: invalidate,
  });
}

export function useSwarmRotateJoinToken() {
  const { t } = useTranslation('swarm');
  const envId = useEnvironmentStore((s) => s.currentId);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (kind: 'worker' | 'manager') =>
      api.post(`/swarm/join-tokens/${kind}/rotate${envSuffix(envId)}`, {}).then(assertSuccess),
    meta: { success: t('toast.tokenRotated') },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['swarm', 'join-tokens'] }),
  });
}

export function useSwarmRotateUnlockKey() {
  const { t } = useTranslation('swarm');
  const envId = useEnvironmentStore((s) => s.currentId);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => api.post<SwarmUnlockKey>(`/swarm/unlock-key/rotate${envSuffix(envId)}`, {}).then(assertSuccess),
    meta: { success: t('toast.unlockKeyRotated') },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['swarm', 'unlock-key'] }),
  });
}

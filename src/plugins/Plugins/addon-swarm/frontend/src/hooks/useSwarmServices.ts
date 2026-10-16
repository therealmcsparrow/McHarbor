// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { useEnvironmentStore } from '../host-env';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { SwarmService, SwarmServiceDetail, SwarmServiceWriteRequest } from '../types';

function envParams(envId: string | null | undefined): Record<string, string> {
  return envId ? { env: envId } : {};
}

function envSuffix(envId: string | null | undefined): string {
  return envId ? `?env=${envId}` : '';
}

export function useSwarmServices(enabled = true) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['swarm', 'services', envId],
    queryFn: () => api.get<SwarmService[]>('/swarm/services', envParams(envId)).then((r) => assertSuccess(r) ?? []),
    enabled,
    // Rolling updates move task counts every few seconds; a page that only
    // refreshed on navigation would show a converged service mid-rollout.
    refetchInterval: 10_000,
  });
}

export function useSwarmService(id: string | undefined) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['swarm', 'service', id, envId],
    queryFn: () => api.get<SwarmServiceDetail>(`/swarm/services/${id}`, envParams(envId)).then(assertSuccess),
    enabled: Boolean(id),
    refetchInterval: 10_000,
  });
}

/**
 * The detail above masks credential-looking env values. This reads them: the route needs
 * swarm_services.manage and the host audits every call, so it runs only while the operator
 * asks, and the values never outlive their last subscriber.
 */
export function useSwarmServiceEnv(id: string | undefined, enabled: boolean) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['swarm', 'service-env', id, envId],
    queryFn: () =>
      api
        .get<{ env: string[] }>(`/swarm/services/${id}/env`, envParams(envId))
        .then(assertSuccess)
        .then((data) => data.env ?? []),
    enabled: Boolean(id) && enabled,
    // Each call returns plaintext and writes an audit row, so it never re-runs on its own.
    staleTime: 0,
    gcTime: 0,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  });
}

export function useSwarmServiceLogs(id: string | undefined, tail = 200, enabled = true) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['swarm', 'service-logs', id, tail, envId],
    queryFn: () =>
      api
        .get<{ logs: string }>(`/swarm/services/${id}/logs`, {
          ...envParams(envId),
          tail: String(tail),
        })
        .then((r) => assertSuccess(r)?.logs ?? ''),
    enabled: Boolean(id) && enabled,
    refetchInterval: 15_000,
  });
}

/**
 * Invalidate the service list and, when known, the one service that changed.
 * Task counts live on both, so a mutation that refreshed only the detail page
 * would leave a stale replica count on the list behind it.
 */
function useServiceInvalidation() {
  const queryClient = useQueryClient();
  return (id?: string) => {
    void queryClient.invalidateQueries({ queryKey: ['swarm', 'services'] });
    void queryClient.invalidateQueries({ queryKey: ['swarm', 'tasks'] });
    void queryClient.invalidateQueries({ queryKey: ['swarm', 'overview'] });
    if (id) void queryClient.invalidateQueries({ queryKey: ['swarm', 'service', id] });
  };
}

export function useScaleSwarmService() {
  const { t } = useTranslation('swarm');
  const envId = useEnvironmentStore((s) => s.currentId);
  const invalidate = useServiceInvalidation();

  return useMutation({
    mutationFn: ({ id, replicas }: { id: string; replicas: number }) =>
      api.post(`/swarm/services/${id}/scale${envSuffix(envId)}`, { replicas }).then(assertSuccess),
    meta: { success: t('toast.scaled') },
    onSuccess: (_data, variables) => invalidate(variables.id),
  });
}

export function useRestartSwarmService() {
  const { t } = useTranslation('swarm');
  const envId = useEnvironmentStore((s) => s.currentId);
  const invalidate = useServiceInvalidation();

  return useMutation({
    mutationFn: (id: string) => api.post(`/swarm/services/${id}/restart${envSuffix(envId)}`, {}).then(assertSuccess),
    meta: { success: t('toast.restarted') },
    onSuccess: (_data, id) => invalidate(id),
  });
}

export function useRollbackSwarmService() {
  const { t } = useTranslation('swarm');
  const envId = useEnvironmentStore((s) => s.currentId);
  const invalidate = useServiceInvalidation();

  return useMutation({
    mutationFn: (id: string) => api.post(`/swarm/services/${id}/rollback${envSuffix(envId)}`, {}).then(assertSuccess),
    meta: { success: t('toast.rolledBack') },
    onSuccess: (_data, id) => invalidate(id),
  });
}

export function useRemoveSwarmService() {
  const { t } = useTranslation('swarm');
  const envId = useEnvironmentStore((s) => s.currentId);
  const invalidate = useServiceInvalidation();

  return useMutation({
    mutationFn: (id: string) => api.del(`/swarm/services/${id}`, envParams(envId)).then(assertSuccess),
    meta: { success: t('toast.serviceRemoved') },
    onSuccess: () => invalidate(),
  });
}

export function useCreateSwarmService() {
  const { t } = useTranslation('swarm');
  const envId = useEnvironmentStore((s) => s.currentId);
  const invalidate = useServiceInvalidation();

  return useMutation({
    mutationFn: (body: SwarmServiceWriteRequest) =>
      api.post<{ id: string }>(`/swarm/services${envSuffix(envId)}`, body).then(assertSuccess),
    meta: { success: t('toast.serviceCreated') },
    onSuccess: () => invalidate(),
  });
}

export function useUpdateSwarmService() {
  const { t } = useTranslation('swarm');
  const envId = useEnvironmentStore((s) => s.currentId);
  const invalidate = useServiceInvalidation();

  return useMutation({
    mutationFn: ({ id, body }: { id: string; body: SwarmServiceWriteRequest }) =>
      api.put(`/swarm/services/${id}${envSuffix(envId)}`, body).then(assertSuccess),
    meta: { success: t('toast.serviceUpdated') },
    onSuccess: (_data, variables) => invalidate(variables.id),
  });
}

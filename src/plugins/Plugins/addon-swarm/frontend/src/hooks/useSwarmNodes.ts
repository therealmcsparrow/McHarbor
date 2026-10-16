// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { useEnvironmentStore } from '../host-env';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { SwarmNode } from '../types';

function envParams(envId: string | null | undefined): Record<string, string> {
  return envId ? { env: envId } : {};
}

function envSuffix(envId: string | null | undefined): string {
  return envId ? `?env=${envId}` : '';
}

export function useSwarmNodes(enabled = true) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['swarm', 'nodes', envId],
    queryFn: () => api.get<SwarmNode[]>('/swarm/nodes', envParams(envId)).then((r) => assertSuccess(r) ?? []),
    enabled,
    refetchInterval: 15_000,
  });
}

export function useSwarmNode(id: string | undefined) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['swarm', 'node', id, envId],
    queryFn: () => api.get<SwarmNode>(`/swarm/nodes/${id}`, envParams(envId)).then(assertSuccess),
    enabled: Boolean(id),
    refetchInterval: 15_000,
  });
}

/**
 * A node change moves the control plane: capability, tasks and the overview
 * are all downstream of it, so they all go.
 */
function useNodeInvalidation() {
  const queryClient = useQueryClient();
  return (id?: string) => {
    void queryClient.invalidateQueries({ queryKey: ['swarm', 'nodes'] });
    void queryClient.invalidateQueries({ queryKey: ['swarm', 'capability'] });
    void queryClient.invalidateQueries({ queryKey: ['swarm', 'overview'] });
    void queryClient.invalidateQueries({ queryKey: ['swarm', 'tasks'] });
    if (id) void queryClient.invalidateQueries({ queryKey: ['swarm', 'node', id] });
  };
}

export function useSetNodeAvailability() {
  const { t } = useTranslation('swarm');
  const envId = useEnvironmentStore((s) => s.currentId);
  const invalidate = useNodeInvalidation();

  return useMutation({
    mutationFn: ({ id, availability }: { id: string; availability: 'active' | 'pause' | 'drain' }) =>
      api.post(`/swarm/nodes/${id}/availability${envSuffix(envId)}`, { availability }).then(assertSuccess),
    meta: { success: t('toast.nodeUpdated') },
    onSuccess: (_data, variables) => invalidate(variables.id),
  });
}

export function useSetNodeRole() {
  const { t } = useTranslation('swarm');
  const envId = useEnvironmentStore((s) => s.currentId);
  const invalidate = useNodeInvalidation();

  return useMutation({
    mutationFn: ({ id, role }: { id: string; role: 'manager' | 'worker' }) =>
      api.post(`/swarm/nodes/${id}/role${envSuffix(envId)}`, { role }).then(assertSuccess),
    meta: { success: t('toast.nodeUpdated') },
    onSuccess: (_data, variables) => invalidate(variables.id),
  });
}

export function useSetNodeLabels() {
  const { t } = useTranslation('swarm');
  const envId = useEnvironmentStore((s) => s.currentId);
  const invalidate = useNodeInvalidation();

  return useMutation({
    mutationFn: ({ id, labels }: { id: string; labels: Record<string, string> }) =>
      api.put(`/swarm/nodes/${id}${envSuffix(envId)}`, { labels }).then(assertSuccess),
    meta: { success: t('toast.nodeLabelsUpdated') },
    onSuccess: (_data, variables) => invalidate(variables.id),
  });
}

export function useRemoveSwarmNode() {
  const { t } = useTranslation('swarm');
  const envId = useEnvironmentStore((s) => s.currentId);
  const invalidate = useNodeInvalidation();

  return useMutation({
    mutationFn: ({ id, force }: { id: string; force?: boolean }) => {
      const params = envParams(envId);
      if (force) params.force = 'true';
      return api.del(`/swarm/nodes/${id}`, params).then(assertSuccess);
    },
    meta: { success: t('toast.nodeRemoved') },
    onSuccess: () => invalidate(),
  });
}

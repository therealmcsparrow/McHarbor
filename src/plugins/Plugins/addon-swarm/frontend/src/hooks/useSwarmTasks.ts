// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { useEnvironmentStore } from '../host-env';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { SwarmTask } from '../types';

export type SwarmTaskFilter = {
  service?: string;
  node?: string;
  state?: string;
};

/**
 * Tasks, optionally narrowed server-side.
 *
 * The filters go to the daemon rather than being applied here: a cluster's
 * task history can run to thousands of rows, and fetching all of them to show
 * one service's four is the difference between a page that opens and one that
 * hangs.
 */
export function useSwarmTasks(filter: SwarmTaskFilter = {}, enabled = true) {
  const envId = useEnvironmentStore((s) => s.currentId);

  const params: Record<string, string> = {};
  if (envId) params.env = envId;
  if (filter.service) params.service = filter.service;
  if (filter.node) params.node = filter.node;
  if (filter.state) params.state = filter.state;

  return useQuery({
    queryKey: ['swarm', 'tasks', envId, filter.service ?? '', filter.node ?? '', filter.state ?? ''],
    queryFn: () => api.get<SwarmTask[]>('/swarm/tasks', params).then((r) => assertSuccess(r) ?? []),
    enabled,
    refetchInterval: 10_000,
  });
}

export function useSwarmTask(id: string | undefined) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['swarm', 'task', id, envId],
    queryFn: () => api.get<SwarmTask>(`/swarm/tasks/${id}`, envId ? { env: envId } : {}).then(assertSuccess),
    enabled: Boolean(id),
  });
}

export function useSwarmTaskLogs(id: string | undefined, tail = 200, enabled = true) {
  const envId = useEnvironmentStore((s) => s.currentId);
  const params: Record<string, string> = { tail: String(tail) };
  if (envId) params.env = envId;

  return useQuery({
    queryKey: ['swarm', 'task-logs', id, tail, envId],
    queryFn: () =>
      api.get<{ logs: string }>(`/swarm/tasks/${id}/logs`, params).then((r) => assertSuccess(r)?.logs ?? ''),
    enabled: Boolean(id) && enabled,
    refetchInterval: 15_000,
  });
}

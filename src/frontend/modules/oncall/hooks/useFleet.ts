// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQueries } from '@tanstack/react-query';
import type { ContainerInfo } from '@core/types/docker';
import type { EnvironmentListItem } from '@resources/hooks/useEnvironmentList';
import { fetchOnCall, oldestCachedAt, useOnCallQuery } from '@resources/hooks/useOnCallQuery';

export type StateCounts = {
  running: number;
  stopped: number;
  restarting: number;
  other: number;
};

export type FleetEntry = {
  environment: EnvironmentListItem;
  containers: ContainerInfo[];
  counts: StateCounts;
  isLoading: boolean;
  error: string | null;
};

export function countStates(containers: ContainerInfo[]): StateCounts {
  const counts: StateCounts = { running: 0, stopped: 0, restarting: 0, other: 0 };
  for (const container of containers) {
    const state = String(container.State);
    if (state === 'running') counts.running++;
    else if (state === 'exited' || state === 'dead' || state === 'stopped') counts.stopped++;
    else if (state === 'restarting') counts.restarting++;
    else counts.other++;
  }
  return counts;
}

export function useFleet() {
  const environments = useOnCallQuery<EnvironmentListItem[]>(['environments'], '/environments', undefined, {
    refetchInterval: 30_000,
  });
  const list = environments.data?.data ?? [];
  const containers = useQueries({
    queries: list.map((environment) => ({
      queryKey: ['oncall', 'containers', environment.id],
      queryFn: () => fetchOnCall<ContainerInfo[]>('/containers', { all: 'true', env: environment.id }),
      enabled: environment.isActive,
      refetchInterval: 30_000,
      networkMode: 'always' as const,
    })),
  });
  const entries: FleetEntry[] = list.map((environment, index) => {
    const query = containers[index];
    const rows = query?.data?.data ?? [];
    return {
      environment,
      containers: rows,
      counts: countStates(rows),
      isLoading: Boolean(query?.isLoading),
      error: query?.error instanceof Error ? query.error.message : null,
    };
  });
  return {
    entries,
    isLoading: environments.isLoading,
    error: environments.error,
    refetch: environments.refetch,
    cachedAt: oldestCachedAt(environments.data?.cachedAt, ...containers.map((query) => query.data?.cachedAt)),
  };
}

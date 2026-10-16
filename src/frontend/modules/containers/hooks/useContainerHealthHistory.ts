// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
import type { ContainerHealthHistory } from '@core/types/docker';
import { assertSuccess } from '@resources/utils/api-mutation';

/** Number of blocks the Health and Uptime strips render. */
export const HEALTH_HISTORY_LIMIT = 30;

/**
 * Reads the persisted healthcheck probe series and run-state samples for one
 * container. The backend also folds the live Docker state into the store on
 * every read, so the poll interval doubles as the sampling cadence for
 * whichever container is currently on screen.
 */
export function useContainerHealthHistory(containerId: string, enabled = true, limit = HEALTH_HISTORY_LIMIT) {
  const envId = useEnvironmentStore((s) => s.currentId);

  return useQuery({
    queryKey: ['container-health-history', envId, containerId, limit],
    queryFn: () =>
      api
        .get<ContainerHealthHistory>(`/containers/${containerId}/health/history`, {
          limit: String(limit),
          ...(envId ? { env: envId } : {}),
        })
        .then(assertSuccess),
    enabled: enabled && !!containerId,
    refetchInterval: 30_000,
  });
}

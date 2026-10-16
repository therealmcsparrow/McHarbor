// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { AgentHealthHistory } from '@core/types/docker';

/** Number of blocks the agent health and uptime strips render. */
export const AGENT_HEALTH_HISTORY_LIMIT = 30;

/**
 * Reads the sampled health and uptime series for one agent environment.
 *
 * The backend folds the live observation into the store on every read, so this
 * poll interval doubles as a sampling cadence for whichever environment is
 * currently on screen — the same arrangement the container health history uses.
 * Thirty seconds matches both the collector's tick and the default keepalive
 * interval, so a faster poll would only re-read the same bucket.
 */
export function useAgentHealthHistory(envId: string, enabled = true, limit = AGENT_HEALTH_HISTORY_LIMIT) {
  return useQuery({
    queryKey: ['agents', envId, 'health-history', limit],
    queryFn: () =>
      api
        .get<AgentHealthHistory>(`/agents/${envId}/health/history`, {
          limit: String(limit),
        })
        .then(assertSuccess),
    enabled: enabled && envId !== '',
    refetchInterval: 30_000,
    placeholderData: (previous) => previous,
  });
}

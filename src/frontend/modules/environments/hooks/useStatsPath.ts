// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export type StatsPathMode = 'push' | 'pull' | 'direct' | 'off';

export type StatsPathReason =
  | 'agent_too_old'
  | 'awaiting_first_batch'
  | 'agent_disconnected'
  | 'agent_metrics_disabled';

export type StatsPath = {
  mode: StatsPathMode;
  reason?: StatsPathReason;
  intervalSeconds?: number;
  lastBatchAt?: string;
  agentVersion?: string;
  agentBuild?: string;
  requiredVersion?: string;
  requiredBuild?: string;
};

export function useStatsPath(envId: string, enabled = true) {
  return useQuery({
    queryKey: ['metrics', 'stats-path', envId],
    queryFn: () => api.get<StatsPath>('/metrics/stats-path', { env: envId }).then(assertSuccess),
    enabled: enabled && envId !== '',
    refetchInterval: 15_000,
    placeholderData: (previous) => previous,
  });
}

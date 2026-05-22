// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
import { useCurrentEnvironmentActivitySettings } from '@resources/hooks/useCurrentEnvironmentActivitySettings';

export type DashboardStats = {
  containers: { total: number; running: number; stopped: number; paused: number };
  images: number;
  volumes: number;
  networks: number;
  cpuHistory?: Array<{ timestamp: string; value: number }>;
  memoryHistory?: Array<{ timestamp: string; value: number }>;
  networkRxHistory?: Array<{ timestamp: string; value: number }>;
  networkTxHistory?: Array<{ timestamp: string; value: number }>;
  blockReadHistory?: Array<{ timestamp: string; value: number }>;
  blockWriteHistory?: Array<{ timestamp: string; value: number }>;
};

export function useDashboardStats() {
  const envId = useEnvironmentStore((s) => s.currentId);
  const { collectContainerMetricsEnabled } = useCurrentEnvironmentActivitySettings();
  return useQuery({
    queryKey: ['dashboard-stats', envId],
    queryFn: () =>
      api
        .get<DashboardStats>('/dashboard/stats', envId ? { env: envId } : {})
        .then((r) => r.data),
    refetchInterval: collectContainerMetricsEnabled ? 15_000 : false,
    enabled: collectContainerMetricsEnabled,
  });
}

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import type { ContainerMetric } from '@core/types/docker';
import { useEnvironmentStore } from '@resources/stores/environment';
import { useEnvironmentUploadActive } from '@resources/stores/upload-activity';
import { useCurrentEnvironmentActivitySettings } from './useCurrentEnvironmentActivitySettings';

export type BulkContainerMetric = ContainerMetric;

export function useContainersBulkStats() {
  const envId = useEnvironmentStore((state) => state.currentId);
  const { collectContainerMetricsEnabled } = useCurrentEnvironmentActivitySettings();
  const uploadActive = useEnvironmentUploadActive(envId);

  return useQuery({
    queryKey: ['containers-bulk-stats', envId],
    queryFn: () =>
      api
        .get<ContainerMetric[]>('/containers/stats/summary', envId ? { env: envId } : {})
        .then((response) => {
          const map = new Map<string, BulkContainerMetric>();
          for (const metric of response.data ?? []) {
            map.set(metric.id, metric);
          }
          return map;
        }),
    refetchInterval: collectContainerMetricsEnabled && !uploadActive ? 5_000 : false,
    enabled: collectContainerMetricsEnabled && !uploadActive,
  });
}

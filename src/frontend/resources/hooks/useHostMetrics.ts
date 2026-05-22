// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import type { HostMetrics } from '@core/types/docker';
import { useEnvironmentStore } from '@resources/stores/environment';

export function useHostMetrics() {
  const envId = useEnvironmentStore((state) => state.currentId);

  return useQuery({
    queryKey: ['host-metrics', envId],
    queryFn: () =>
      api
        .get<HostMetrics>('/metrics/host', envId ? { env: envId } : {})
        .then((response) => response.data),
    refetchInterval: 30_000,
  });
}

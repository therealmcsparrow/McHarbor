// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';

export function useContainerLogs(containerId: string, tail = 500, live = false) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['container-logs', envId, containerId, tail],
    queryFn: () =>
      api
        .get<{ logs: string }>(`/containers/${containerId}/logs`, {
          tail: String(tail),
          timestamps: 'true',
          ...(envId ? { env: envId } : {}),
        })
        .then((r) => r.data?.logs ?? ''),
    enabled: !!containerId,
    refetchInterval: live ? 2_000 : false,
  });
}

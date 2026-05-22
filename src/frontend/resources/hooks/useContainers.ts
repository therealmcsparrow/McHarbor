// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import type { ContainerInfo } from '@core/types/docker';
import { useEnvironmentStore } from '@resources/stores/environment';

export function useContainers(all = true) {
  const envId = useEnvironmentStore((state) => state.currentId);

  return useQuery({
    queryKey: ['containers', envId, all],
    queryFn: () =>
      api
        .get<ContainerInfo[]>('/containers', {
          all: String(all),
          ...(envId ? { env: envId } : {}),
        })
        .then((response) => response.data ?? []),
    refetchInterval: 10_000,
  });
}

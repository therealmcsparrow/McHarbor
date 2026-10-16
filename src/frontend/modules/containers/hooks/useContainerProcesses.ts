// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
import type { ContainerProcesses } from '@core/types/docker';
import { assertSuccess } from '@resources/utils/api-mutation';

export function useContainerProcesses(containerId: string, enabled = true, poll = true) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['container-processes', envId, containerId],
    queryFn: () =>
      api.get<ContainerProcesses>(`/containers/${containerId}/top`, envId ? { env: envId } : {}).then(assertSuccess),
    enabled: enabled && !!containerId,
    refetchInterval: poll ? 5_000 : false,
  });
}

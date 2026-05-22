// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';

export type ContainerService = {
  name: string;
  status: string;
  sub?: string;
};

export type ContainerServicesResult = {
  initSystem: string;
  services: ContainerService[];
};

export function useContainerServices(containerId: string, enabled = true) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['container-services', envId, containerId],
    queryFn: () =>
      api
        .get<ContainerServicesResult>(`/containers/${containerId}/services`, {
          ...(envId ? { env: envId } : {}),
        })
        .then((r) => r.data ?? { initSystem: '', services: [] }),
    enabled: enabled && !!containerId,
    staleTime: 30_000,
  });
}

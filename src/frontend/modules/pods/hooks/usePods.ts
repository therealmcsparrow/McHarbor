// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
import type { PodSummary, PodDetail } from '@core/types/kubernetes';

export function usePods(namespace = '') {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['pods', envId, namespace],
    queryFn: () =>
      api
        .get<PodSummary[]>('/pods', {
          ...(envId ? { env: envId } : {}),
          ...(namespace ? { namespace } : {}),
        })
        .then((r) => r.data ?? []),
    refetchInterval: 10_000,
  });
}

export function usePod(namespace: string, name: string) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['pod', envId, namespace, name],
    queryFn: () =>
      api
        .get<PodDetail>(`/pods/${namespace}/${name}`, envId ? { env: envId } : {})
        .then((r) => r.data),
    enabled: !!namespace && !!name,
  });
}

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
import type { NamespaceSummary } from '@core/types/kubernetes';

export function useNamespaces() {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['namespaces', envId],
    queryFn: () =>
      api
        .get<NamespaceSummary[]>('/namespaces', envId ? { env: envId } : {})
        .then((r) => r.data ?? []),
    refetchInterval: 30_000,
  });
}

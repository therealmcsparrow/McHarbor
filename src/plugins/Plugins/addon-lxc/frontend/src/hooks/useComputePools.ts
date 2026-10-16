// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export interface ComputePool {
  id: string;
  environmentId: string;
  name: string;
  backend: string;
  target: string;
  isDefault: boolean;
}

export function useComputePools(environmentId?: string) {
  return useQuery({
    queryKey: ['compute', 'pools', environmentId],
    enabled: Boolean(environmentId),
    queryFn: async () =>
      assertSuccess(await api.get<ComputePool[]>('/compute/pools', { env: environmentId ?? '' })) ?? [],
  });
}

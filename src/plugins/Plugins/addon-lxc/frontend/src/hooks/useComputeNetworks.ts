// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export interface ComputeNetwork {
  id: string;
  environmentId: string;
  name: string;
  mode?: string;
  bridge: string;
  vlanTag?: number;
  uplinkLink?: string;
  linkMode?: string;
  /** Absent on a compute build that predates IPv6, which is treated as "may be on". */
  ipv6Enabled?: boolean;
  isDefault: boolean;
}

export function useComputeNetworks(environmentId?: string) {
  return useQuery({
    queryKey: ['compute', 'networks', environmentId],
    enabled: Boolean(environmentId),
    queryFn: async () =>
      assertSuccess(
        await api.get<ComputeNetwork[]>('/compute/networks', {
          env: environmentId!,
        }),
      ) ?? [],
  });
}

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export type HostEnvironment = { id: string; name: string };

/**
 * The host's environments, for linking a cluster to the one it runs on. Read from the
 * host API rather than its store: a plugin bundle's copy of the store is a second,
 * unsynchronised instance (see host-env notes in addon-swarm).
 */
export function useHostEnvironments() {
  return useQuery({
    queryKey: ['kubernetes', 'host-environments'],
    queryFn: () => api.get<HostEnvironment[]>('/environments').then(assertSuccess),
    staleTime: 60_000,
  });
}

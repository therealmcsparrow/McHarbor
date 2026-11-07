// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

/** Shares the `['versions']` cache with the environments page; only the McHarbor entry is read here. */
export function useHostVersion(enabled = true): string | undefined {
  const query = useQuery({
    queryKey: ['versions'],
    queryFn: async () => assertSuccess(await api.get<{ mcharbor: { name: string; version: string } }>('/versions')),
    enabled,
    staleTime: 60_000,
  });
  return query.data?.mcharbor.version;
}

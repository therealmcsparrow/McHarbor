// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export type OnCallResult<T> = {
  data: T;
  cachedAt: string | null;
};

export async function fetchOnCall<T>(path: string, params?: Record<string, string>): Promise<OnCallResult<T>> {
  const { body, cachedAt } = await api.getWithMeta<T>(path, params);
  return { data: assertSuccess(body), cachedAt };
}

type OnCallQueryOptions = {
  enabled?: boolean;
  refetchInterval?: number;
};

export function useOnCallQuery<T>(
  key: readonly unknown[],
  path: string,
  params?: Record<string, string>,
  options: OnCallQueryOptions = {},
) {
  return useQuery({
    queryKey: ['oncall', ...key],
    queryFn: () => fetchOnCall<T>(path, params),
    enabled: options.enabled ?? true,
    refetchInterval: options.refetchInterval,
    networkMode: 'always',
  });
}

export function oldestCachedAt(...values: Array<string | null | undefined>): string | null {
  const present = values.filter((value): value is string => Boolean(value)).sort();
  return present[0] ?? null;
}

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery, useMutation, useQueryClient, type UseQueryOptions } from '@tanstack/react-query';
import { api, type ApiResponse } from '@core/api/client';
import { envParams } from '@core/api/env-url';
import { useEnvironmentStore } from '@resources/stores/environment';
import { assertSuccess, type MutationMeta } from '@resources/utils/api-mutation';

/**
 * Hook for GET requests with environment-scoped cache keys.
 */
export function useApiQuery<T>(
  key: string[],
  path: string,
  params?: Record<string, string>,
  options?: Omit<UseQueryOptions<T>, 'queryKey' | 'queryFn'>
) {
  const envId = useEnvironmentStore((s) => s.currentId);

  return useQuery<T>({
    queryKey: [...key, envId],
    queryFn: async () => {
      const res = await api.get<T>(path, envParams(params));
      if (!res.success) {
        throw new Error(res.error ?? 'Request failed');
      }
      return res.data as T;
    },
    ...options,
  });
}

/**
 * Hook for POST/PUT/DELETE mutations with auto-invalidation.
 */
export function useApiMutation<TData = unknown, TVariables = unknown>(
  method: 'post' | 'put' | 'patch' | 'del',
  path: string | ((vars: TVariables) => string),
  invalidateKeys?: string[][],
  meta?: MutationMeta
) {
  const queryClient = useQueryClient();

  return useMutation<TData, Error, TVariables>({
    mutationFn: async (variables: TVariables) => {
      const url = typeof path === 'function' ? path(variables) : path;
      const fn = api[method].bind(api) as (path: string, body?: unknown) => Promise<ApiResponse<TData>>;
      return fn(url, variables).then(assertSuccess);
    },
    meta,
    onSuccess: () => {
      if (invalidateKeys) {
        invalidateKeys.forEach((key) => {
          queryClient.invalidateQueries({ queryKey: key });
        });
      }
    },
  });
}

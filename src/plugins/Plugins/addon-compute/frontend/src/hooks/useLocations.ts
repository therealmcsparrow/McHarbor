// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type ApiResponse } from '@core/api/client';
import { apiErrorOf, describeApiError, ApiError, type ErrorDetails } from '@resources/utils/api-mutation';
import type { EnvironmentLocationsView, Locations } from '../types';

class LocationsRequestError extends ApiError {
  constructor(
    readonly code?: string,
    details?: Omit<ErrorDetails, 'at'>,
  ) {
    const summary = details?.summary ?? describeApiError({ code });
    super(summary, details?.status, { summary, ...details, code });
    this.name = 'LocationsRequestError';
  }
}

function unwrap<T>(res: ApiResponse<T>): T {
  if (!res.success) throw new LocationsRequestError(res.code ?? res.error, apiErrorOf(res).details);
  return res.data as T;
}

const KEYS = {
  environment: (envId: string) => ['compute', 'locations', envId] as const,
};

export function useEnvironmentLocations(envId: string | undefined) {
  return useQuery({
    queryKey: KEYS.environment(envId ?? ''),
    queryFn: async () => unwrap(await api.get<EnvironmentLocationsView>(`/compute/locations/${envId}`)),
    enabled: Boolean(envId),
    refetchOnWindowFocus: false,
  });
}

export function useSaveLocations() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: { envId: string; body: Locations }) =>
      unwrap(await api.put<EnvironmentLocationsView>(`/compute/locations/${input.envId}`, input.body)),
    onSuccess: (_data, input) => {
      void queryClient.invalidateQueries({ queryKey: KEYS.environment(input.envId) });
      void queryClient.invalidateQueries({ queryKey: ['compute', 'overview'] });
    },
  });
}

export function useRemoveLocations() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (envId: string) => unwrap(await api.del<EnvironmentLocationsView>(`/compute/locations/${envId}`)),
    onSuccess: (_data, envId) => {
      void queryClient.invalidateQueries({ queryKey: KEYS.environment(envId) });
      void queryClient.invalidateQueries({ queryKey: ['compute', 'overview'] });
    },
  });
}

export function useApplyLocations() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (envId: string) =>
      unwrap(await api.post<EnvironmentLocationsView>(`/compute/locations/${envId}/apply`, {})),
    onSuccess: (_data, envId) => {
      void queryClient.invalidateQueries({ queryKey: KEYS.environment(envId) });
    },
  });
}

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { ApiError, apiErrorOf, type ErrorDetails } from '@resources/utils/api-mutation';
import type { Capability, ComputeEnvironment, InstallResult, Overview, Pool, PoolBackendInfo } from '../types';
import { KEYS, params, unwrap } from './computeBase';

export { ComputeRequestError } from './computeBase';
export * from './useComputeImages';
export * from './useComputeNetworks';

/**
 * Server state for the compute foundation.
 *
 * Every read is a TanStack query and every write a mutation that invalidates
 * what it changed — no manual fetch, no useState mirror of server data.
 *
 * The polling intervals are chosen rather than uniform. A capability report
 * describes packages on someone's host and changes when they change something,
 * so it is not polled at all; a download's progress changes every second and is
 * polled only while one is actually in flight.
 */

/** The compute overview: one card per host. */
export function useOverview() {
  return useQuery({
    queryKey: KEYS.overview,
    queryFn: async () => unwrap(await api.get<Overview>('/compute/overview')),
    // Pool capacity and agent reachability both drift on their own, so the
    // landing page refreshes without the operator having to ask.
    refetchInterval: 30_000,
  });
}

/** Every environment's compute settings. */
export function useEnvironments() {
  return useQuery({
    queryKey: KEYS.environments,
    queryFn: async () => unwrap(await api.get<ComputeEnvironment[]>('/compute/environments')) ?? [],
  });
}

/**
 * One host's stored capability report.
 *
 * This reads what was last probed rather than probing: opening a page should not
 * run external binaries on someone's machine. The panel offers an explicit
 * refresh, which is `useProbe`.
 */
export function useCapability(envId: string | undefined) {
  return useQuery({
    queryKey: KEYS.capability(envId ?? ''),
    enabled: Boolean(envId),
    queryFn: async () => unwrap(await api.get<ComputeEnvironment>(`/compute/environments/${envId}/capability`)),
  });
}

/** Re-check a host. */
export function useProbe() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (envId: string) => unwrap(await api.post<Capability>(`/compute/environments/${envId}/probe`, {})),
    onSuccess: (_data, envId) => {
      void queryClient.invalidateQueries({ queryKey: KEYS.capability(envId) });
      void queryClient.invalidateQueries({ queryKey: KEYS.overview });
    },
  });
}

/**
 * A failed install, with the host's own words and the package manager's output beside the
 * code, because "the install failed" alone sends the operator to the host to find out why.
 */
export class CapabilityInstallError extends ApiError {
  constructor(
    readonly code: string | undefined,
    readonly detail: string | undefined,
    readonly output: string | undefined,
    details?: Omit<ErrorDetails, 'at'>,
  ) {
    super(detail || code || 'install failed', details?.status, {
      summary: detail || code || 'install failed',
      ...details,
      code,
    });
    this.name = 'CapabilityInstallError';
  }
}

/** Have the host's agent install the packages for one missing capability item. */
export function useInstallCapability() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ envId, item }: { envId: string; item: string }) => {
      const res = await api.post<InstallResult>(`/compute/environments/${envId}/capability/install`, { item });
      if (!res.success) {
        const detail = res.error && res.error !== res.code ? res.error : undefined;
        throw new CapabilityInstallError(res.code, detail, res.data?.output, apiErrorOf(res).details);
      }
      return res.data as InstallResult;
    },
    // Settled, not success: a failed install still stores the host's fresh capability.
    onSettled: (_data, _error, { envId }) => {
      void queryClient.invalidateQueries({ queryKey: KEYS.capability(envId) });
      void queryClient.invalidateQueries({ queryKey: KEYS.overview });
    },
  });
}

/** Turn compute on or off for a host. */
export function useUpdateEnvironment() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      envId: string;
      computeEnabled?: boolean;
      defaultPoolId?: string;
      defaultNetworkId?: string;
      sharedLibrary?: boolean;
    }) => {
      const { envId, ...body } = input;
      return unwrap(await api.put<ComputeEnvironment>(`/compute/environments/${envId}`, body));
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: KEYS.environments });
      void queryClient.invalidateQueries({ queryKey: KEYS.overview });
    },
  });
}

/** Storage pools, optionally for one host. */
export function usePools(envId?: string) {
  return useQuery({
    queryKey: KEYS.pools(envId),
    queryFn: async () => unwrap(await api.get<Pool[]>('/compute/pools', params({ env: envId }))) ?? [],
    refetchInterval: 60_000,
  });
}

/** Which storage backends a host can offer, and where a pool could live. */
export function usePoolBackends(envId: string | undefined) {
  return useQuery({
    queryKey: KEYS.poolBackends(envId ?? ''),
    enabled: Boolean(envId),
    queryFn: async () =>
      unwrap(await api.get<PoolBackendInfo[]>('/compute/pools/detect', params({ env: envId }))) ?? [],
    // Detection runs `zfs list` and friends on the host. Once per dialog open
    // is right; polling would be running commands on someone's machine on a
    // timer for no reason.
    staleTime: 5 * 60_000,
  });
}

export function useCreatePool() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (body: {
      environmentId: string;
      name: string;
      backend: string;
      target: string;
      isDefault?: boolean;
    }) => unwrap(await api.post<Pool>('/compute/pools', body)),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['compute', 'pools'] });
      void queryClient.invalidateQueries({ queryKey: KEYS.overview });
    },
  });
}

/** Re-sample a pool's capacity. */
export function useRefreshPool() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (poolId: string) => unwrap(await api.put<Pool>(`/compute/pools/${poolId}`, {})),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['compute', 'pools'] });
    },
  });
}

export function useDeletePool() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: { poolId: string; force?: boolean }) =>
      unwrap(await api.del<null>(`/compute/pools/${input.poolId}`, input.force ? { force: 'true' } : undefined)),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['compute', 'pools'] });
      void queryClient.invalidateQueries({ queryKey: KEYS.overview });
    },
  });
}

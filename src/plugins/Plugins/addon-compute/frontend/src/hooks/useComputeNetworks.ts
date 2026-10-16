// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import type { HostLink, Lease, Network } from '../types';
import { KEYS, params, unwrap } from './computeBase';

/** Guest networks, optionally for one host. */
export function useNetworks(envId?: string) {
  return useQuery({
    queryKey: KEYS.networks(envId),
    queryFn: async () => unwrap(await api.get<Network[]>('/compute/networks', params({ env: envId }))) ?? [],
  });
}

/** A host's network interfaces, for the attach picker. */
export function useHostLinks(envId: string | undefined) {
  return useQuery({
    queryKey: KEYS.links(envId ?? ''),
    enabled: Boolean(envId),
    queryFn: async () => unwrap(await api.get<HostLink[]>('/compute/networks/detect', params({ env: envId }))) ?? [],
    staleTime: 60_000,
  });
}

export function useCreateNetwork() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (body: Record<string, unknown>) => unwrap(await api.post<Network>('/compute/networks', body)),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['compute', 'networks'] });
      void queryClient.invalidateQueries({ queryKey: KEYS.overview });
    },
  });
}

export function useUpdateNetwork() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ id, body }: { id: string; body: Record<string, unknown> }) =>
      unwrap(await api.put<Network>(`/compute/networks/${id}`, body)),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['compute', 'networks'] });
      void queryClient.invalidateQueries({ queryKey: KEYS.overview });
    },
  });
}

export function useDeleteNetwork() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (networkId: string) => unwrap(await api.del<null>(`/compute/networks/${networkId}`)),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['compute', 'networks'] });
      void queryClient.invalidateQueries({ queryKey: KEYS.overview });
    },
  });
}

/** A managed network's DHCP leases. */
export function useLeases(networkId: string | undefined) {
  return useQuery({
    queryKey: KEYS.leases(networkId ?? ''),
    enabled: Boolean(networkId),
    queryFn: async () => unwrap(await api.get<Lease[]>(`/compute/networks/${networkId}/leases`)) ?? [],
    refetchInterval: 30_000,
  });
}

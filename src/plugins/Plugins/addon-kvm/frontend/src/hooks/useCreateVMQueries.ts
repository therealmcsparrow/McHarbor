// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { ComputeImage, ComputeNetwork, ComputePool, PoolVolume } from '../components/create-vm/model';

export function useCreateVMQueries(environmentId: string, poolId: string) {
  // These three come from the Compute Foundation plugin's API.
  const { data: pools } = useQuery({
    queryKey: ['compute', 'pools', environmentId],
    enabled: Boolean(environmentId),
    queryFn: async () => assertSuccess(await api.get<ComputePool[]>('/compute/pools', { env: environmentId })) ?? [],
  });
  const { data: networks } = useQuery({
    queryKey: ['compute', 'networks', environmentId],
    enabled: Boolean(environmentId),
    queryFn: async () =>
      assertSuccess(
        await api.get<ComputeNetwork[]>('/compute/networks', {
          env: environmentId,
        }),
      ) ?? [],
  });
  const { data: images } = useQuery({
    queryKey: ['compute', 'images', environmentId, 'cloud_image'],
    enabled: Boolean(environmentId),
    queryFn: async () =>
      assertSuccess(
        await api.get<ComputeImage[]>('/compute/images', {
          env: environmentId,
          kind: 'cloud_image',
        }),
      ) ?? [],
  });

  const { data: discs } = useQuery({
    queryKey: ['compute', 'images', environmentId, 'iso'],
    enabled: Boolean(environmentId),
    queryFn: async () =>
      assertSuccess(
        await api.get<ComputeImage[]>('/compute/images', {
          env: environmentId,
          kind: 'iso',
        }),
      ) ?? [],
  });

  // Provisions a directory pool at the data location the operator named, and
  // returns the host's existing pool unchanged when it already has one.
  const ensurePool = useMutation({
    mutationFn: async (target: string) =>
      assertSuccess(
        await api.post<ComputePool>('/compute/pools/default', {
          environmentId,
          target,
        }),
      ),
  });

  const pool = pools?.find((entry) => entry.id === poolId);

  const {
    data: freeVolumes,
    isPending: volumesPending,
    isError: volumesFailed,
  } = useQuery({
    queryKey: ['vm', 'pool-volumes', environmentId, pool?.id],
    enabled: Boolean(environmentId && pool),
    queryFn: async () => {
      if (!pool) return [];
      return (
        assertSuccess(
          await api.get<PoolVolume[]>('/vms/pool-volumes', {
            env: environmentId,
            poolId: pool.id,
            poolBackend: pool.backend,
            poolTarget: pool.target,
          }),
        ) ?? []
      );
    },
  });

  return {
    pools,
    networks,
    images,
    discs,
    ensurePool,
    pool,
    freeVolumes,
    volumesPending,
    volumesFailed,
  };
}

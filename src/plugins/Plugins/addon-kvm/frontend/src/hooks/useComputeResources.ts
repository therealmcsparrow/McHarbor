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
  capacityBytes: number;
  usedBytes: number;
  instant: boolean;
  isDefault: boolean;
}

export interface ComputeNetwork {
  id: string;
  environmentId: string;
  name: string;
  mode?: string;
  bridge: string;
  vlanTag?: number;
  uplinkLink?: string;
  linkMode?: string;
  isDefault: boolean;
}

export interface ComputeImage {
  id: string;
  kind: string;
  displayName: string;
  arch: string;
  initSystem?: string;
  cache?: { state: string; localPath: string };
}

export function useComputePools(environmentId?: string) {
  return useQuery({
    queryKey: ['compute', 'pools', environmentId],
    enabled: Boolean(environmentId),
    queryFn: async () => assertSuccess(await api.get<ComputePool[]>('/compute/pools', { env: environmentId! })) ?? [],
  });
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

export function useComputeImages(environmentId: string | undefined, kind: string) {
  return useQuery({
    queryKey: ['compute', 'images', environmentId, kind],
    enabled: Boolean(environmentId),
    queryFn: async () =>
      assertSuccess(
        await api.get<ComputeImage[]>('/compute/images', {
          env: environmentId!,
          kind,
        }),
      ) ?? [],
    select: (images: ComputeImage[]) => images.filter((image) => image.cache?.state === 'ready'),
  });
}

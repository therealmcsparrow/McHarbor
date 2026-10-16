// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useQueries } from '@tanstack/react-query';
import { api } from '@core/api/client';
import type { ContainerInfo, ImageInfo, VolumeInfo, NetworkInfo } from '@core/types/docker';
import type { StackInfo } from './globalSearchModel';
import { usePluginSearchSources } from './useGlobalSearchPluginData';

type SearchEnv = { id: string; name: string };
export type WithEnv<T> = T & { envId: string; envName: string };

type ResourceSpec = {
  kind: string;
  path: string;
  all?: boolean;
  refetchMs: number;
  staleMs: number;
};

function useResource<T>(envs: SearchEnv[], isOpen: boolean, spec: ResourceSpec) {
  const queries = useQueries({
    queries: envs.map((env) => ({
      queryKey: [`global-search-${spec.kind}`, env.id],
      queryFn: () =>
        api.get<T[]>(spec.path, spec.all ? { all: 'true', env: env.id } : { env: env.id }).then((r) =>
          (r.data ?? []).map((item) => ({
            ...item,
            envId: env.id,
            envName: env.name,
          })),
        ),
      refetchInterval: isOpen ? spec.refetchMs : false,
      staleTime: spec.staleMs,
      enabled: isOpen,
    })),
  });
  const items = useMemo(() => queries.flatMap((q) => q.data ?? []), [queries]);
  return { items: items as WithEnv<T>[], isLoading: queries.some((q) => q.isLoading) };
}

export function useGlobalSearchData(envs: SearchEnv[], isOpen: boolean) {
  const containers = useResource<ContainerInfo>(envs, isOpen, {
    kind: 'containers',
    path: '/containers',
    all: true,
    refetchMs: 15_000,
    staleMs: 5_000,
  });
  const images = useResource<ImageInfo>(envs, isOpen, {
    kind: 'images',
    path: '/images',
    refetchMs: 30_000,
    staleMs: 10_000,
  });
  const volumes = useResource<VolumeInfo>(envs, isOpen, {
    kind: 'volumes',
    path: '/volumes',
    refetchMs: 30_000,
    staleMs: 10_000,
  });
  const networks = useResource<NetworkInfo>(envs, isOpen, {
    kind: 'networks',
    path: '/networks',
    refetchMs: 30_000,
    staleMs: 10_000,
  });
  const stacks = useResource<StackInfo>(envs, isOpen, {
    kind: 'stacks',
    path: '/stacks',
    refetchMs: 15_000,
    staleMs: 5_000,
  });
  const plugins = usePluginSearchSources(envs, isOpen);

  return {
    containers: containers.items,
    images: images.items,
    volumes: volumes.items,
    networks: networks.items,
    stacks: stacks.items,
    pluginGroups: plugins.groups,
    failedPluginSources: plugins.failedSources,
    isLoading:
      containers.isLoading ||
      images.isLoading ||
      volumes.isLoading ||
      networks.isLoading ||
      stacks.isLoading ||
      plugins.isLoading,
  };
}

export type GlobalSearchData = ReturnType<typeof useGlobalSearchData>;

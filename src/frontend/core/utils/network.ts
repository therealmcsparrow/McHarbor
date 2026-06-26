// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ContainerInspect, NetworkInfo } from '@core/types/docker';

const builtInDockerNetworks = new Set(['bridge', 'host', 'none']);

export function isBuiltInNetwork(network: Pick<NetworkInfo, 'BuiltIn' | 'Name'>): boolean {
  if (network.BuiltIn) return true;
  return builtInDockerNetworks.has(network.Name.toLowerCase());
}

export function getNetworkContainerCount(network: Pick<NetworkInfo, 'Containers'>): number {
  const containers = network.Containers;
  if (typeof containers === 'number') return containers;
  if (containers && typeof containers === 'object') return Object.keys(containers).length;
  return 0;
}

export function isNetworkInUse(network: Pick<NetworkInfo, 'Containers'>): boolean {
  return getNetworkContainerCount(network) > 0;
}

export type OrphanedNetworkRef = {
  id: string;
  name: string;
};

export function findOrphanedNetworks(
  containerInspect: ContainerInspect,
  networks: NetworkInfo[],
): OrphanedNetworkRef[] {
  const attached = containerInspect.NetworkSettings?.Networks;
  if (!attached) return [];

  const orphans: OrphanedNetworkRef[] = [];
  const seen = new Set<string>();

  for (const netName of Object.keys(attached)) {
    if (builtInDockerNetworks.has(netName)) continue;
    if (seen.has(netName)) continue;
    seen.add(netName);

    const net = networks.find((n) => n.Name === netName);
    if (!net) continue;

    const count = getNetworkContainerCount(net);
    if (count === 0) {
      orphans.push({ id: net.Id, name: net.Name });
    }
  }

  return orphans;
}

export function findBulkOrphanedNetworks(
  containerIds: string[],
  containers: { Id: string; NetworkSettings?: { Networks?: Record<string, unknown> } }[],
  networks: NetworkInfo[],
): OrphanedNetworkRef[] {
  const selectedSet = new Set(containerIds);
  const attachedCounts = new Map<string, number>();

  for (const c of containers) {
    if (!selectedSet.has(c.Id)) continue;
    const nets = c.NetworkSettings?.Networks;
    if (!nets) continue;
    for (const netName of Object.keys(nets)) {
      if (builtInDockerNetworks.has(netName)) continue;
      attachedCounts.set(netName, (attachedCounts.get(netName) ?? 0) + 1);
    }
  }

  const orphans: OrphanedNetworkRef[] = [];
  for (const net of networks) {
    if (builtInDockerNetworks.has(net.Name)) continue;
    const attached = attachedCounts.get(net.Name) ?? 0;
    if (attached === 0) continue;
    const total = getNetworkContainerCount(net);
    if (attached < total) continue;
    orphans.push({ id: net.Id, name: net.Name });
  }
  return orphans;
}

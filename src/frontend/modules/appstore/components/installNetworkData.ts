// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { api } from '@core/api/client';
import type { ContainerInfo, NetworkInfo } from '@core/types/docker';
import { assertSuccess } from '@resources/utils/api-mutation';

export type InstalledNetworkOption = {
  name: string;
  driver: string;
};

export function splitCsv(value: string): string[] {
  return value
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean);
}

export async function fetchInstalledNetworks(selectedEnvId: string): Promise<InstalledNetworkOption[]> {
  const params: Record<string, string> = selectedEnvId ? { env: selectedEnvId } : {};

  try {
    const networks = assertSuccess(await api.get<NetworkInfo[]>('/networks', params));
    return networks.map((item) => ({ name: item.Name, driver: item.Driver }));
  } catch (error) {
    const containers = assertSuccess(await api.get<ContainerInfo[]>('/containers', { ...params, all: 'true' }));
    const names = new Set<string>();
    containers.forEach((container) => {
      Object.keys(container.NetworkSettings?.Networks ?? {}).forEach((networkName) => {
        names.add(networkName);
      });
    });
    if (names.size === 0) throw error;
    return [...names].map((itemName) => ({ name: itemName, driver: '' }));
  }
}

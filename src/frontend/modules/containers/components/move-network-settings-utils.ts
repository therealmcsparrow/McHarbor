// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { MoveNetworkConfig } from '../hooks/useContainers';

export function csv(value?: string[]) {
  return value?.join(', ') ?? '';
}

export function splitCsv(value: string) {
  return value.split(',').map((item) => item.trim()).filter(Boolean);
}

export function updateIpam(network: MoveNetworkConfig, field: 'Subnet' | 'Gateway' | 'IPRange', value: string): MoveNetworkConfig {
  const first = network.ipam?.Config?.[0] ?? {};
  return {
    ...network,
    ipam: {
      ...network.ipam,
      Config: [{ ...first, [field]: value }],
    },
  };
}

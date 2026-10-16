// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export interface CapabilityItem {
  key: string;
  present: boolean;
  detail?: string;
  remediation?: string;
}

export interface HostCapability {
  canRunVms: boolean;
  hostCpus?: number;
  hostMemoryBytes?: number;
  hostSockets?: number;
  hostPhysicalCores?: number;
  items?: CapabilityItem[];
}

interface ComputeEnvironmentEnvelope {
  environmentId: string;
  computeEnabled: boolean;
  capability?: HostCapability;
  probedAt?: string;
}

export function useComputeCapability(environmentId?: string) {
  return useQuery({
    queryKey: ['compute', 'capability', environmentId],
    enabled: Boolean(environmentId),
    staleTime: 60_000,
    queryFn: async () => {
      const environment = assertSuccess(
        await api.get<ComputeEnvironmentEnvelope>(`/compute/environments/${environmentId}/capability`),
      );
      return environment.capability;
    },
  });
}

export function capabilityItem(capability: HostCapability | undefined, key: string): CapabilityItem | undefined {
  return capability?.items?.find((item) => item.key === key);
}

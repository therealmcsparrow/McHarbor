// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { apiClientPath, usePluginCapability } from '@core/plugins/capabilities';
import { useAgentInventory } from '@resources/hooks/useAgentInventory';
import { assertSuccess } from '@resources/utils/api-mutation';

const REFRESH_MS = 30_000;

type DashboardSummary = {
  environments: number;
  containers?: number;
  stacks?: number;
  unreachable?: number;
};

type ScanSummary = {
  needsReview?: number;
};

type EstateItem = Record<string, unknown>;

export type EstateSummary = {
  environments: number;
  containers?: number;
  stacks?: number;
  unreachable?: number;
  vms?: number;
  vmEnvironments?: number;
  agentsBehind?: number;
  scansNeedReview?: number;
};

/**
 * Every part is optional and independently resolved: a caller without the key
 * for one of them sees that segment absent, never reported as zero.
 */
export function useEstateSummary(): { data: EstateSummary | undefined; isLoading: boolean } {
  // The VM count comes from whichever loaded plugin declares the `vms` estate source.
  const vmSource = usePluginCapability('estate-source', 'vms');
  const vmPath = vmSource?.path ? apiClientPath(vmSource.path) : '';
  const vmEnvironmentField = vmSource?.environmentField ?? '';

  const summary = useQuery({
    queryKey: ['dashboard', 'estate-summary'],
    queryFn: () => api.get<DashboardSummary>('/dashboard/summary').then(assertSuccess),
    refetchInterval: REFRESH_MS,
    retry: false,
  });

  const agents = useAgentInventory();

  const scans = useQuery({
    queryKey: ['dashboard', 'estate-scans'],
    queryFn: () => api.get<ScanSummary>('/scans/summary').then(assertSuccess),
    refetchInterval: REFRESH_MS,
    retry: false,
  });

  const vms = useQuery({
    queryKey: ['dashboard', 'estate-vms', vmPath],
    queryFn: () => api.get<EstateItem[]>(vmPath).then(assertSuccess),
    refetchInterval: REFRESH_MS,
    retry: false,
    enabled: vmPath !== '',
  });

  if (!summary.data) {
    return { data: undefined, isLoading: summary.isLoading };
  }

  const guests = vms.data ?? [];
  const guestEnvironments = new Set(guests.map((vm) => vm[vmEnvironmentField]).filter(Boolean));
  const outdated = agents.data?.agents.filter((a) => a.outdated).length;

  return {
    data: {
      environments: summary.data.environments,
      containers: summary.data.containers,
      stacks: summary.data.stacks,
      unreachable: summary.data.unreachable,
      vms: vms.data ? guests.length : undefined,
      vmEnvironments: vms.data ? guestEnvironments.size : undefined,
      agentsBehind: agents.data ? outdated : undefined,
      scansNeedReview: scans.data?.needsReview,
    },
    isLoading: false,
  };
}

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { OverviewEntry } from '../../types';

export type ReadinessState = 'pass' | 'warn' | 'fail';

export interface ReadinessRow {
  key: string;
  state: ReadinessState;
  detail?: string;
  detailKey?: string;
}

function unavailableKey(entry: OverviewEntry): string | undefined {
  if (entry.available) return undefined;
  if (entry.unavailableReason === 'not_agent_environment') return 'agent.required.title';
  if (entry.unavailableReason === 'agent_too_old') return 'agent.tooOld.title';
  return 'agent.offline.title';
}

export function readinessRows(entry: OverviewEntry): ReadinessRow[] {
  const capability = entry.capability;
  const pools = entry.pools ?? [];
  const networks = entry.networks ?? [];

  return [
    {
      key: 'agent',
      state: entry.available ? 'pass' : 'fail',
      detailKey: unavailableKey(entry),
    },
    {
      key: 'probed',
      state: capability ? 'pass' : 'warn',
    },
    {
      key: 'supported',
      state: capability ? (capability.supported ? 'pass' : 'fail') : 'warn',
      detail: capability?.unsupportedWhy,
    },
    {
      key: 'vms',
      state: capability ? (capability.canRunVms ? 'pass' : 'fail') : 'warn',
    },
    {
      key: 'containers',
      state: capability ? (capability.canRunContainers ? 'pass' : 'warn') : 'warn',
    },
    {
      key: 'pool',
      state: pools.length > 0 ? 'pass' : 'fail',
    },
    {
      key: 'network',
      state: networks.length > 0 ? 'pass' : 'fail',
    },
    {
      key: 'sharedLibrary',
      state: entry.sharedLibrary ? 'pass' : 'warn',
    },
  ];
}

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { DemoConfig, DemoLimits, DemoSizes } from '@resources/types/demo';

export const DEMO_DEFAULTS: DemoConfig = {
  environments: 16,
  containers: 1212,
  virtualMachines: 120,
  systemContainers: 80,
  kubernetesClusters: 3,
  kubernetesDeployments: 264,
  swarmClusters: 2,
  swarmNodes: 9,
  swarmServices: 60,
  historyDays: 90,
};

export const DEMO_LIMITS: DemoLimits = {
  environments: { min: 1, max: 48 },
  containers: { min: 0, max: 6000 },
  virtualMachines: { min: 0, max: 600 },
  systemContainers: { min: 0, max: 400 },
  kubernetesClusters: { min: 0, max: 9 },
  kubernetesDeployments: { min: 0, max: 1500 },
  swarmClusters: { min: 0, max: 5 },
  swarmNodes: { min: 0, max: 60 },
  swarmServices: { min: 0, max: 400 },
  historyDays: { min: 7, max: 365 },
};

export const DEMO_STATUS_SIZES = { defaults: DEMO_DEFAULTS, limits: DEMO_LIMITS };

export function demoSizes(initial: DemoConfig = DEMO_DEFAULTS): DemoSizes {
  return { initial, defaults: DEMO_DEFAULTS, limits: DEMO_LIMITS };
}

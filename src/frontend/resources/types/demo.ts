// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export type DemoCounts = {
  environments: number;
  containers: number;
  stacks: number;
  virtualMachines: number;
  systemContainers: number;
  clusters: number;
  pods: number;
  swarmClusters: number;
  swarmNodes: number;
  swarmServices: number;
  historyDays: number;
};

export type DemoConfig = {
  environments: number;
  containers: number;
  virtualMachines: number;
  systemContainers: number;
  kubernetesClusters: number;
  kubernetesDeployments: number;
  swarmClusters: number;
  swarmNodes: number;
  swarmServices: number;
  historyDays: number;
};

export type DemoConfigField = keyof DemoConfig;

export type DemoRange = { min: number; max: number };

export type DemoLimits = Record<DemoConfigField, DemoRange>;

export type DemoStatus = {
  /** A demo_mode file sits in McHarbor's root; without it Settings hides the section. */
  available: boolean;
  active: boolean;
  databaseExists: boolean;
  clustered: boolean;
  canEnter: boolean;
  enteredAt?: string;
  counts?: DemoCounts;
  config?: DemoConfig;
  defaults: DemoConfig;
  limits: DemoLimits;
};

export type DemoSizes = {
  initial: DemoConfig;
  defaults: DemoConfig;
  limits: DemoLimits;
};

export type DemoChangeRequest = { config: DemoConfig };

export type DemoOutcome = {
  restartScheduled: boolean;
  restartRequired: boolean;
};

export type DemoAction = 'enter' | 'exit' | 'reset';

export type DemoSizesMode = 'enter' | 'resize';

export type DemoRefusal = 'already_active' | 'not_active' | 'clustered' | 'busy';

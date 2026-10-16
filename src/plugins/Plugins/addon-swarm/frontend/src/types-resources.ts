// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { SwarmCapability, SwarmCluster, SwarmService } from './types';

export type SwarmTask = {
  id: string;
  name: string;
  serviceId: string;
  serviceName: string;
  nodeId: string;
  nodeName: string;
  slot: number;
  image: string;
  desiredState: string;
  state: string;
  message: string;
  error: string;
  containerId: string;
  exitCode: number;
  createdAt: string;
  updatedAt: string;
  statusAt: string;
};

// --- nodes -----------------------------------------------------------------

export type SwarmNode = {
  id: string;
  hostname: string;
  role: 'manager' | 'worker';
  availability: 'active' | 'pause' | 'drain';
  state: string;
  message: string;
  addr: string;
  isLeader: boolean;
  reachability: string;
  engineVersion: string;
  os: string;
  architecture: string;
  nanoCpus: number;
  memoryBytes: number;
  labels: Record<string, string>;
  engineLabels: Record<string, string>;
  taskCount: number;
  createdAt: string;
  updatedAt: string;
  version: number;
  self: boolean;
};

// --- secrets and configs ---------------------------------------------------

export type SwarmSecret = {
  id: string;
  name: string;
  labels: Record<string, string>;
  stackName: string;
  driver: string;
  inUseBy: string[];
  createdAt: string;
  updatedAt: string;
  version: number;
};

export type SwarmConfig = {
  id: string;
  name: string;
  labels: Record<string, string>;
  stackName: string;
  size: number;
  /** base64; present only on the single-config read. */
  data?: string;
  inUseBy: string[];
  createdAt: string;
  updatedAt: string;
  version: number;
};

export type SwarmPayloadWriteRequest = {
  name: string;
  /** base64 */
  data: string;
  labels?: Record<string, string>;
};

// --- stacks and networks ---------------------------------------------------

export type SwarmStack = {
  name: string;
  services: number;
  runningTasks: number;
  desiredTasks: number;
  networks: string[];
  secrets: string[];
  configs: string[];
  createdAt: string;
  updatedAt: string;
};

export type SwarmStackDetail = SwarmStack & {
  serviceList: SwarmService[] | null;
};

export type SwarmStackServiceError = {
  service: string;
  reason: string;
};

export type SwarmDeployResult = {
  name: string;
  created: string[] | null;
  updated: string[] | null;
  removed: string[] | null;
  networks: string[] | null;
  rejected: SwarmStackServiceError[] | null;
};

export type SwarmOverlayNetwork = {
  id: string;
  name: string;
  driver: string;
  scope: string;
  internal: boolean;
  attachable: boolean;
  ingress: boolean;
  subnets: string[] | null;
  stackName: string;
  labels: Record<string, string>;
  createdAt: string;
};

export type SwarmOverview = {
  capability: SwarmCapability;
  cluster: SwarmCluster | null;
  nodesTotal: number;
  nodesReady: number;
  nodesDown: number;
  nodesDraining: number;
  managers: number;
  workers: number;
  servicesTotal: number;
  servicesConverged: number;
  servicesDegraded: number;
  tasksRunning: number;
  tasksDesired: number;
  tasksFailed: number;
  stacks: number;
  secrets: number;
  configs: number;
  networks: number;
};

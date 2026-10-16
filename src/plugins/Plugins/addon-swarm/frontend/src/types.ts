// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/**
 * Mirrors of the DTOs in `src/backend/modules/swarm/model.go`.
 *
 * Field-for-field, including the JSON casing the Go tags produce. When a DTO
 * changes on the server, it changes here in the same commit — a silently
 * diverging mirror is worse than no mirror, because the compiler stops
 * catching the drift.
 */

export type SwarmCapability = {
  capable: boolean;
  nodeId: string;
  /** 'manager' | 'worker' | '' */
  nodeRole: string;
  controlAvailable: boolean;
  clusterId: string;
  nodes: number;
  managers: number;
};

export type SwarmCluster = {
  id: string;
  name: string;
  createdAt: string;
  updatedAt: string;
  version: number;
  labels: Record<string, string>;
  nodes: number;
  managers: number;
  autoLockManagers: boolean;
  taskHistoryLimit: number;
  heartbeatTickMillis: number;
  snapshotInterval: number;
  certExpiryHours: number;
  dataPathPort: number;
  defaultAddrPool: string[] | null;
  subnetSize: number;
  rootRotationInProgress: boolean;
};

export type SwarmJoinTokens = {
  worker: string;
  manager: string;
};

export type SwarmUnlockKey = {
  key: string;
};

export type SwarmInitRequest = {
  listenAddr?: string;
  advertiseAddr?: string;
  dataPathAddr?: string;
  forceNewCluster?: boolean;
  autoLockManagers?: boolean;
  defaultAddrPool?: string[];
  subnetSize?: number;
};

export type SwarmJoinRequest = {
  listenAddr?: string;
  advertiseAddr?: string;
  dataPathAddr?: string;
  remoteAddrs: string[];
  joinToken: string;
};

export type SwarmClusterUpdateRequest = {
  taskHistoryLimit?: number;
  autoLockManagers?: boolean;
  certExpiryHours?: number;
  snapshotInterval?: number;
};

// --- services --------------------------------------------------------------

export type SwarmServiceMode = 'replicated' | 'global' | 'replicated-job' | 'global-job' | 'unknown';

export type SwarmServicePort = {
  name: string;
  protocol: string;
  targetPort: number;
  publishedPort: number;
  publishMode: string;
};

export type SwarmService = {
  id: string;
  name: string;
  image: string;
  mode: SwarmServiceMode;
  replicas: number;
  runningTasks: number;
  desiredTasks: number;
  ports: SwarmServicePort[] | null;
  networks: string[] | null;
  stackName: string;
  labels: Record<string, string>;
  createdAt: string;
  updatedAt: string;
  version: number;
  updateState: string;
  updateMessage: string;
};

export type SwarmServiceMount = {
  type: string;
  source: string;
  target: string;
  readOnly: boolean;
};

export type SwarmServiceFileRef = {
  id: string;
  name: string;
  target: string;
};

export type SwarmServiceResources = {
  limitNanoCpus: number;
  limitMemoryBytes: number;
  reserveNanoCpus: number;
  reserveMemoryBytes: number;
};

export type SwarmServiceRestart = {
  condition: string;
  delaySeconds: number;
  maxAttempts: number;
};

export type SwarmServiceRollout = {
  parallelism: number;
  delaySeconds: number;
  failureAction: string;
  maxFailureRatio: number;
  order: string;
};

export type SwarmServiceDetail = SwarmService & {
  command: string[] | null;
  args: string[] | null;
  env: string[] | null;
  mounts: SwarmServiceMount[] | null;
  constraints: string[] | null;
  secrets: SwarmServiceFileRef[] | null;
  configs: SwarmServiceFileRef[] | null;
  resources: SwarmServiceResources;
  endpointMode: string;
  restartPolicy: SwarmServiceRestart;
  updateConfig: SwarmServiceRollout;
  hasPreviousSpec: boolean;
  containerLabels: Record<string, string>;
};

export type SwarmServiceWriteRequest = {
  name: string;
  image: string;
  mode: 'replicated' | 'global';
  replicas: number;
  command?: string[];
  args?: string[];
  env?: string[];
  labels?: Record<string, string>;
  constraints?: string[];
  networks?: string[];
  ports?: SwarmServicePort[];
  mounts?: SwarmServiceMount[];
  limitNanoCpus?: number;
  limitMemoryBytes?: number;
  reserveNanoCpus?: number;
  reserveMemoryBytes?: number;
};

// --- tasks -----------------------------------------------------------------

export * from './types-resources';

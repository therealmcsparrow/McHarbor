// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export { containerToEditForm } from './edit-form-parse';
export { buildRecreatePayload, buildUpdatePayload } from './edit-form-payload';

export type PortMappingEntry = {
  containerPort: string;
  hostPort: string;
  hostIp: string;
  protocol: string;
};

export type DeviceMappingEntry = {
  pathOnHost: string;
  pathInContainer: string;
  cgroupPermissions: string;
};

export type MountType = 'bind' | 'volume' | 'tmpfs';

export type MountEntry = {
  type: MountType;
  /** Host path for bind, volume name for volume; unused for tmpfs. */
  source: string;
  target: string;
  readOnly: boolean;
  propagation: string;
};

export type HealthcheckConfig = {
  enabled: boolean;
  command: string;
  interval: number;
  timeout: number;
  retries: number;
  startPeriod: number;
};

export type EditFormData = {
  // General / Image
  image: string;

  // Environment tab
  env: string[];
  labels: Record<string, string>;
  cmd: string[];
  entrypoint: string[];
  workingDir: string;
  hostname: string;
  domainname: string;
  user: string;
  tty: boolean;
  openStdin: boolean;
  stopSignal: string;

  // Healthcheck
  healthcheck: HealthcheckConfig;

  // Network tab
  portMappings: PortMappingEntry[];
  exposedPorts: string[];
  dns: string[];
  dnsSearch: string[];
  dnsOptions: string[];
  extraHosts: string[];
  networkMode: string;

  // Mounts tab (the secret-files mount is never part of this list)
  mounts: MountEntry[];

  // Resources tab
  restartPolicyName: string;
  restartPolicyMaxRetry: number;
  memory: number;
  memorySwap: number;
  memoryReservation: number;
  nanoCpus: number;
  cpuShares: number;
  cpuPeriod: number;
  cpuQuota: number;
  cpusetCpus: string;
  cpusetMems: string;
  blkioWeight: number;
  privileged: boolean;
  readonlyRootfs: boolean;
  capAdd: string[];
  capDrop: string[];
  securityOpt: string[];
  shmSize: number;
  pidMode: string;
  init: boolean;
  autoRemove: boolean;
  oomKillDisable: boolean;
  pidsLimit: number;
  devices: DeviceMappingEntry[];
  gpuEnabled: boolean;
  gpuDriver: string;
  gpuCount: number;
  gpuDeviceIds: string[];
  gpuCapabilities: string[];

  // Logging
  logDriver: string;
  logOptions: Record<string, string>;
};

export type ChangeClassification = {
  hasResourceChanges: boolean;
  hasConfigChanges: boolean;
  changedResourceFields: string[];
  changedConfigFields: string[];
};

export type ResourceEditField =
  | 'memory'
  | 'memorySwap'
  | 'memoryReservation'
  | 'nanoCpus'
  | 'cpuShares'
  | 'cpuPeriod'
  | 'cpuQuota'
  | 'cpusetCpus'
  | 'cpusetMems'
  | 'blkioWeight';

const RESOURCE_FIELDS = new Set([
  'memory',
  'memorySwap',
  'memoryReservation',
  'nanoCpus',
  'cpuShares',
  'cpuPeriod',
  'cpuQuota',
  'cpusetCpus',
  'cpusetMems',
  'blkioWeight',
  'restartPolicyName',
  'restartPolicyMaxRetry',
]);

export function classifyChanges(original: EditFormData, current: EditFormData): ChangeClassification {
  const changedResourceFields: string[] = [];
  const changedConfigFields: string[] = [];

  for (const key of Object.keys(original) as Array<keyof EditFormData>) {
    const orig = original[key];
    const curr = current[key];
    if (JSON.stringify(orig) === JSON.stringify(curr)) continue;

    if (RESOURCE_FIELDS.has(key)) {
      changedResourceFields.push(key);
    } else {
      changedConfigFields.push(key);
    }
  }

  return {
    hasResourceChanges: changedResourceFields.length > 0,
    hasConfigChanges: changedConfigFields.length > 0,
    changedResourceFields,
    changedConfigFields,
  };
}

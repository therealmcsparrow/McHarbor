// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export type MoveContainerPlan = {
  sourceEnvId: string;
  targetEnvId: string;
  containerId: string;
  containerName: string;
  targetName: string;
  image: {
    reference: string;
    id: string;
    size?: number;
    exists: boolean;
    willTransfer: boolean;
  };
  stack: {
    name?: string;
    service?: string;
    labelsPreserve: boolean;
    managedRecord: boolean;
  };
  volumes: Array<{
    type: string;
    name?: string;
    targetName?: string;
    source?: string;
    targetSource?: string;
    destination: string;
    targetDestination: string;
    mode?: string;
    exists: boolean;
    willCreate: boolean;
    willCopy: boolean;
    manual: boolean;
  }>;
  networkMode?: string;
  networks: Array<{
    name: string;
    sourceName: string;
    targetName: string;
    id?: string;
    driver?: string;
    exists: boolean;
    willCreate: boolean;
    aliases?: string[];
    targetAliases?: string[];
    ipAddress?: string;
    targetIpAddress?: string;
    macAddress?: string;
    targetMacAddress?: string;
    builtin: boolean;
    internal: boolean;
    attachable: boolean;
    ipam?: {
      Driver?: string;
      Config?: Array<{ Subnet?: string; Gateway?: string; IPRange?: string }>;
    };
    options?: Record<string, string>;
    labels?: Record<string, string>;
  }>;
  ports: Array<{
    containerPort: string;
    hostIp?: string;
    hostPort?: string;
  }>;
  requiredChanges: string[];
  warnings: string[];
};

export type MoveNetworkConfig = {
  sourceName: string;
  targetName: string;
  driver?: string;
  internal?: boolean;
  attachable?: boolean;
  ipam?: {
    Driver?: string;
    Config?: Array<{ Subnet?: string; Gateway?: string; IPRange?: string }>;
  };
  options?: Record<string, string>;
  labels?: Record<string, string>;
  aliases?: string[];
  ipAddress?: string;
  macAddress?: string;
};

export type MoveVolumeConfig = {
  sourceName: string;
  sourceDestination?: string;
  targetName?: string;
  targetSource?: string;
  targetDestination?: string;
};

export type MoveContainerOptions = {
  id: string;
  targetEnvId: string;
  targetName?: string;
  networkMode?: string;
  networks?: MoveNetworkConfig[];
  volumes?: MoveVolumeConfig[];
  transferImage: boolean;
  createMissingNetworks: boolean;
  createMissingVolumes: boolean;
  copyNamedVolumes: boolean;
  startTarget: boolean;
  stopSource: boolean;
  removeSource: boolean;
};

export type MoveContainerEvent = {
  step: number;
  total: number;
  message: string;
  status: 'progress' | 'done' | 'error';
  phase?: string;
  bytesTransferred?: number;
  bytesTotal?: number;
  targetContainerId?: string;
  targetName?: string;
};

export type MoveProgressLogEntry = {
  id: string;
  message: string;
  phase?: string;
};

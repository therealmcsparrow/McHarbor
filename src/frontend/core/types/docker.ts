// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export * from './dockerContainers';
export * from './dockerImages';
export type VolumeInfo = {
  Name: string;
  Driver: string;
  Mountpoint: string;
  CreatedAt: string;
  Status?: Record<string, unknown> | null;
  Labels: Record<string, string> | null;
  Scope: string;
  Options: Record<string, string> | null;
  UsageData?: {
    Size: number;
    RefCount: number;
  };
  RefCount: number;
  Protected?: boolean;
  UsedBy?: string[];
};

export type NetworkInfo = {
  Id: string;
  Name: string;
  Created: string;
  Scope: string;
  Driver: string;
  EnableIPv6: boolean;
  IPAM: {
    Driver: string;
    Config: Array<{
      Subnet?: string;
      Gateway?: string;
      IPRange?: string;
      AuxiliaryAddresses?: Record<string, string>;
    }>;
  };
  Internal: boolean;
  Attachable: boolean;
  Containers:
    | number
    | Record<
        string,
        {
          Name: string;
          EndpointID: string;
          MacAddress: string;
          IPv4Address: string;
          IPv6Address: string;
        }
      >;
  Options: Record<string, string> | null;
  Labels: Record<string, string> | null;
  BuiltIn?: boolean;
  ContainerNames?: string[];
};

export type DockerInfo = {
  ID: string;
  Containers: number;
  ContainersRunning: number;
  ContainersPaused: number;
  ContainersStopped: number;
  Images: number;
  Driver: string;
  MemTotal: number;
  NCPU: number;
  ServerVersion: string;
  OperatingSystem: string;
  Architecture: string;
  KernelVersion: string;
  Name: string;
};

export type DockerSystemInfo = {
  id: string;
  serverVersion: string;
  apiVersion: string;
  minApiVersion: string;
  gitCommit: string;
  goVersion: string;
  os: string;
  architecture: string;
  kernelVersion: string;
  hostname: string;
  ncpu: number;
  memTotal: number;
  storageDriver: string;
  dockerRootDir: string;
  driverStatus: string[][];
  cgroupDriver: string;
  cgroupVersion: string;
  defaultRuntime: string;
  runtimes: string[];
  containers: number;
  containersRunning: number;
  containersPaused: number;
  containersStopped: number;
  images: number;
  securityOptions: string[];
  pluginsVolume: string[];
  pluginsNetwork: string[];
  pluginsLog: string[];
  labels: string[];
  swarmActive: boolean;
  swarmNodeId: string;
  swarmManagers: number;
  swarmNodes: number;
  loggingDriver: string;
  isolation: string;
};

export type HostInfo = {
  ncpu: number;
  memTotal: number;
  memUsed: number;
  memPercent: number;
  cpuPercent: number;
  load1: number;
  load5: number;
  load15: number;
  serverVersion: string;
  os: string;
  architecture: string;
  kernelVersion: string;
  hostname: string;
  uptime: number;
  systemTime: string;
};

export type DiskUsage = {
  imagesSize: number;
  containersSize: number;
  volumesSize: number;
  buildCacheSize: number;
  total: number;
};

export type HostFSUsage = {
  path: string;
  total: number;
  used: number;
  percent: number;
};

export type HostFSState = 'ok' | 'stale' | 'unknown' | 'unsupported';

export type HostFSStatus = {
  environmentId: string;
  state: HostFSState;
  level: 'normal' | 'warning' | 'critical' | null;
  path: string;
  totalBytes: number;
  usedBytes: number;
  percent: number;
  lastSeen: string | null;
  reason?: 'agent_offline' | 'agent_too_old' | 'read_failed';
  thresholds: { warning: number; critical: number };
};

export type HostMetrics = {
  host: HostInfo;
  disk: DiskUsage;
  hostFs: HostFSUsage;
  agentLimit: boolean;
};

export type PruneType = 'system' | 'builder' | 'volumes' | 'images' | 'containers' | 'networks';

export type PruneRequest = {
  type: PruneType;
  volumes?: boolean;
  confirm: boolean;
};

export type PruneResult = {
  type: PruneType;
  itemsDeleted: number;
  spaceReclaimed: number;
};

export type AutohealPreference = {
  containerId: string;
  containerName: string;
  enabled: boolean;
  lastHealAt?: string;
  restartCount: number;
  wasEverHealthy: boolean;
};

export * from './dockerHealth';

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export type ContainerState =
  | 'created'
  | 'running'
  | 'paused'
  | 'restarting'
  | 'removing'
  | 'exited'
  | 'dead';

export type ContainerInfo = {
  Id: string;
  Names: string[];
  Image: string;
  ImageID: string;
  Command: string;
  Created: number;
  Ports: ContainerPort[];
  Labels: Record<string, string>;
  StackName?: string;
  StackService?: string;
  State: ContainerState;
  Status: string;
  Protected?: boolean;
  NetworkSettings: {
    Networks: Record<string, ContainerNetwork>;
  };
  Mounts: ContainerMount[];
};

export type ContainerPort = {
  IP?: string;
  PrivatePort: number;
  PublicPort?: number;
  Type: string;
};

export type ContainerNetwork = {
  IPAMConfig: unknown;
  Links: unknown;
  Aliases: string[] | null;
  NetworkID: string;
  EndpointID: string;
  Gateway: string;
  IPAddress: string;
  IPPrefixLen: number;
  IPv6Gateway: string;
  GlobalIPv6Address: string;
  MacAddress: string;
};

export type ContainerMount = {
  Type: string;
  Name?: string;
  Source: string;
  Destination: string;
  Driver?: string;
  Mode: string;
  RW: boolean;
  Propagation: string;
};

export type ContainerInspect = {
  Id: string;
  Image: string;
  Name: string;
  Created: string;
  RestartCount: number;
  State: {
    Status: ContainerState;
    Running: boolean;
    Paused: boolean;
    Restarting: boolean;
    OOMKilled: boolean;
    Dead: boolean;
    Pid: number;
    ExitCode: number;
    Error: string;
    StartedAt: string;
    FinishedAt: string;
    Health?: {
      Status: string;
      FailingStreak: number;
      Log: Array<{
        Start: string;
        End: string;
        ExitCode: number;
        Output: string;
      }> | null;
    };
  };
  Config: {
    Hostname: string;
    Domainname: string;
    User: string;
    Image: string;
    Env: string[] | null;
    Cmd: string[] | null;
    Entrypoint: string[] | null;
    Labels: Record<string, string>;
    ExposedPorts?: Record<string, object>;
    WorkingDir: string;
    Tty: boolean;
    OpenStdin: boolean;
    StopSignal?: string;
    Healthcheck?: {
      Test: string[] | null;
      Interval: number;
      Timeout: number;
      Retries: number;
      StartPeriod: number;
    } | null;
  };
  NetworkSettings: {
    Networks: Record<string, ContainerNetwork>;
    Ports: Record<string, Array<{ HostIp: string; HostPort: string }> | null>;
  };
  Mounts: ContainerMount[];
  HostConfig: {
    RestartPolicy: { Name: string; MaximumRetryCount: number };
    Binds: string[] | null;
    PortBindings: Record<string, Array<{ HostIp: string; HostPort: string }>> | null;
    Memory: number;
    MemorySwap: number;
    MemoryReservation: number;
    NanoCpus: number;
    CpuShares: number;
    CpuPeriod: number;
    CpuQuota: number;
    CpusetCpus: string;
    CpusetMems: string;
    NetworkMode: string;
    BlkioWeight: number;
    BlkioDeviceReadBps: Array<{ Path: string; Rate: number }> | null;
    BlkioDeviceWriteBps: Array<{ Path: string; Rate: number }> | null;
    Privileged: boolean;
    ReadonlyRootfs: boolean;
    Dns: string[] | null;
    DnsSearch: string[] | null;
    CapAdd: string[] | null;
    CapDrop: string[] | null;
    ShmSize: number;
    PidMode: string;
    Runtime: string;
    LogConfig: { Type: string; Config: Record<string, string> };
    AutoRemove: boolean;
    ExtraHosts: string[] | null;
    Init: boolean | null;
    SecurityOpt: string[] | null;
    UsernsMode: string;
    OomKillDisable: boolean | null;
    PidsLimit: number | null;
    DnsOptions: string[] | null;
    Devices: Array<{ PathOnHost: string; PathInContainer: string; CgroupPermissions: string }> | null;
    DeviceRequests: Array<{
      Driver: string;
      Count: number;
      DeviceIDs: string[] | null;
      Capabilities: string[][] | null;
      Options: Record<string, string> | null;
    }> | null;
    Ulimits: Array<{ Name: string; Soft: number; Hard: number }> | null;
  };
  Protected?: boolean;
};

export type ImageInfo = {
  Id: string;
  ParentId: string;
  RepoTags: string[] | null;
  RepoDigests: string[] | null;
  Created: number;
  Size: number;
  SharedSize: number;
  VirtualSize: number;
  Labels: Record<string, string> | null;
  Containers: number;
  Protected?: boolean;
};

export type ImageInspect = {
  Id: string;
  RepoTags: string[] | null;
  RepoDigests: string[] | null;
  Parent: string;
  Created: string;
  DockerVersion: string;
  Author: string;
  Config: {
    Env: string[] | null;
    Cmd: string[] | null;
    Entrypoint: string[] | null;
    ExposedPorts: Record<string, object> | null;
    Volumes: Record<string, object> | null;
    Labels: Record<string, string> | null;
    WorkingDir: string;
    User: string;
    StopSignal: string;
  };
  Architecture: string;
  Variant: string;
  Os: string;
  OsVersion: string;
  Size: number;
  RootFS: {
    Type: string;
    Layers: string[] | null;
  };
  GraphDriver: {
    Name: string;
    Data: Record<string, string> | null;
  };
  Metadata: {
    LastTagTime: string;
  };
  Protected?: boolean;
};

export type ImageHistoryItem = {
  Comment: string;
  Created: number;
  CreatedBy: string;
  Id: string;
  Size: number;
  Tags: string[] | null;
};

export type VolumeInfo = {
  Name: string;
  Driver: string;
  Mountpoint: string;
  CreatedAt: string;
  Labels: Record<string, string> | null;
  Scope: string;
  Options: Record<string, string> | null;
  UsageData?: {
    Size: number;
    RefCount: number;
  };
  RefCount: number;
  Protected?: boolean;
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
    }>;
  };
  Internal: boolean;
  Attachable: boolean;
  Containers: Record<
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

export type HostMetrics = {
  host: HostInfo;
  disk: DiskUsage;
};

export type ContainerMetric = {
  id: string;
  name: string;
  cpuPercent: number;
  memUsage: number;
  memLimit: number;
  memPercent: number;
  netRx: number;
  netTx: number;
  blockRead: number;
  blockWrite: number;
  pids: number;
};

export type FileEntry = {
  name: string;
  path: string;
  size: number;
  mode: string;
  isDir: boolean;
  modTime: string;
  linkTarget?: string;
};

export type ContainerProcesses = {
  Titles: string[];
  Processes: string[][];
};

export type ContainerStats = {
  read: string;
  cpu_stats: {
    cpu_usage: { total_usage: number };
    system_cpu_usage: number;
    online_cpus: number;
  };
  precpu_stats: {
    cpu_usage: { total_usage: number };
    system_cpu_usage: number;
  };
  memory_stats: {
    usage: number;
    limit: number;
    stats?: { cache?: number };
  };
  networks?: Record<string, { rx_bytes: number; tx_bytes: number }>;
  blkio_stats: {
    io_service_bytes_recursive: Array<{ major: number; minor: number; op: string; value: number }> | null;
  };
};

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export type ContainerState = 'created' | 'running' | 'paused' | 'restarting' | 'removing' | 'exited' | 'dead';

/** Must stay in sync with docker.SelfTarget in core/docker/selfprotect.go. */
export type HostSelfTarget = 'app' | 'helper' | 'protected';

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
  /** Set when acting on the container needs host_self.manage: McHarbor itself, a helper, or a protected container. */
  SelfTarget?: HostSelfTarget;
  /** Why a container on McHarbor's own environment reaches its host; gates a shell or file write. */
  HostAccess?: string[];
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
    /** `--tmpfs` mounts: target → options. Docker lists these here, not under Mounts. */
    Tmpfs?: Record<string, string> | null;
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
  SelfTarget?: HostSelfTarget;
  HostAccess?: string[];
};

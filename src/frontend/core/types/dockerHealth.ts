// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

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

/** One healthcheck probe Docker ran, as persisted by the health collector. */
export type HealthProbe = {
  start: string;
  end?: string;
  exitCode: number;
  durationMs: number;
  output?: string;
};

/** One observation of a container's run state, sampled on a fixed grid. */
export type UptimeSample = {
  sampledAt: string;
  state: string;
  running: boolean;
  health?: string;
};

/** Payload behind the Overview tab's Health and Uptime cards. */
export type ContainerHealthHistory = {
  containerId: string;
  hasHealthcheck: boolean;
  status: string;
  failingStreak: number;
  intervalSeconds: number;
  timeoutSeconds: number;
  retries: number;
  probes: HealthProbe[];
  passCount: number;
  failCount: number;
  samples: UptimeSample[];
  sampleIntervalSeconds: number;
  runningCount: number;
  downCount: number;
  uptimePercent: number;
  currentState: string;
  startedAt?: string;
  restartCount: number;
};

/**
 * The derived health of a remote agent, which is orthogonal to its
 * connected/disconnected status: an agent can be connected and degraded at the
 * same time when its keepalive has gone stale or its round trip has blown past
 * the point where ordinary Docker calls start timing out.
 *
 * `unknown` is an agent environment that has never connected at all, kept
 * distinct from `unhealthy` so a freshly created environment awaiting its first
 * agent does not read as broken.
 */
export type AgentHealth = 'healthy' | 'degraded' | 'unhealthy' | 'unknown';

/** One sampled observation of an agent's connection and health. */
export type AgentHealthSample = {
  sampledAt: string;
  connected: boolean;
  health: AgentHealth;
  /** Absent when there was no live connection to measure. */
  latencyMs?: number;
};

/** One point of the keepalive latency chart under the agent health strip. */
export type AgentLatencyPoint = {
  timestamp: string;
  value: number;
};

/** Payload behind the environment Overview tab's agent health and uptime cards. */
export type AgentHealthHistory = {
  envId: string;
  status: string;
  health: AgentHealth;
  latencyMs?: number;
  pingIntervalSeconds: number;
  samples: AgentHealthSample[];
  sampleIntervalSeconds: number;
  connectedCount: number;
  downCount: number;
  degradedCount: number;
  uptimePercent: number;
  /** Connected-to-disconnected transitions inside the sampled window. */
  outages: number;
  latencySeries: AgentLatencyPoint[];
  avgLatencyMs: number;
  maxLatencyMs: number;
  lastSeen?: string;
};

/** One piece of host hardware reported by `GET /metrics/hardware`. */
export type HardwareDevice = {
  id: string;
  kind: 'gpu' | 'network' | 'usb' | 'disk';
  /** Subsystem the device was read from: `pci`, `usb`, `block`, or whatever
   *  the kernel names for an off-bus adapter (`vmbus`, `platform`). */
  bus: string;
  name: string;
  model?: string;
  driver?: string;
  /** Kernel interface name of a network adapter, e.g. `enp3s0`. */
  interface?: string;
  speedMbps?: number;
  capacity?: number;
  removable?: boolean;
  memoryTotal?: number;
  utilization?: number;
  temperature?: number;
};

/**
 * Devices of the host running McHarbor — not of the selected environment.
 *
 * `available` is separate from three empty lists on purpose: "this
 * platform exposes no inventory" is a different statement from "this
 * machine has no devices", and only the first is worth explaining.
 */
export type HardwareInventory = {
  available: boolean;
  gpus: HardwareDevice[];
  networkAdapters: HardwareDevice[];
  attachedDevices: HardwareDevice[];
  scope: string;
};

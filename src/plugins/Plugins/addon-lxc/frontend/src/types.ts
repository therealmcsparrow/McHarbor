// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/** Mirrored from `backend/model.go`. */

export type ContainerState =
  | 'defined'
  | 'running'
  /** Every process suspended by the cgroup freezer. Not the same as stopped. */
  | 'frozen'
  | 'stopping'
  | 'stopped'
  | 'crashed'
  /** The host's agent cannot be reached, so nothing about this container is confirmed. */
  | 'unknown';

export type InitSystem = 'systemd' | 'openrc' | 'none';
export type MountKind = 'volume' | 'bind' | 'device';
export type SnapshotMechanism = 'zfs' | 'btrfs' | 'lvm' | 'rsync';
export type AddressMode = 'dhcp' | 'static';
export type UnavailableReason =
  | 'not_agent_environment'
  | 'agent_offline'
  | 'agent_too_old'
  | 'agent_bridge_missing'
  | '';

/**
 * cgroup v2 limits.
 *
 * `cpuMax` stays in the kernel's own "quota period" form because that is what
 * `cpu.max` contains. Converting it to a core count for storage would mean two
 * lossy conversions between the operator and the file; the UI converts for
 * display only, where a rounding error is cosmetic.
 */
export interface Limits {
  cpuMax?: string;
  cpuWeight?: number;
  cpuset?: string;
  memoryMaxBytes?: number;
  /** A soft limit the kernel throttles against before it kills anything. */
  memoryHighBytes?: number;
  swapMaxBytes: number;
  pidsMax?: number;
  ioMax?: string;
  diskQuotaBytes?: number;
}

export interface NIC {
  id: string;
  containerId: string;
  networkId: string;
  /** Copied from the network at create time — this plugin cannot read compute's rows. */
  bridge: string;
  vlanTag?: number;
  attachKind?: string;
  parentLink?: string;
  linkMode?: string;
  slot: number;
  /** What the interface is called inside the container. */
  guestName: string;
  mac: string;
  /** What it is called on the host, which is what `ip link` shows. */
  vethHost?: string;
  addressMode: AddressMode;
  staticIp?: string;
  staticGw?: string;
  staticIpv6?: string;
  staticGw6?: string;
  staticDns?: string[];
  createdAt: string;
  updatedAt: string;
}

export interface Mount {
  id: string;
  containerId: string;
  /**
   * A volume lives inside the container's own pool and needs only ct.manage.
   * A bind is an arbitrary host path and a device is a host device node; both
   * need ct.privileged.
   */
  kind: MountKind;
  source: string;
  destination: string;
  readonly: boolean;
  /**
   * Whether the mount is id-shifted into the container's uid range. Without it
   * every file in it appears to belong to nobody from inside an unprivileged
   * container.
   */
  shiftIds: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface Snapshot {
  id: string;
  containerId: string;
  parentId?: string;
  name: string;
  description?: string;
  mechanism: SnapshotMechanism;
  /**
   * Whether the container was frozen while this was taken. False means
   * crash-consistent — the same state a power cut produces.
   */
  wasFrozen: boolean;
  sizeBytes?: number;
  chainDepth: number;
  handle: string;
  createdAt: string;
  updatedAt: string;
}

export interface Container {
  id: string;
  environmentId: string;
  name: string;
  description?: string;

  state: ContainerState;
  /** Computed per response from whether the owning agent is connected. */
  hostAvailable: boolean;
  unavailableReason?: UnavailableReason;

  osFamily?: string;
  osVersion?: string;

  poolId: string;
  poolBackend: string;
  poolTarget: string;
  rootfsPath: string;
  /** Set means this container is copy-on-write over a template's rootfs. */
  templatePath?: string;

  initSystem: InitSystem;
  initArgv?: string[];

  privileged: boolean;
  nesting: boolean;

  uidMapStart?: number;
  uidMapCount?: number;
  gidMapStart?: number;
  gidMapCount?: number;
  idmapMechanism?: 'idmapped' | 'chown';

  /** What was asked for. */
  limits: Limits;
  /** What the kernel took. The UI shows this and edits the one above. */
  appliedLimits?: Limits;
  /** Null for a container whose limits have never reached a running kernel. */
  limitsAppliedAt?: string;

  autostart: boolean;
  autostartOrder: number;
  autostartDelaySeconds: number;

  isTemplate: boolean;
  templateSourceId?: string;

  /** Read from the container's netns by the agent, not from inside it. */
  observedIps?: string[];

  unitName?: string;
  initPid?: number;

  lastStartedAt?: string;
  exitReason?: string;
  lastError?: string;
  createdAt: string;
  updatedAt: string;

  /** Detail response only. */
  nics?: NIC[];
  mounts?: Mount[];
  snapshots?: Snapshot[];
}

export interface Stats {
  containerId: string;
  cpuPercent: number;
  memoryBytes: number;
  memoryMaxBytes: number;
  memoryHighBytes: number;
  swapBytes: number;
  pidsCurrent: number;
  pidsMax: number;
  ioReadBytes: number;
  ioWriteBytes: number;
  netRxBytes: number;
  netTxBytes: number;
  rootfsUsedBytes: number;
  sampledAt: string;
}

export interface Process {
  /** As seen inside the container. */
  pid: number;
  /** The same process from the host, where its pid is different. */
  hostPid?: number;
  ppid?: number;
  user?: string;
  /** From inside the container and therefore untrusted. Rendered as text. */
  command?: string;
  cpuPercent?: number;
  memoryBytes?: number;
}

/** What the limits route returns: asked for, taken, and the gap between them. */
export interface LimitsResponse {
  requested: Limits;
  applied: Limits;
  /** The fields the kernel did not accept as asked. */
  clamped?: string[];
  /** False means the container was stopped and nothing was written to a cgroup. */
  live: boolean;
}

export type ConsoleKind = 'tty' | 'exec';

export interface ConsoleTicket {
  ticket: string;
  expiresIn: number;
  viewers: number;
}

export type { CreateContainerInput, CreateMountInput, CreateNICInput } from './createTypes';

/** Whether a container is executing, which "running" alone does not answer. */
export function isLive(container: Container): boolean {
  return container.state === 'running' || container.state === 'frozen';
}

/** Whether a container's limits can be changed without a restart. */
export function limitsApplyLive(container: Container): boolean {
  return container.hostAvailable && isLive(container);
}

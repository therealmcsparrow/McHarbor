// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export type VMState =
  | 'defined'
  | 'running'
  | 'paused'
  | 'stopping'
  | 'stopped'
  | 'deleting'
  | 'crashed'
  | 'migrating'
  | 'unknown';

export type GuestOSFamily = 'linux' | 'windows' | 'bsd' | 'other';
export type Consistency = 'crash' | 'paused' | 'filesystem' | 'application';
export type UnavailableReason = 'not_agent_environment' | 'agent_offline' | 'agent_too_old' | '';

export interface Disk {
  /** Returned by a live add, never stored: 'hotplug_unclaimed' when the guest did not pick it up. */
  notice?: string;
  id: string;
  vmId: string;
  poolId: string;
  poolBackend: string;
  poolTarget: string;
  slot: number;
  bus: string;
  volumePath: string;
  format: string;
  sizeBytes: number;
  backingPath?: string;
  cacheMode: string;
  discard: boolean;
  readonly: boolean;
  isBoot: boolean;
  isCdrom: boolean;

  /** Explicit place in the boot order; 0 keeps the derivation from isBoot. */
  bootIndex?: number;
  /** Lets a second guest open this disk. Needs a cluster filesystem. */
  shareRw?: boolean;

  /** I/O limits, zero meaning unlimited. */
  readIops?: number;
  writeIops?: number;
  readBps?: number;
  writeBps?: number;
  createdAt: string;
  updatedAt: string;
}

export interface NIC {
  /** As Disk.notice. */
  notice?: string;
  id: string;
  vmId: string;
  networkId: string;
  bridge: string;
  vlanTag?: number;
  attachKind?: string;
  parentLink?: string;
  linkMode?: string;
  slot: number;
  model: string;
  mac: string;
  tapName?: string;
  linkDown?: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface Snapshot {
  id: string;
  vmId: string;
  parentId?: string;
  name: string;
  description?: string;
  mechanism: string;
  consistency: Consistency;
  withMemory: boolean;
  sizeBytes?: number;
  chainDepth: number;
  handle: string;
  childCount: number;
  createdAt: string;
  updatedAt: string;
}

export interface Device {
  id: string;
  vmId: string;
  kind: 'pci' | 'usb' | 'block';
  hostAddress: string;
  description?: string;
  createdAt: string;
  updatedAt: string;
}

export interface CloudInit {
  vmId: string;
  hostname?: string;
  timezone?: string;
  sshKeys?: string[];
  defaultUser?: string;
  hasPassword: boolean;
  networkMode: 'dhcp' | 'static';
  staticIp?: string;
  staticGateway?: string;
  staticDns?: string[];
  userDataExtra?: string;
  instanceId: string;
}

export interface VM {
  id: string;
  environmentId: string;
  name: string;
  description?: string;
  state: VMState;

  guestOsFamily?: GuestOSFamily;
  guestOsVariant?: string;

  vcpus: number;
  cpuModel: string;
  cpuSockets: number;
  cpuCores: number;
  cpuThreads: number;
  memoryMb: number;
  memoryMaxMb?: number;

  machineType: string;
  firmware: 'uefi' | 'bios';
  secureBoot: boolean;
  tpmEnabled: boolean;
  editableNow?: string[];
  displayType: string;
  /** displayType with Auto resolved: the adapter the guest boots with. */
  displayAdapter?: string;
  /** Boot resolution the adapter offers the guest; 0 by 0 is QEMU's default. */
  displayWidth?: number;
  displayHeight?: number;
  /** The SPICE display profile the guest launches with; Fast LAN when it never chose. */
  displayProfileId?: string;

  autostart: boolean;
  autostartOrder: number;
  autostartDelaySeconds: number;

  isTemplate: boolean;
  templateSourceId?: string;

  /**
   * QEMU's own run state behind `state`. It is what separates a deliberately
   * paused guest from one QEMU stopped because its disk backend failed.
   */
  runState?: string;

  /** Hotplug ceiling and what the guest holds now; `memoryMb` is the boot size. */
  vcpusMax?: number;
  memoryCurrentMb?: number;
  virtioMem?: boolean;
  hugePages?: boolean;

  numaPolicy?: 'none' | 'pin' | 'mirror' | '';
  numaHostNodes?: number[];

  vsockCid?: number;
  ioThread?: boolean;

  guestAgentState?: 'connected' | 'absent' | 'unknown' | '';
  guestHostname?: string;
  guestOsName?: string;
  guestOsVersion?: string;
  guestReportedIps?: string[];

  uuid: string;
  unitName?: string;

  lastStartedAt?: string;
  lastError?: string;

  hostAvailable: boolean;
  unavailableReason?: UnavailableReason;
  deleteProgress?: { percent: number; step?: string };
  snapshotProgress?: { percent: number; step?: string };
  revertProgress?: { percent: number; step?: string };
  /** How far a clone's disk copies have got, on the clone's own row while it is created. */
  cloneProgress?: { percent: number; step?: string };

  disks?: Disk[];
  nics?: NIC[];
  currentSnapshotId?: string;
  snapshots?: Snapshot[];
  devices?: Device[];
  cloudInit?: CloudInit;

  createdAt: string;
  updatedAt: string;
}

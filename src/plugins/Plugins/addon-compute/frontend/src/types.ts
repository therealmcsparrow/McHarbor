// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export type ComputeBackend = 'dir' | 'zfs' | 'lvm' | 'btrfs';
export type NetworkMode = 'bridged' | 'managed' | 'external' | 'macvlan';
export type MacvlanLinkMode = 'bridge' | 'vepa' | 'private' | 'passthru';
export type ImageKind = 'cloud_image' | 'rootfs' | 'iso';

export type UnavailableReason = 'not_agent_environment' | 'agent_offline' | 'agent_too_old' | '';

export interface CapabilityItem {
  key: string;
  present: boolean;
  detail?: string;
  remediation?: string;
  critical?: boolean;
  blocks?: 'vm' | 'ct' | 'both' | '';
  /** The host's agent can install this missing item itself. */
  installable?: boolean;
}

/** What installing a capability item did. Must match InstallResult in backend/compute_model.go. */
export interface InstallResult {
  item: string;
  packages?: string[];
  output?: string;
  capability?: Capability;
}

export interface QEMUBinary {
  path: string;
  arch: string;
  version: string;
  machineTypes?: string[];
  kvmAccel: boolean;
}

export interface HostLink {
  name: string;
  kind: string;
  mac?: string;
  mtu?: number;
  up: boolean;
  addresses?: string[];
  master?: string;
  managed?: boolean;
  hasDefaultRoute?: boolean;
  upper?: string[];
}

export interface Capability {
  os: string;
  arch: string;
  kernelVersion?: string;
  agentVersion?: string;
  supported: boolean;
  unsupportedWhy?: string;
  canRunVms: boolean;
  canRunContainers: boolean;
  seccompBuiltIn: boolean;
  supervisorMode?: 'dbus' | 'systemd-run' | (string & {});
  items?: CapabilityItem[];
  qemuBinaries?: QEMUBinary[];
  bridges?: HostLink[];
  probedAt?: string;
}

export interface ComputeEnvironment {
  id: string;
  environmentId: string;
  name?: string;
  connectionType?: string;
  computeEnabled: boolean;
  capability?: Capability;
  probedAt?: string;
  defaultPoolId?: string;
  defaultNetworkId?: string;
  available: boolean;
  unavailableReason?: UnavailableReason;
  createdAt: string;
  updatedAt: string;
  sharedLibrary?: boolean;
}

export interface Pool {
  id: string;
  environmentId: string;
  name: string;
  backend: ComputeBackend;
  target: string;
  isDefault: boolean;
  capacityBytes: number;
  usedBytes: number;
  allocatedBytes: number;
  volumeCount: number;
  instant: boolean;
  sampledAt?: string;
  state: string;
  lastError?: string;
  createdAt: string;
  updatedAt: string;
}

export interface PoolCandidate {
  target: string;
  mountpoint?: string;
  capacityBytes?: number;
  freeBytes?: number;
}

export interface PoolFile {
  name: string;
  path: string;
  isDir?: boolean;
  isSymlink?: boolean;
  linkTarget?: string;
  sizeBytes?: number;
  mode?: string;
  modifiedAt?: string;
  ownerUid?: number;
  ownerGid?: number;
}

export interface PoolBackendInfo {
  backend: ComputeBackend;
  available: boolean;
  detail?: string;
  remediation?: string;
  instant?: boolean;
  candidates?: PoolCandidate[];
}

export interface Network {
  id: string;
  environmentId: string;
  name: string;
  mode: NetworkMode;
  bridge: string;
  vlanTag?: number;
  subnetCidr?: string;
  gatewayIp?: string;
  dhcpEnabled: boolean;
  dhcpRangeStart?: string;
  dhcpRangeEnd?: string;
  natEnabled: boolean;
  uplinkLink?: string;
  linkMode?: string;
  hostAccess: boolean;
  hostAccessIp?: string;
  ipv6Enabled?: boolean;
  hostAccessIpv6?: string;
  hostLinkName?: string;
  isDefault: boolean;
  state: string;
  lastError?: string;
  createdAt: string;
  updatedAt: string;
}

/** The host-side interface a direct network can give the host, as the dialog edits it. */
export interface HostLinkValues {
  hostAccess: boolean;
  hostAccessIp: string;
  ipv6Enabled: boolean;
  hostAccessIpv6: string;
  hostLinkName: string;
}

export interface Lease {
  id: string;
  networkId: string;
  mac: string;
  ip: string;
  hostname?: string;
  guestKind?: string;
  guestId?: string;
  expiresAt?: string;
  createdAt: string;
  updatedAt: string;
}

export type * from './types-images';
export type * from './types-overview';
export type * from './types-logs';

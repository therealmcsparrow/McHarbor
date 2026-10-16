// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { GuestOSFamily } from './types-vm';

export interface Stats {
  vmId: string;
  cpuPercent: number;
  memoryBytes: number;
  memoryMaxBytes: number;
  balloonBytes?: number;
  diskReadBytes: number;
  diskWriteBytes: number;
  netRxBytes: number;
  netTxBytes: number;
  sampledAt: string;
}

/** One stored interval: disk and network are already bytes per second. */
export interface MetricsPoint {
  sampledAt: string;
  cpuPercent: number;
  memoryPercent: number;
  diskReadRate: number;
  diskWriteRate: number;
  netRxRate: number;
  netTxRate: number;
  memoryBytes: number;
  memoryMaxBytes: number;
}

export interface HostDevice {
  kind: 'pci' | 'usb';
  address: string;
  description?: string;
  vendor?: string;
  product?: string;
  driver?: string;
  class?: string;
  iommuGroup?: string;
  assignable: boolean;
  reason?: string;
  attachedTo?: { vmId: string; name: string }[];
}

export interface ConsoleTicket {
  ticket: string;
  /** One single-use ticket per SPICE channel, present only when a batch was asked for. */
  tickets?: string[];
  expiresIn: number;
  viewers: number;
}

export type ConsoleKind = 'vnc' | 'serial' | 'spice';

export interface GuestLog {
  source: string;
  lines: string[];
  notices?: string[];
}

export interface PoolVolume {
  name: string;
  volumePath: string;
  format?: string;
  sizeBytes?: number;
  isBlock?: boolean;
}

export interface CreateVMInput {
  environmentId: string;
  name: string;
  description?: string;
  guestOsFamily?: GuestOSFamily;
  guestOsVariant?: string;
  vcpus: number;
  memoryMb: number;
  memoryMaxMb?: number;
  firmware?: 'uefi' | 'bios';
  secureBoot?: boolean;
  tpmEnabled?: boolean;
  poolId: string;
  poolBackend: string;
  poolTarget: string;
  diskSizeGb: number;
  diskBus?: string;
  diskVolumePath?: string;
  diskFormat?: string;
  diskSizeBytes?: number;
  networkId: string;
  bridge: string;
  vlanTag?: number;
  attachKind?: string;
  parentLink?: string;
  linkMode?: string;
  imageId?: string;
  imageLocalPath?: string;
  isoPaths?: string[];
  cloudInit?: {
    hostname?: string;
    timezone?: string;
    sshKeys?: string[];
    defaultUser?: string;
    password?: string;
    /** Vault secret names, read once at create as the requesting user. */
    passwordSecret?: string;
    sshKeySecrets?: string[];
    networkMode?: 'dhcp' | 'static';
    staticIp?: string;
    staticGateway?: string;
    staticDns?: string[];
    userDataExtra?: string;
    installGuestAgent?: boolean;
  };
  autostart?: boolean;
}

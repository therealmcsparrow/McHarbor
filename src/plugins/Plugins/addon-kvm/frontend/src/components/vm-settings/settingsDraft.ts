// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { VM } from '../../types';

export type Draft = {
  name: string;
  description: string;
  vcpus: number;
  memoryMb: number;
  memoryMaxMb: number;
  cpuModel: string;
  machineType: string;
  firmware: string;
  secureBoot: boolean;
  tpmEnabled: boolean;
  displayType: string;
  displayWidth: number;
  displayHeight: number;
  displayProfileId: string;
  autostart: boolean;
  autostartOrder: number;
  autostartDelaySeconds: number;
  vcpusMax: number;
  virtioMem: boolean;
  hugePages: boolean;
  numaPolicy: string;
  ioThread: boolean;
  vsockEnabled: boolean;
  numaHostNodes: number[];
};

export type GroupName = 'identity' | 'resources' | 'platform' | 'display' | 'startup' | 'features';

export const GROUP_FIELDS: Record<GroupName, readonly (keyof Draft)[]> = {
  identity: ['name', 'description'],
  resources: ['vcpus', 'memoryMb', 'memoryMaxMb', 'cpuModel'],
  platform: ['machineType', 'firmware', 'secureBoot', 'tpmEnabled', 'displayType', 'displayWidth', 'displayHeight'],
  display: ['displayProfileId'],
  startup: ['autostart', 'autostartOrder', 'autostartDelaySeconds'],
  features: ['vcpusMax', 'virtioMem', 'hugePages', 'numaPolicy', 'numaHostNodes', 'ioThread', 'vsockEnabled'],
};

export function draftFrom(vm: VM): Draft {
  return {
    name: vm.name,
    description: vm.description ?? '',
    vcpus: vm.vcpus,
    memoryMb: vm.memoryMb,
    memoryMaxMb: vm.memoryMaxMb ?? 0,
    cpuModel: vm.cpuModel ?? '',
    machineType: vm.machineType ?? '',
    firmware: vm.firmware ?? 'uefi',
    secureBoot: vm.secureBoot ?? false,
    tpmEnabled: vm.tpmEnabled ?? false,
    displayType: vm.displayType ?? '',
    displayWidth: vm.displayWidth ?? 0,
    displayHeight: vm.displayHeight ?? 0,
    displayProfileId: vm.displayProfileId ?? '',
    autostart: vm.autostart ?? false,
    autostartOrder: vm.autostartOrder ?? 0,
    autostartDelaySeconds: vm.autostartDelaySeconds ?? 0,
    vcpusMax: vm.vcpusMax ?? 0,
    virtioMem: vm.virtioMem ?? false,
    hugePages: vm.hugePages ?? false,
    numaPolicy: vm.numaPolicy ?? '',
    ioThread: vm.ioThread ?? false,
    vsockEnabled: (vm.vsockCid ?? 0) >= 3,
    numaHostNodes: vm.numaHostNodes ?? [],
  };
}

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export const VM_FEATURE = {
  VIRTIO_MEM: 'virtio_mem',
  CPU_HOTPLUG: 'cpu_hotplug',
  HUGEPAGES: 'hugepages',
  NUMA: 'numa',
  VIRTIOFS: 'virtiofs',
  VSOCK: 'vsock',
  IO_THROTTLE: 'io_throttle',
  IOTHREAD: 'iothread',
  DIRTY_BITMAP: 'dirty_bitmap',
  BLOCKDEV_BACKUP: 'blockdev_backup',
  VIRTIO_GPU: 'virtio_gpu',
  VIRTIO_SND: 'virtio_snd',
  USB_U2F: 'usb_u2f',
  SPICE: 'spice',
  CLONE: 'clone',
  MIGRATE_COLD: 'migrate_cold',
  MIGRATE_LIVE: 'migrate_live',
  SERIAL_RESIZE: 'serial_resize',
  HOTPLUG: 'hotplug',
  DISPLAY_RESIZE: 'display_resize',
} as const;

export type VMFeature = (typeof VM_FEATURE)[keyof typeof VM_FEATURE];

export interface HugePagesInfo {
  total: number;
  free: number;
  sizeKb?: number;
  surplus?: number;
  reserved?: number;
}

export interface NUMANodeInfo {
  id: number;
  cpus?: number[];
  memoryBytes?: number;
  freeBytes?: number;
}

export interface QEMUCapability {
  version?: string;
  arch?: string;
  machineTypes?: string[];
  cpuModels?: string[];
  features: Partial<Record<VMFeature, boolean>>;
  hugePages?: HugePagesInfo;
  numaNodes?: NUMANodeInfo[];
  probedAt: string;
}

export function useQEMUCapability(environmentId?: string) {
  return useQuery({
    queryKey: ['kvm', 'qemu-capability', environmentId],
    enabled: Boolean(environmentId),
    staleTime: 10 * 60_000,
    retry: false,
    queryFn: async () => assertSuccess(await api.get<QEMUCapability>(`/vms/capability?env=${environmentId}`)),
  });
}

export function supportsFeature(capability: QEMUCapability | undefined, feature: VMFeature): boolean {
  return capability?.features?.[feature] === true;
}

export function machineTypeMissing(capability: QEMUCapability | undefined, machineType: string | undefined): boolean {
  if (!machineType || !capability?.machineTypes?.length) return false;
  return !capability.machineTypes.includes(machineType);
}

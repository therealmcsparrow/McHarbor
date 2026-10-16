// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export const DISK_BUSES = ['virtio-blk', 'virtio-scsi', 'sata', 'ide'] as const;

export function defaultDiskBus(osFamily: string | undefined): string {
  return osFamily === 'windows' ? 'sata' : 'virtio-blk';
}

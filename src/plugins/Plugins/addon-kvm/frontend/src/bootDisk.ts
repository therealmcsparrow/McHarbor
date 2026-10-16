// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { Disk } from './types';

/**
 * The disk the guest boots from: the one flagged as boot, or the non-disc disk
 * first in the boot order. Must stay in step with `isBootDisk` in
 * backend/hotplug.go, which refuses the same disk while the guest runs.
 */
export function isBootDisk(disks: Disk[], disk: Disk): boolean {
  if (disk.isCdrom) return false;
  if (disk.isBoot) return true;
  let best: Disk | undefined;
  let bestIndex = 0;
  for (const candidate of disks) {
    if (candidate.isCdrom) continue;
    const index = candidate.bootIndex && candidate.bootIndex > 0 ? candidate.bootIndex : candidate.isBoot ? 0 : -1;
    if (index < 0) continue;
    if (!best || index < bestIndex) {
      best = candidate;
      bestIndex = index;
    }
  }
  return best?.id === disk.id;
}

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export function formatVmMemory(memoryMb: number): string {
  return memoryMb < 1024 ? `${memoryMb} MB` : `${Math.round(memoryMb / 1024)} GB`;
}

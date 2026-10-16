// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { HostCapability } from './hooks/useComputeCapability';

export type HostLimits = {
  maxVCPUs: number;
  maxMemoryMb: number;
  memoryStepMb: number;
  hostCpus: number;
  hostMemoryMb: number;
  reserveMb: number;
  /** Memory other running guests on this host already hold. */
  otherAllocatedMb: number;
  otherAllocatedVCPUs: number;
  physicalCores: number;
  idealMaxVCPUs: number;
  safeMaxVCPUs: number;
  singleGuestMaxVCPUs: number;
  safeVCPUs: number;
  known: boolean;
  /** The guest asks for more than the host can back. */
  cpusOverBudget: boolean;
  cpusOverSafe: boolean;
  memoryOverBudget: boolean;
};

export type VCPUZone = { upTo: number; tone: 'safe' | 'unsafe' };

const MB = 1024 * 1024;
const MEMORY_STEP_MB = 128;
const MEMORY_RESERVE_FRACTION = 0.15;
const MEMORY_RESERVE_FLOOR_MB = 2048;
const HOST_RESERVE_THREADS = 2;
const RATIO_IDEAL = 1;
const RATIO_SAFE = 2;

/**
 * The ceilings describe the host, never the guest being edited.
 *
 * CPU and memory are bounded differently on purpose. CPU time is shared, so the
 * hardware limit stays the host's thread count and nothing below it is refused;
 * the thresholds are advice, scored across every running guest at once because
 * an overcommit ratio is a property of the host and not of one guest. Ratios
 * are taken against physical cores, since SMT is worth roughly a third more
 * throughput rather than a second execution unit. Memory is not shared — every
 * megabyte handed to one guest is unavailable to the next — so the memory
 * ceiling subtracts both the host's own reserve and what the other running
 * guests already hold.
 */
export function hostLimits(
  capability: HostCapability | undefined,
  current: { vcpus: number; memoryMb: number },
  otherAllocatedMb = 0,
  otherAllocatedVCPUs = 0,
): HostLimits {
  const hostCpus = capability?.hostCpus ?? 0;
  const hostMemoryMb = Math.floor((capability?.hostMemoryBytes ?? 0) / MB);

  if (hostCpus <= 0 || hostMemoryMb <= 0) {
    // Nothing is known about the host, so nothing is claimed about it: the
    // caller falls back to plain number inputs rather than inventing a bound.
    return {
      maxVCPUs: Math.max(current.vcpus, 1),
      maxMemoryMb: Math.max(current.memoryMb, MEMORY_STEP_MB),
      memoryStepMb: MEMORY_STEP_MB,
      hostCpus: 0,
      hostMemoryMb: 0,
      reserveMb: 0,
      otherAllocatedMb: 0,
      otherAllocatedVCPUs: 0,
      physicalCores: 0,
      idealMaxVCPUs: 0,
      safeMaxVCPUs: 0,
      singleGuestMaxVCPUs: 0,
      safeVCPUs: 0,
      known: false,
      cpusOverBudget: false,
      cpusOverSafe: false,
      memoryOverBudget: false,
    };
  }

  const reserveMb = Math.max(MEMORY_RESERVE_FLOOR_MB, Math.floor(hostMemoryMb * MEMORY_RESERVE_FRACTION));
  const availableMb = hostMemoryMb - reserveMb - Math.max(0, otherAllocatedMb);
  const maxMemoryMb = Math.max(MEMORY_STEP_MB, Math.floor(availableMb / MEMORY_STEP_MB) * MEMORY_STEP_MB);

  const physicalCores = Math.max(1, Math.min(capability?.hostPhysicalCores || hostCpus, hostCpus));
  const otherVCPUs = Math.max(0, otherAllocatedVCPUs);

  const idealMaxVCPUs = Math.max(1, physicalCores * RATIO_IDEAL - otherVCPUs);
  const safeMaxVCPUs = Math.max(1, physicalCores * RATIO_SAFE - otherVCPUs);
  const singleGuestMaxVCPUs = Math.max(1, hostCpus - HOST_RESERVE_THREADS);
  const safeVCPUs = Math.min(safeMaxVCPUs, singleGuestMaxVCPUs);

  return {
    maxVCPUs: hostCpus,
    maxMemoryMb,
    memoryStepMb: MEMORY_STEP_MB,
    hostCpus,
    hostMemoryMb,
    reserveMb,
    otherAllocatedMb: Math.max(0, otherAllocatedMb),
    otherAllocatedVCPUs: otherVCPUs,
    physicalCores,
    idealMaxVCPUs,
    safeMaxVCPUs,
    singleGuestMaxVCPUs,
    safeVCPUs,
    known: true,
    cpusOverBudget: current.vcpus > hostCpus,
    cpusOverSafe: current.vcpus > safeVCPUs,
    memoryOverBudget: current.memoryMb > maxMemoryMb,
  };
}

export function vcpuZones(limits: HostLimits): readonly VCPUZone[] | undefined {
  if (!limits.known || limits.safeVCPUs >= limits.maxVCPUs) return undefined;
  return [
    { upTo: limits.safeVCPUs, tone: 'safe' },
    { upTo: limits.maxVCPUs, tone: 'unsafe' },
  ];
}

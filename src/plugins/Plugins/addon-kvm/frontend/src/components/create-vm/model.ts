// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { GuestOSFamily } from '../../types';

export type Step = 'source' | 'resources' | 'storage' | 'network' | 'setup' | 'summary';
export type Source = 'image' | 'iso' | 'blank' | 'template';
export type DiskMode = 'new' | 'existing';
export const IMAGE_STEPS: Step[] = ['source', 'resources', 'storage', 'network', 'setup', 'summary'];
export const MANUAL_STEPS: Step[] = ['source', 'resources', 'storage', 'network', 'summary'];
export const TEMPLATE_STEPS: Step[] = ['source', 'summary'];

// What the field shows before the compute plugin answers, and when neither the
// environment nor McHarbor names a Drives location — the agent's own built-in
// path, which is where a pool lands in that case.
export const AGENT_DEFAULT_DATA_LOCATION = '/var/lib/mcharbor/compute';

/** Sizes that cover most of what people actually create. */
export const PRESETS: Record<string, { vcpus: number; memoryMb: number; diskGb: number }> = {
  small: { vcpus: 1, memoryMb: 2048, diskGb: 20 },
  medium: { vcpus: 2, memoryMb: 4096, diskGb: 40 },
  large: { vcpus: 4, memoryMb: 8192, diskGb: 80 },
};

export interface ComputePool {
  id: string;
  environmentId: string;
  name: string;
  backend: string;
  target: string;
  capacityBytes: number;
  usedBytes: number;
  instant: boolean;
  isDefault: boolean;
}

export interface ComputeNetwork {
  id: string;
  environmentId: string;
  name: string;
  mode?: string;
  bridge: string;
  vlanTag?: number;
  uplinkLink?: string;
  linkMode?: string;
  isDefault: boolean;
}

export interface PoolVolume {
  name: string;
  volumePath: string;
  format?: string;
  sizeBytes?: number;
}

export interface ComputeImage {
  id: string;
  kind: string;
  displayName: string;
  arch: string;
  initSystem?: string;
  cache?: { state: string; localPath: string };
}

/**
 * Guess the guest family from the image name.
 *
 * It only picks the display adapter default, and the operator can change that
 * afterwards — so a guess is fine here, where it would not be for anything that
 * affects whether the guest boots.
 */
export function guessOSFamily(displayName?: string): GuestOSFamily {
  const lower = (displayName ?? '').toLowerCase();
  if (lower.includes('windows')) return 'windows';
  if (lower.includes('bsd')) return 'bsd';
  return 'linux';
}

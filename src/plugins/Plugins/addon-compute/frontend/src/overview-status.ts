// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { OverviewEntry, Pool } from './types';

/**
 * One host's readiness, collapsed to a single tone and label.
 *
 * The table cell and the card both need this answer, and the failure mode of
 * deriving it twice is a row that reads "Ready" beside a card that reads
 * "Setup not finished" for the same host.
 */

export type HostTone = 'success' | 'warning' | 'destructive' | 'secondary';

export interface HostStatus {
  /** A key under `overview.status.` in the compute bundle. */
  key: string;
  tone: HostTone;
}

/** Ranks a host from unusable to ready, most blocking condition first. */
export function hostStatus(entry: OverviewEntry): HostStatus {
  if (entry.unavailableReason === 'not_agent_environment') {
    return { key: 'notAgent', tone: 'secondary' };
  }
  if (!entry.computeEnabled) {
    return { key: 'disabled', tone: 'secondary' };
  }
  if (entry.unavailableReason === 'agent_too_old') {
    return { key: 'tooOld', tone: 'destructive' };
  }
  if (!entry.available) {
    return { key: 'offline', tone: 'destructive' };
  }
  return entry.setupComplete ? { key: 'ready', tone: 'success' } : { key: 'incomplete', tone: 'warning' };
}

/**
 * The fullest pool on a host, which is the one that predicts an outage.
 *
 * A pool that fills pauses every guest on it, so the worst one is what belongs
 * on a summary rather than an average across pools that hides it.
 */
export function worstPool(entry: OverviewEntry): Pool | undefined {
  const pools = entry.pools ?? [];
  return pools.reduce<Pool | undefined>((worst, pool) => {
    if (!worst) return pool;
    const ratio = pool.capacityBytes > 0 ? pool.usedBytes / pool.capacityBytes : 0;
    const best = worst.capacityBytes > 0 ? worst.usedBytes / worst.capacityBytes : 0;
    return ratio > best ? pool : worst;
  }, undefined);
}

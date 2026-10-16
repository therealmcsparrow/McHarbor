// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { StatusTone } from '@resources/theme/status-palette';
import type { SwarmService } from './types';

/**
 * Presentation helpers for the Swarm module.
 *
 * Everything here is pure and string-free of user-facing copy: the tone
 * functions return a token, never a label, so the caller keeps its strings in
 * the i18n bundle where they belong.
 */

/** `2/3` for a replicated service, `3/3 (global)` handled by the caller's label. */
export function replicaText(service: Pick<SwarmService, 'runningTasks' | 'desiredTasks'>): string {
  return `${service.runningTasks}/${service.desiredTasks}`;
}

/**
 * Whether a service has as many tasks running as it wants.
 *
 * `desiredTasks === 0` is converged, not degraded: a service scaled to zero is
 * doing exactly what it was asked to do.
 */
export function isConverged(service: Pick<SwarmService, 'runningTasks' | 'desiredTasks'>): boolean {
  return service.runningTasks >= service.desiredTasks;
}

export function serviceTone(service: Pick<SwarmService, 'runningTasks' | 'desiredTasks'>): StatusTone {
  if (isConverged(service)) return service.desiredTasks === 0 ? 'neutral' : 'success';
  return service.runningTasks === 0 ? 'danger' : 'warning';
}

/**
 * Tone for a task's current state.
 *
 * The states are SwarmKit's own lifecycle, and they are not a simple
 * good/bad split: `complete` is a success for a job task and would read as an
 * outage if it were coloured like `shutdown`.
 */
export function taskStateTone(state: string): StatusTone {
  switch (state) {
    case 'running':
      return 'success';
    case 'complete':
      return 'info';
    case 'failed':
    case 'rejected':
      return 'danger';
    case 'orphaned':
      return 'critical';
    case 'shutdown':
    case 'remove':
      return 'neutral';
    default:
      // new / allocated / pending / assigned / accepted / preparing / ready /
      // starting — all transitional.
      return 'warning';
  }
}

export function nodeStateTone(state: string): StatusTone {
  switch (state) {
    case 'ready':
      return 'success';
    case 'down':
      return 'danger';
    case 'disconnected':
      return 'warning';
    default:
      return 'neutral';
  }
}

export function nodeAvailabilityTone(availability: string): StatusTone {
  switch (availability) {
    case 'active':
      return 'success';
    case 'pause':
      return 'warning';
    case 'drain':
      return 'info';
    default:
      return 'neutral';
  }
}

/**
 * Tone for a rolling update's state. `paused` is a warning rather than a
 * neutral: a rollout that stopped halfway is a thing somebody has to decide
 * about, not a resting state.
 */
export function updateStateTone(state: string): StatusTone {
  switch (state) {
    case 'completed':
    case 'rollback_completed':
      return 'success';
    case 'updating':
    case 'rollback_started':
      return 'info';
    case 'paused':
    case 'rollback_paused':
      return 'warning';
    default:
      return 'neutral';
  }
}

/** SwarmKit stores CPU as billionths of a core. */
export function formatCpus(nanoCpus: number): string {
  if (!nanoCpus) return '—';
  const cpus = nanoCpus / 1e9;
  return cpus >= 1 ? `${cpus.toFixed(cpus % 1 === 0 ? 0 : 2)}` : cpus.toFixed(3).replace(/0+$/, '');
}

export function formatBytes(bytes: number): string {
  if (!bytes) return '—';
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  return `${value.toFixed(value >= 10 || unit === 0 ? 0 : 1)} ${units[unit]}`;
}

/** Short id, the way the Docker CLI shows one. */
export function shortId(id: string): string {
  return id.length > 12 ? id.slice(0, 12) : id;
}

/**
 * A service's published ports, rendered the way `docker service ls` does.
 * Returns an empty string when nothing is published, so a caller can fall back
 * to a dash without testing the array itself.
 */
export function portsText(service: Pick<SwarmService, 'ports'>): string {
  if (!service.ports?.length) return '';
  return service.ports
    .filter((p) => p.publishedPort > 0)
    .map((p) => `${p.publishedPort}:${p.targetPort}/${p.protocol}`)
    .join(', ');
}

/** Strips the stack prefix so a stack's services read as `web`, not `blog_web`. */
export function serviceShortName(itemName: string, stackName: string): string {
  if (stackName && itemName.startsWith(`${stackName}_`)) {
    return itemName.slice(stackName.length + 1);
  }
  return itemName;
}

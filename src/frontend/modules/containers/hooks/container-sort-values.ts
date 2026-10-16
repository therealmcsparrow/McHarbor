// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { ipv4SortKey } from '@resources/utils/ip-sort';
import type { ContainerInfo } from '@core/types/docker';
import { getContainerIP, getStackName, parseHealth } from '../components/container-utils';
import type { ContainerUpdateResult } from './container-column-cells';

const UNIT_SECONDS: Record<string, number> = {
  second: 1,
  minute: 60,
  hour: 3600,
  day: 86400,
  week: 604800,
  month: 2592000,
  year: 31536000,
};

/** Seconds a running container has been up, read from Docker's "Up 3 hours (healthy)" status text; null when not running. */
export function uptimeSeconds(statusText: string): number | null {
  const up = statusText.match(/^Up\s+(.+?)(\s+\(|$)/);
  if (!up?.[1]) return null;
  const text = up[1].trim().toLowerCase();
  if (text.startsWith('less than')) return 0;
  const first = text.split(' ')[0] ?? '';
  const amount = /^\d+$/.test(first) ? Number(first) : 1;
  const unit = Object.keys(UNIT_SECONDS).find((u) => text.includes(u));
  if (!unit || !Number.isFinite(amount)) return null;
  return amount * (UNIT_SECONDS[unit] ?? 0);
}

export function ipSortKey(container: ContainerInfo): string | null {
  return ipv4SortKey(getContainerIP(container));
}

/** The lowest published host port; null when nothing is published. */
export function firstPublicPort(ports: ContainerInfo['Ports']): number | null {
  const published = (ports ?? []).map((p) => p.PublicPort).filter((n): n is number => !!n);
  return published.length ? Math.min(...published) : null;
}

export function healthSortValue(container: ContainerInfo): string {
  return parseHealth(container.Status) ?? 'none';
}

export function stackSortValue(container: ContainerInfo): string | null {
  return getStackName(container);
}

/** Orders update results error, up to date, update available; null when no check has run. */
export function updateSortValue(result: ContainerUpdateResult | undefined): number | null {
  if (!result) return null;
  if (result.error) return 0;
  return result.updateAvailable ? 2 : 1;
}

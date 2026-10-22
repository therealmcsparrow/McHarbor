// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQueries, useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { PERM } from '@core/auth/permissionKeys';
import { usePermission } from '@core/auth/usePermission';
import type { HostFSStatus } from '@core/types/docker';
import { assertSuccess } from '@resources/utils/api-mutation';

export const DEFAULT_DISK_WARNING_PERCENT = 85;
export const DEFAULT_DISK_CRITICAL_PERCENT = 95;

const REFRESH_MS = 60_000;

export type HostDiskLevel = 'normal' | 'warning' | 'critical';

/**
 * The server's `level` wins so the strip and the alert rules cannot disagree; only when it is
 * null do the reported thresholds apply, and a missing or zero threshold falls back to 85/95.
 */
export function hostDiskLevel(
  s: Pick<HostFSStatus, 'percent' | 'thresholds'> & { level?: HostFSStatus['level'] },
): HostDiskLevel {
  if (s.level) return s.level;
  const warning = s.thresholds?.warning || DEFAULT_DISK_WARNING_PERCENT;
  const critical = s.thresholds?.critical || DEFAULT_DISK_CRITICAL_PERCENT;
  if (s.percent >= critical) return 'critical';
  if (s.percent >= warning) return 'warning';
  return 'normal';
}

function hostFsQuery(envId: string) {
  return {
    queryKey: ['host-fs', envId] as const,
    queryFn: () => api.get<HostFSStatus>('/metrics/host/fs', { env: envId }).then(assertSuccess),
    refetchInterval: REFRESH_MS,
    retry: false,
  };
}

/** The query only runs with host.view, so a denied caller gets `allowed: false` instead of a 403. */
export function useHostDiskStatus(envId: string | undefined, enabled = true) {
  const allowed = usePermission(PERM.HOST_VIEW);
  const query = useQuery({
    ...hostFsQuery(envId ?? ''),
    enabled: allowed && enabled && Boolean(envId),
  });
  return { ...query, allowed };
}

export type HostDiskFleet = { critical: number; warning: number; noData: number };

/** Counts across environments for the dashboard sentence; shares the per-environment cache keys. */
export function useHostDiskFleet(envIds: string[]): HostDiskFleet | undefined {
  const allowed = usePermission(PERM.HOST_VIEW);
  const results = useQueries({
    queries: envIds.map((id) => ({ ...hostFsQuery(id), enabled: allowed })),
  });
  if (!allowed || envIds.length === 0) return undefined;
  const fleet: HostDiskFleet = { critical: 0, warning: 0, noData: 0 };
  for (const result of results) {
    const row = result.data;
    if (!row || row.state === 'unsupported') continue;
    if (row.state !== 'ok') {
      fleet.noData += 1;
      continue;
    }
    const level = hostDiskLevel(row);
    if (level === 'critical') fleet.critical += 1;
    if (level === 'warning') fleet.warning += 1;
  }
  return fleet;
}

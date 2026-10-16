// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { useQueryClient } from '@tanstack/react-query';

export const KEYS = {
  list: (envId?: string, templates?: boolean) =>
    ['vms', 'list', envId ?? 'all', templates ? 'templates' : 'guests'] as const,
  detail: (id: string) => ['vms', 'detail', id] as const,
  stats: (id: string) => ['vms', 'stats', id] as const,
  statsHistory: (id: string) => ['vms', 'stats-history', id] as const,
  disks: (id: string) => ['vms', 'disks', id] as const,
  nics: (id: string) => ['vms', 'nics', id] as const,
  snapshots: (id: string) => ['vms', 'snapshots', id] as const,
};

export function params(entries: Record<string, string | undefined>): Record<string, string> | undefined {
  const out: Record<string, string> = {};
  for (const [key, value] of Object.entries(entries)) {
    if (value) out[key] = value;
  }
  return Object.keys(out).length > 0 ? out : undefined;
}

/** Invalidate everything about one guest after a change. */
export function invalidateVM(queryClient: ReturnType<typeof useQueryClient>, id?: string) {
  void queryClient.invalidateQueries({ queryKey: ['vms', 'list'] });
  if (id) {
    void queryClient.invalidateQueries({ queryKey: ['vms', 'detail', id] });
    void queryClient.invalidateQueries({ queryKey: ['vms', 'disks', id] });
    void queryClient.invalidateQueries({ queryKey: ['vms', 'nics', id] });
    void queryClient.invalidateQueries({ queryKey: ['vms', 'snapshots', id] });
  }
}

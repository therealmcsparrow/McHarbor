// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { useQueryClient } from '@tanstack/react-query';

export const KEYS = {
  list: (envId?: string, templates?: boolean) =>
    ['containers', 'list', envId ?? 'all', templates ? 'templates' : 'containers'] as const,
  detail: (id: string) => ['containers', 'detail', id] as const,
  stats: (id: string) => ['containers', 'stats', id] as const,
  processes: (id: string) => ['containers', 'processes', id] as const,
  nics: (id: string) => ['containers', 'nics', id] as const,
  mounts: (id: string) => ['containers', 'mounts', id] as const,
  snapshots: (id: string) => ['containers', 'snapshots', id] as const,
};

export function params(entries: Record<string, string | undefined>): Record<string, string> | undefined {
  const out: Record<string, string> = {};
  for (const [key, value] of Object.entries(entries)) {
    if (value) out[key] = value;
  }
  return Object.keys(out).length > 0 ? out : undefined;
}

/** Invalidate everything about one container after a change. */
export function invalidate(queryClient: ReturnType<typeof useQueryClient>, id?: string) {
  void queryClient.invalidateQueries({ queryKey: ['containers', 'list'] });
  if (id) {
    for (const scope of ['detail', 'stats', 'nics', 'mounts', 'snapshots'] as const) {
      void queryClient.invalidateQueries({ queryKey: ['containers', scope, id] });
    }
  }
}

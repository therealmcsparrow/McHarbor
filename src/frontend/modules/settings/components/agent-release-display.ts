// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { AgentRelease, AgentReleaseStatus } from '../hooks/useAgentReleases';

export const STATUS_KEY: Record<AgentReleaseStatus, string> = {
  pulling: 'agentReleases.status.pulling',
  ready: 'agentReleases.status.ready',
  failed: 'agentReleases.status.failed',
};

export const STATUS_VARIANT: Record<AgentReleaseStatus, 'secondary' | 'success' | 'destructive'> = {
  pulling: 'secondary',
  ready: 'success',
  failed: 'destructive',
};

/**
 * Whether the stored releases already cover version at build: the same or a newer build
 * is stored, or one is downloading. Matching on the version alone would hide every rebuild.
 */
export function storedCovers(releases: AgentRelease[], version: string, build = ''): boolean {
  return releases.some(
    (r) => r.version === version && r.status !== 'failed' && (r.status === 'pulling' || r.build >= build),
  );
}

export function platforms(release: AgentRelease, kind: 'binary' | 'image'): string {
  return release.assets
    .filter((a) => a.kind === kind)
    .map((a) => `${a.os}/${a.arch}`)
    .join(', ');
}

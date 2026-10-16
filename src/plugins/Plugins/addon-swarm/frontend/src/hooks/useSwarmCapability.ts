// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { useEnvironmentStore } from '../host-env';
import type { SwarmCapability } from '../types';

const EMPTY: SwarmCapability = {
  capable: false,
  nodeId: '',
  nodeRole: '',
  controlAvailable: false,
  clusterId: '',
  nodes: 0,
  managers: 0,
};

/**
 * Whether the current environment is a Swarm, and what role its local node
 * plays.
 *
 * This drives the sidebar section and every route guard in the module, so it
 * is asked on effectively every page load. `staleTime` matches the server's
 * own 60s cache: asking more often than the server can answer differently
 * spends a round trip to learn nothing.
 *
 * The endpoint answers `capable: false` rather than an error for a daemon that
 * is unreachable or simply is not a Swarm, so there is no error state to
 * render here — a missing answer and a negative answer mean the same thing to
 * every caller.
 */
export function useSwarmCapability() {
  const envId = useEnvironmentStore((s) => s.currentId);

  const query = useQuery({
    queryKey: ['swarm', 'capability', envId],
    queryFn: () =>
      api.get<SwarmCapability>('/swarm/capability', envId ? { env: envId } : {}).then((r) => r.data ?? EMPTY),
    staleTime: 60_000,
    refetchInterval: 60_000,
  });

  const capability = query.data ?? EMPTY;

  return {
    ...query,
    capability,
    /** The environment is in a Swarm, whatever role this node plays. */
    isSwarm: capability.capable,
    /**
     * The environment can answer control-plane questions. Every listing in
     * this module needs a manager; a worker can only report what it is.
     */
    isManager: capability.capable && capability.controlAvailable,
  };
}

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';

/**
 * Fetches configured notification channel types and returns a set of
 * satisfied capability strings (e.g. 'email', 'channel:slack', 'channel:any',
 * 'environment:any', 'environment:docker').
 *
 * Nodes with a `requires` field that isn't in the set should be disabled.
 */
export function useNodeAvailability() {
  const { data } = useQuery({
    queryKey: ['workflow-nodes', 'capabilities'],
    queryFn: async () => {
      const canonical = await api.get<string[]>('/communication-channels/capabilities');

      const environments = await api.get<Array<{ id: string; orchestratorType: string }>>('/environments');

      if (canonical.success) {
        return {
          channelTypes: canonical.data ?? [],
          environments: environments.data ?? [],
        };
      }

      const legacy = await api.get<string[]>('/notifications/configured-types');
      return {
        channelTypes: legacy.data ?? [],
        environments: environments.data ?? [],
      };
    },
    staleTime: 60_000,
  });

  return useMemo(() => {
    const capabilities = new Set<string>();
    let hasCommunicationChannel = false;
    if (data?.channelTypes) {
      for (const capability of data.channelTypes) {
        capabilities.add(capability);
        capabilities.add(`channel:${capability}`);
        if (capability !== 'email') {
          hasCommunicationChannel = true;
        }
      }
      if (hasCommunicationChannel) {
        capabilities.add('channel:any');
      }
    }

    if (data?.environments && data.environments.length > 0) {
      capabilities.add('environment:any');

      for (const environment of data.environments) {
        if (environment.orchestratorType) {
          capabilities.add(`environment:${environment.orchestratorType}`);
        }
      }
    }

    return capabilities;
  }, [data]);
}

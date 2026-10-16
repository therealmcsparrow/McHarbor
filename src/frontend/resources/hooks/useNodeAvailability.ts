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
      // The legacy fallback to /notifications/configured-types is gone with
      // the module that served it: migration 072 dropped its table, so the
      // fallback could only ever have returned an error.
      const canonical = await api.get<string[]>('/communication-channels/capabilities');

      const environments = await api.get<Array<{ id: string }>>('/environments');

      return {
        channelTypes: canonical.data ?? [],
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

      // `environment:docker` was the only value this ever produced once
      // Kubernetes moved out, and `environment:any` above already covers it.
      capabilities.add('environment:docker');
    }

    return capabilities;
  }, [data]);
}

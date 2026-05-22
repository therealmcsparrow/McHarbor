// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api, type PaginatedData } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
import { useCurrentEnvironmentActivitySettings } from '@resources/hooks/useCurrentEnvironmentActivitySettings';
import type { ContainerEvent } from '../components/EventDetails';

export function useEvents() {
  const envId = useEnvironmentStore((state) => state.currentId);
  const { trackContainerEventsEnabled } = useCurrentEnvironmentActivitySettings();

  return useQuery({
    queryKey: ['events', envId],
    queryFn: () =>
      api
        .get<PaginatedData<ContainerEvent>>('/activity', {
          per_page: '100',
          ...(envId ? { env: envId } : {}),
        })
        .then((response) => response.data?.items ?? []),
    refetchInterval: trackContainerEventsEnabled ? 10_000 : false,
  });
}

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useInfiniteQuery } from '@tanstack/react-query';
import { api, type PaginatedData } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
import type { DateRange } from '@resources/utils/date-range';
import type { ContainerEvent } from '../components/EventDetails';

const PAGE_SIZE = 100;

export type EventsPage = PaginatedData<ContainerEvent>;

export function useEvents(range: DateRange = { from: null, to: null }) {
  const envId = useEnvironmentStore((state) => state.currentId);

  const baseParams: Record<string, string> = {
    per_page: String(PAGE_SIZE),
  };
  if (envId) baseParams.env = envId;
  if (range.from) baseParams.from = range.from.toISOString();
  if (range.to) baseParams.to = range.to.toISOString();

  return useInfiniteQuery({
    queryKey: [
      'events',
      envId,
      range.from?.toISOString() ?? null,
      range.to?.toISOString() ?? null,
    ],
    queryFn: async ({ pageParam }) => {
      const params: Record<string, string> = { ...baseParams, page: String(pageParam) };
      const response = await api.get<EventsPage>('/activity', params);
      return response.data ?? { items: [], total: 0, page: 1, per_page: PAGE_SIZE, total_pages: 0 };
    },
    initialPageParam: 1,
    getNextPageParam: (lastPage) =>
      lastPage.page < lastPage.total_pages ? lastPage.page + 1 : undefined,
  });
}

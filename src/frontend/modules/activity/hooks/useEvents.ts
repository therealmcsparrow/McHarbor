// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The `/activity` endpoint is server-paged, so this is a plain paged query.
// It used to be a `useInfiniteQuery` driving an intersection-observer sentinel;
// the page now uses the shared pager like every other list, which also means the
// row count it reports is the real total rather than "however much you scrolled".

import { useQuery } from '@tanstack/react-query';
import { api, type PaginatedData } from '@core/api/client';
import type { DateRange } from '@resources/utils/date-range';
import type { SearchMode } from '@resources/utils/search-filter';
import type { ContainerEvent } from '../components/EventDetails';

const DEFAULT_PAGE_SIZE = 25;

export type EventsPage = PaginatedData<ContainerEvent>;

type UseEventsPageArgs = {
  page: number;
  perPage?: number;
  range?: DateRange;
  search?: string;
  /**
   * `contains` and `exact` are matched by the API, so the pager total counts
   * the rows the search actually returns. `regex` has no portable SQL
   * equivalent across the three supported drivers, so it stays a client-side
   * filter over the page in hand.
   */
  mode?: SearchMode;
  envId?: string;
};

export function useEventsPage({
  page,
  perPage = DEFAULT_PAGE_SIZE,
  range = { from: null, to: null },
  search = '',
  mode = 'contains',
  envId = '',
}: UseEventsPageArgs) {
  const params: Record<string, string> = {
    page: String(page),
    per_page: String(perPage),
  };
  if (envId) params.env = envId;
  if (range.from) params.from = range.from.toISOString();
  if (range.to) params.to = range.to.toISOString();
  const serverSearch = mode === 'regex' ? '' : search.trim();
  if (serverSearch) {
    params.search = serverSearch;
    params.search_mode = mode;
  }

  return useQuery({
    queryKey: [
      'events',
      envId,
      page,
      perPage,
      range.from?.toISOString() ?? null,
      range.to?.toISOString() ?? null,
      serverSearch,
      serverSearch ? mode : null,
    ],
    queryFn: async () => {
      const response = await api.get<EventsPage>('/activity', params);
      return response.data ?? { items: [], total: 0, page: 1, per_page: perPage, total_pages: 0 };
    },
    // Docker events arrive continuously; a page opened on the newest events
    // is stale within seconds otherwise.
    refetchInterval: 15_000,
  });
}
